package node

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sync"

	diskpkg "github.com/JLPAY/gwayne/pkg/monitor/disk"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
)

// KubeletSummary 解析 kubelet /stats/summary 返回的关键字段（仅保留巡检所需字段）。
// 参考：https://github.com/kubernetes/kubernetes/blob/master/pkg/kubelet/stats/summary.go
type KubeletSummary struct {
	Node struct {
		NodeName string `json:"nodeName"`
		Memory   struct {
			AvailableBytes  int64 `json:"availableBytes"`
			UsageBytes      int64 `json:"usageBytes"`
			WorkingSetBytes int64 `json:"workingSetBytes"`
		} `json:"memory"`
		Fs struct {
			AvailableBytes int64 `json:"availableBytes"`
			CapacityBytes  int64 `json:"capacityBytes"`
			UsedBytes      int64 `json:"usedBytes"`
		} `json:"fs"`
		// Runtime.ImageFs 是容器镜像所在文件系统（与根 fs 通常分离），巡检磁盘时需一并纳入。
		Runtime struct {
			ImageFs struct {
				AvailableBytes int64 `json:"availableBytes"`
				CapacityBytes  int64 `json:"capacityBytes"`
				UsedBytes      int64 `json:"usedBytes"`
			} `json:"imageFs"`
		} `json:"runtime"`
	} `json:"node"`
}

// 节点健康等级
const (
	StatusOK       = "ok"
	StatusWarning  = "warning"
	StatusCritical = "critical"
)

// NodeInspectionItem 单个节点的巡检结果
type NodeInspectionItem struct {
	Name                string            `json:"name"`
	Ready               bool              `json:"ready"`
	MemoryCapacityBytes int64             `json:"memoryCapacityBytes"`
	MemoryUsedBytes     int64             `json:"memoryUsedBytes"`
	MemoryUsedPercent   float64           `json:"memoryUsedPercent"`
	DiskCapacityBytes   int64             `json:"diskCapacityBytes"`
	DiskUsedBytes       int64             `json:"diskUsedBytes"`
	DiskUsedPercent     float64           `json:"diskUsedPercent"`
	DiskAvailableBytes  int64             `json:"diskAvailableBytes"`
	// Disks 为节点全部真实分区（已过滤虚拟/容器层文件系统，按设备去重、按使用率降序）。
	Disks   []diskpkg.DiskUsage `json:"disks,omitempty"`
	Status  string              `json:"status"`
	Messages []string           `json:"messages,omitempty"`
	Error   string              `json:"error,omitempty"`
}

// NodeInspectionResult 单个集群的巡检结果汇总
type NodeInspectionResult struct {
	Cluster  string               `json:"cluster"`
	Total    int                  `json:"total"`
	Warning  int                  `json:"warning"`
	Critical int                  `json:"critical"`
	Items    []NodeInspectionItem `json:"items"`
	Error    string               `json:"error,omitempty"`
}

// GetNodeStatsSummary 经 API server 代理调用 kubelet /stats/summary，拿到内存 workingSet 与文件系统用量。
func GetNodeStatsSummary(clientset kubernetes.Interface, nodeName string) (*KubeletSummary, error) {
	raw, err := clientset.CoreV1().RESTClient().
		Get().
		Resource("nodes").
		Name(nodeName).
		SubResource("proxy").
		Suffix("stats/summary").
		DoRaw(context.TODO())
	if err != nil {
		return nil, fmt.Errorf("获取节点 %s kubelet 统计失败: %v", nodeName, err)
	}
	var s KubeletSummary
	if err := json.Unmarshal(raw, &s); err != nil {
		return nil, fmt.Errorf("解析节点 %s kubelet 统计失败: %v", nodeName, err)
	}
	return &s, nil
}

// InspectNodes 巡检指定集群所有节点的内存与磁盘使用率。
// 内存来自 kubelet workingSet；磁盘来自 Prometheus node-exporter 的 node_filesystem_*（覆盖全部真实分区），
// node-exporter 无该节点数据时回退到 kubelet 根/镜像文件系统。
// promURL/promToken 为 Prometheus 数据源地址与可选令牌（由调用方从配置读取后注入，便于单测）。
// warning/critical 为百分比阈值，决定 ok/warning/critical 等级。
func InspectNodes(ctx context.Context, clientset kubernetes.Interface, promURL, promToken string, warning, critical int) (*NodeInspectionResult, error) {
	nodeList, err := clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("列举节点失败: %v", err)
	}

	result := &NodeInspectionResult{Total: len(nodeList.Items)}

	// 收集节点名，一次性向 Prometheus 拉取全部节点的分区使用率（比逐节点查询高效）。
	names := make([]string, 0, len(nodeList.Items))
	for i := range nodeList.Items {
		names = append(names, nodeList.Items[i].Name)
	}
	diskMap, diskErr := diskpkg.GetNodeDiskUsage(promURL, promToken, names)
	if diskErr != nil {
		klog.Warningf("通过 Prometheus 拉取磁盘分区使用率失败，将回退 kubelet 文件系统: %v", diskErr)
	}

	items := make([]NodeInspectionItem, 0, len(nodeList.Items))
	var wg sync.WaitGroup
	var mu sync.Mutex
	sem := make(chan struct{}, 10)
	for i := range nodeList.Items {
		n := &nodeList.Items[i]
		wg.Add(1)
		go func(n *v1.Node) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			disks := diskMap[n.Name]
			item := inspectOneNode(ctx, clientset, n, disks, diskErr != nil, warning, critical)
			mu.Lock()
			items = append(items, item)
			mu.Unlock()
		}(n)
	}
	wg.Wait()

	for _, it := range items {
		switch it.Status {
		case StatusCritical:
			result.Critical++
		case StatusWarning:
			result.Warning++
		}
	}
	result.Items = items
	return result, nil
}

