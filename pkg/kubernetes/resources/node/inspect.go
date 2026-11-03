package node

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sync"

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
	Name                string   `json:"name"`
	Ready               bool     `json:"ready"`
	MemoryCapacityBytes int64    `json:"memoryCapacityBytes"`
	MemoryUsedBytes     int64    `json:"memoryUsedBytes"`
	MemoryUsedPercent   float64  `json:"memoryUsedPercent"`
	DiskCapacityBytes   int64    `json:"diskCapacityBytes"`
	DiskUsedBytes       int64    `json:"diskUsedBytes"`
	DiskUsedPercent     float64  `json:"diskUsedPercent"`
	DiskAvailableBytes  int64    `json:"diskAvailableBytes"`
	Status              string   `json:"status"`
	Messages            []string `json:"messages,omitempty"`
	Error               string   `json:"error,omitempty"`
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

// GetNodeStatsSummary 经 API server 代理调用 kubelet /stats/summary，
// 一次调用同时拿到内存 workingSet 与根 fs 的用量，无需依赖 metrics-server。
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
// warning/critical 为百分比阈值，决定 ok/warning/critical 等级。
func InspectNodes(ctx context.Context, clientset kubernetes.Interface, warning, critical int) (*NodeInspectionResult, error) {
	nodeList, err := clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("列举节点失败: %v", err)
	}

	result := &NodeInspectionResult{Total: len(nodeList.Items)}
	items := make([]NodeInspectionItem, 0, len(nodeList.Items))

	// 并发拉取 kubelet 统计，限制并发避免瞬时打爆 kubelet
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

			item := inspectOneNode(ctx, clientset, n, warning, critical)
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
func inspectOneNode(ctx context.Context, clientset kubernetes.Interface, n *v1.Node, warning, critical int) NodeInspectionItem {
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

	// 拉取 kubelet 统计；失败则标记为告警并说明原因，便于发现不可达节点
	summary, err := GetNodeStatsSummary(clientset, n.Name)
	if err != nil {
		item.Error = err.Error()
		item.Status = StatusWarning
		item.Messages = []string{"kubelet 统计拉取失败，无法评估使用率"}
		klog.Warningf("节点 %s 巡检拉取 kubelet 统计失败: %v", n.Name, err)
		return item
	}

	// 内存使用率（workingSet 才是压力口径，而非含缓存的 usageBytes）
	memUsed := summary.Node.Memory.WorkingSetBytes
	item.MemoryUsedBytes = memUsed
	if item.MemoryCapacityBytes > 0 {
		item.MemoryUsedPercent = round1(float64(memUsed) / float64(item.MemoryCapacityBytes) * 100)
	}

	// 磁盘使用率（node.fs 为节点根文件系统；capacity=0 时回退到 ephemeral-storage）
	diskCap := summary.Node.Fs.CapacityBytes
	if diskCap <= 0 {
		if epi := n.Status.Capacity[v1.ResourceEphemeralStorage]; !epi.IsZero() {
			diskCap = epi.Value()
		}
	}
	diskUsed := summary.Node.Fs.UsedBytes
	item.DiskCapacityBytes = diskCap
	item.DiskUsedBytes = diskUsed
	item.DiskAvailableBytes = summary.Node.Fs.AvailableBytes
	if diskCap > 0 {
		item.DiskUsedPercent = round1(float64(diskUsed) / float64(diskCap) * 100)
	}

	status, msgs := evaluateStatus(item.MemoryUsedPercent, item.DiskUsedPercent, item.Ready, warning, critical)
	item.Status = status
	item.Messages = msgs
	return item
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

// round1 保留一位小数
func round1(f float64) float64 {
	return math.Round(f*10) / 10
}