// inspectOneNode 巡检单个节点
func inspectOneNode(ctx context.Context, clientset kubernetes.Interface, n *v1.Node, disks []diskpkg.DiskUsage, promFailed bool, warning, critical int) NodeInspectionItem {
	item := NodeInspectionItem{Name: n.Name}

	// Ready 状态
	for _, c := range n.Status.Conditions {
		if c.Type == v1.NodeReady {
			item.Ready = c.Status == v1.ConditionTrue
		}
	}

	// 内存容量（来自 node.Status.Capacity）
	if mem := n.Status.Capacity.Memory(); mem != nil {
		item.MemoryCapacityBytes = mem.Value()
	}

	// 拉取 kubelet 统计（内存 + 作为磁盘的回退来源）
	summary, err := GetNodeStatsSummary(clientset, n.Name)
	if err != nil {
		item.Error = err.Error()
		item.Status = StatusWarning
		item.Messages = []string{"kubelet 统计拉取失败，无法评估内存/磁盘使用率"}
		klog.Warningf("节点 %s 巡检拉取 kubelet 统计失败: %v", n.Name, err)
		return item
	}

	// 内存使用率（workingSet 才是压力口径，而非含缓存的 usageBytes）
	memUsed := summary.Node.Memory.WorkingSetBytes
	item.MemoryUsedBytes = memUsed
	if item.MemoryCapacityBytes > 0 {
		item.MemoryUsedPercent = pct(memUsed, item.MemoryCapacityBytes)
	}

	// 磁盘：优先用 Prometheus 全分区数据；缺失（node-exporter 无数据）时回退 kubelet 根/镜像文件系统
	disksForNode := disks
	useFallback := false
	if len(disksForNode) == 0 {
		disksForNode = disksFromKubelet(summary)
		useFallback = true
	}

	worst := diskpkg.WorstDisk(disksForNode)
	item.Disks = disksForNode
	item.DiskUsedPercent = worst.UsedPercent
	item.DiskCapacityBytes = worst.CapacityBytes
	item.DiskUsedBytes = worst.UsedBytes
	item.DiskAvailableBytes = worst.AvailBytes

	status, msgs := evaluateStatus(item.MemoryUsedPercent, worst.UsedPercent, item.Ready, warning, critical)
	if useFallback {
		msgs = append(msgs, "Prometheus 无该节点分区数据，已回退 kubelet 根/镜像文件系统")
	}
	item.Status = status
	item.Messages = msgs
	return item
}

// disksFromKubelet 当 node-exporter 无数据时，从 kubelet /stats/summary 构造根 fs 与镜像 fs 两个分区。
func disksFromKubelet(summary *KubeletSummary) []diskpkg.DiskUsage {
	disks := []diskpkg.DiskUsage{}
	if summary.Node.Fs.CapacityBytes > 0 {
		used := summary.Node.Fs.UsedBytes
		disks = append(disks, diskpkg.DiskUsage{
			Mountpoint:    "/",
			Device:        "kubelet-root-fs",
			Fstype:        "rootfs",
			CapacityBytes: summary.Node.Fs.CapacityBytes,
			UsedBytes:     used,
			AvailBytes:    summary.Node.Fs.AvailableBytes,
			UsedPercent:   pct(used, summary.Node.Fs.CapacityBytes),
		})
	}
	if summary.Node.Runtime.ImageFs.CapacityBytes > 0 {
		used := summary.Node.Runtime.ImageFs.UsedBytes
		disks = append(disks, diskpkg.DiskUsage{
			Mountpoint:    "/var/lib/docker",
			Device:        "kubelet-image-fs",
			Fstype:        "rootfs",
			CapacityBytes: summary.Node.Runtime.ImageFs.CapacityBytes,
			UsedBytes:     used,
			AvailBytes:    summary.Node.Runtime.ImageFs.AvailableBytes,
			UsedPercent:   pct(used, summary.Node.Runtime.ImageFs.CapacityBytes),
		})
	}
	return disks
}

// evaluateStatus 根据内存/磁盘使用率与 Ready 状态评级，返回等级与告警原因。
func evaluateStatus(memPct, diskPct float64, ready bool, warning, critical int) (string, []string) {
	msgs := []string{}
	level := StatusOK
	add := func(pct float64, what string) {
		if pct <= 0 {
			return
		}
		switch {
		case pct >= float64(critical):
			msgs = append(msgs, fmt.Sprintf("%s使用率 %.1f%% ≥ %d%%(严重)", what, pct, critical))
			level = StatusCritical
		case pct >= float64(warning):
			msgs = append(msgs, fmt.Sprintf("%s使用率 %.1f%% ≥ %d%%(告警)", what, pct, warning))
			if level != StatusCritical {
				level = StatusWarning
			}
		}
	}
	add(memPct, "内存")
	add(diskPct, "磁盘")
	if !ready {
		msgs = append(msgs, "节点 NotReady")
		if level == StatusOK {
			level = StatusWarning
		}
	}
	return level, msgs
}

// pct 计算使用率百分比（保留一位小数），分母为 0 时返回 0。
func pct(used, total int64) float64 {
	if total <= 0 {
		return 0
	}
	return round1(float64(used) / float64(total) * 100)
}

// round1 保留一位小数
func round1(f float64) float64 {
	return math.Round(f*10) / 10
}
