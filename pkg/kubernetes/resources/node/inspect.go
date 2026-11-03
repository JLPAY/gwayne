package node

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"sync"
	"time"

	diskpkg "github.com/JLPAY/gwayne/pkg/monitor/disk"
	"github.com/JLPAY/gwayne/pkg/monitor"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"
)

// 节点健康等级
const (
	StatusOK       = "ok"
	StatusWarning  = "warning"
	StatusCritical = "critical"
)

// ItemReady 节点就绪（非可配置项，始终生效，用于直观标记 NotReady）
const ItemReady InspectItemType = "ready"

// KubeletSummary 解析 kubelet /stats/summary 返回的关键字段（仅保留巡检所需字段）。
type KubeletSummary struct {
	Node struct {
		NodeName string `json:"nodeName"`
		Memory   struct {
			WorkingSetBytes int64 `json:"workingSetBytes"`
		} `json:"memory"`
		Fs struct {
			CapacityBytes int64 `json:"capacityBytes"`
			UsedBytes     int64 `json:"usedBytes"`
		} `json:"fs"`
		Runtime struct {
			ImageFs struct {
				CapacityBytes int64 `json:"capacityBytes"`
				UsedBytes     int64 `json:"usedBytes"`
			} `json:"imageFs"`
		} `json:"runtime"`
	} `json:"node"`
}

// NodeInspectionItem 单个节点的巡检结果（按巡检项拆分到 Checks）
type NodeInspectionItem struct {
	Name     string        `json:"name"`
	Ready    bool          `json:"ready"`
	Checks   []CheckResult `json:"checks"`
	Status   string        `json:"status"`
	Messages []string      `json:"messages,omitempty"`
	Error    string        `json:"error,omitempty"`
}

// NodeInspectionResult 单个集群的巡检结果汇总
type NodeInspectionResult struct {
	Cluster       string            `json:"cluster"`
	Total         int               `json:"total"`
	Warning       int               `json:"warning"`
	Critical      int               `json:"critical"`
	Items         []NodeInspectionItem `json:"items"`
	ClusterChecks []CheckResult     `json:"clusterChecks,omitempty"`
	Error         string            `json:"error,omitempty"`
}

// clusterMetrics 巡检前一次性拉取的集群级指标（按节点名小写索引）
type clusterMetrics struct {
	diskMap    map[string][]diskpkg.DiskUsage
	cpuMap     map[string]float64 // 利用率 0..1
	loadMap    map[string]float64 // node_load1
	coresMap   map[string]float64 // CPU 核数
	fdAllocMap map[string]float64 // node_filefd_allocated
	fdMaxMap   map[string]float64 // node_filefd_maximum
	podCount   map[string]int     // 节点实际 Pod 数
	promFailed bool
}

// GetNodeStatsSummary 经 API server 代理调用 kubelet /stats/summary，拿到内存 workingSet。
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

// InspectNodes 巡检指定集群全部节点，覆盖内存/磁盘/CPU/负载/压力/FD/Pod/kubelet 证书等可配置项。
// promURL/promToken：Prometheus 数据源（CPU/负载/FD/磁盘分区）。restCfg：用于 API Server 证书巡检。
// plan：各巡检项生效阈值（已与默认值合并）。
func InspectNodes(ctx context.Context, clientset kubernetes.Interface, promURL, promToken string, restCfg *rest.Config, plan map[InspectItemType]ItemThreshold) (*NodeInspectionResult, error) {
	nodeList, err := clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("列举节点失败: %v", err)
	}
	result := &NodeInspectionResult{Total: len(nodeList.Items)}

	metrics, mErr := collectClusterMetrics(ctx, clientset, promURL, promToken, nodeList.Items)
	if mErr != nil {
		klog.Warningf("采集集群级指标部分失败（相关项将标记为不可用）: %v", mErr)
	}
	if metrics == nil {
		metrics = &clusterMetrics{diskMap: map[string][]diskpkg.DiskUsage{}, cpuMap: map[string]float64{}, loadMap: map[string]float64{}, coresMap: map[string]float64{}, fdAllocMap: map[string]float64{}, fdMaxMap: map[string]float64{}, podCount: map[string]int{}}
	}
	metrics.promFailed = (mErr != nil)

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
			item := inspectOneNode(ctx, clientset, n, plan, metrics)
			mu.Lock()
			result.Items = append(result.Items, item)
			mu.Unlock()
		}(n)
	}
	wg.Wait()

	for _, it := range result.Items {
		switch it.Status {
		case StatusCritical:
			result.Critical++
		case StatusWarning:
			result.Warning++
		}
	}

	// 集群级巡检项（API Server 证书）
	if th, ok := plan[ItemCertAPIServer]; ok && th.Enabled {
		result.ClusterChecks = append(result.ClusterChecks, inspectAPIServerCert(restCfg, th))
	}

	return result, nil
}

// collectClusterMetrics 一次性拉取 Prometheus 节点指标 + Pod 计数，按节点名索引。
func collectClusterMetrics(ctx context.Context, clientset kubernetes.Interface, promURL, promToken string, nodes []v1.Node) (*clusterMetrics, error) {
	m := &clusterMetrics{
		diskMap:    map[string][]diskpkg.DiskUsage{},
		cpuMap:     map[string]float64{},
		loadMap:    map[string]float64{},
		coresMap:   map[string]float64{},
		fdAllocMap: map[string]float64{},
		fdMaxMap:   map[string]float64{},
		podCount:   map[string]int{},
	}
	names := make([]string, 0, len(nodes))
	for i := range nodes {
		names = append(names, nodes[i].Name)
	}

	// 磁盘分区（node-exporter 全分区）
	diskMap, diskErr := diskpkg.GetNodeDiskUsage(promURL, promToken, names)
	if diskErr != nil {
		klog.Warningf("Prometheus 拉取磁盘分区失败: %v", diskErr)
	} else {
		m.diskMap = diskMap
	}

	queries := map[string]*map[string]float64{
		`1 - avg(rate(node_cpu_seconds_total{mode="idle"}[5m])) by (instance)`: &m.cpuMap,
		`node_load1`:          &m.loadMap,
		`count(node_cpu_seconds_total{mode="idle"}) by (instance)`: &m.coresMap,
		`node_filefd_allocated`: &m.fdAllocMap,
		`node_filefd_maximum`:   &m.fdMaxMap,
	}
	var firstErr error
	for q, dst := range queries {
		samples, e := monitor.InstantQuery(promURL, promToken, q)
		if e != nil {
			if firstErr == nil {
				firstErr = e
			}
			klog.Warningf("Prometheus 查询失败: %s -> %v", q, e)
			continue
		}
		for _, s := range samples {
			(*dst)[normInst(s.Metric["instance"])] = s.FloatValue
		}
	}
	if firstErr != nil {
		return m, firstErr
	}

	// Pod 计数（按 spec.nodeName）
	pods, err := clientset.CoreV1().Pods(metav1.NamespaceAll).List(ctx, metav1.ListOptions{})
	if err != nil {
		klog.Warningf("列举 Pod 失败（Pod 巡检项将不可用）: %v", err)
	} else {
		for i := range pods.Items {
			nn := pods.Items[i].Spec.NodeName
			if nn != "" {
				m.podCount[nn]++
			}
		}
	}
	return m, nil
}

// inspectOneNode 巡检单个节点，生成各项 CheckResult。
func inspectOneNode(ctx context.Context, clientset kubernetes.Interface, n *v1.Node, plan map[InspectItemType]ItemThreshold, m *clusterMetrics) NodeInspectionItem {
	item := NodeInspectionItem{Name: n.Name, Ready: isNodeReady(n)}
	ln := strings.ToLower(n.Name)

	checks := []CheckResult{}

	// 节点就绪（始终检查，非可配置）
	if item.Ready {
		checks = append(checks, CheckResult{Type: ItemReady, Name: "MENU.NODE_INSPECT.READY", Status: StatusOK, Value: "Ready"})
	} else {
		checks = append(checks, CheckResult{Type: ItemReady, Name: "MENU.NODE_INSPECT.READY", Status: StatusWarning, Value: "NotReady"})
	}

	// 拉取 kubelet 统计（内存 + 磁盘回退共用）
	summary, kubeletErr := GetNodeStatsSummary(clientset, n.Name)

	// 内存
	if th, ok := plan[ItemMemory]; ok && th.Enabled {
		if kubeletErr != nil {
			checks = append(checks, unavailableCheck(ItemMemory, "kubelet 统计拉取失败: "+kubeletErr.Error()))
		} else {
			var cap int64
			if mem := n.Status.Capacity.Memory(); mem != nil {
				cap = mem.Value()
			}
			used := summary.Node.Memory.WorkingSetBytes
			if cap > 0 {
				pct := float64(used) / float64(cap) * 100
				checks = append(checks, usageCheck(ItemMemory, pct, th, fmt.Sprintf("%.1f%%", pct), fmt.Sprintf("%s / %s", humanBytes(used), humanBytes(cap))))
			} else {
				checks = append(checks, unavailableCheck(ItemMemory, "节点内存容量为空"))
			}
		}
	}

	// 磁盘
	if th, ok := plan[ItemDisk]; ok && th.Enabled {
		disks := m.diskMap[ln]
		if len(disks) == 0 && kubeletErr == nil {
			disks = disksFromKubelet(summary)
		}
		if len(disks) == 0 {
			if m.promFailed {
				checks = append(checks, unavailableCheck(ItemDisk, "Prometheus 无分区数据且 kubelet 不可达"))
			} else {
				checks = append(checks, unavailableCheck(ItemDisk, "无分区数据"))
			}
		} else {
			worst := diskpkg.WorstDisk(disks)
			detail := fmt.Sprintf("最严重分区 %s %.1f%%", worst.Mountpoint, worst.UsedPercent)
			checks = append(checks, usageCheck(ItemDisk, worst.UsedPercent, th, fmt.Sprintf("%.1f%%", worst.UsedPercent), detail))
		}
	}

	// CPU
	if th, ok := plan[ItemCPU]; ok && th.Enabled {
		if m.promFailed {
			checks = append(checks, unavailableCheck(ItemCPU, "Prometheus 不可达"))
		} else if util, ok := m.cpuMap[ln]; ok {
			pct := util * 100
			checks = append(checks, usageCheck(ItemCPU, pct, th, fmt.Sprintf("%.1f%%", pct), ""))
		} else {
			checks = append(checks, unavailableCheck(ItemCPU, "Prometheus 无该节点 CPU 数据"))
		}
	}

	// 负载
	if th, ok := plan[ItemLoad]; ok && th.Enabled {
		if m.promFailed {
			checks = append(checks, unavailableCheck(ItemLoad, "Prometheus 不可达"))
		} else if load1, ok1 := m.loadMap[ln]; ok1 {
			cores := m.coresMap[ln]
			if cores > 0 {
				ratio := load1 / cores * 100
				checks = append(checks, usageCheck(ItemLoad, ratio, th, fmt.Sprintf("load1 %.2f / cores %.0f", load1, cores), ""))
			} else {
				checks = append(checks, usageCheck(ItemLoad, 0, th, fmt.Sprintf("load1 %.2f (cores 未知)", load1), ""))
			}
		} else {
			checks = append(checks, unavailableCheck(ItemLoad, "Prometheus 无该节点负载数据"))
		}
	}

	// 文件描述符
	if th, ok := plan[ItemFD]; ok && th.Enabled {
		if m.promFailed {
			checks = append(checks, unavailableCheck(ItemFD, "Prometheus 不可达"))
		} else if max, ok := m.fdMaxMap[ln]; ok && max > 0 {
			alloc := m.fdAllocMap[ln]
			pct := float64(alloc) / max * 100
			checks = append(checks, usageCheck(ItemFD, pct, th, fmt.Sprintf("%.1f%%", pct), fmt.Sprintf("%s / %s", humanBytes(int64(alloc)), humanBytes(int64(max)))))
		} else {
			checks = append(checks, unavailableCheck(ItemFD, "Prometheus 无该节点 FD 数据"))
		}
	}

	// Pod 数量
	if th, ok := plan[ItemPod]; ok && th.Enabled {
		used := m.podCount[n.Name]
		alloc := int64(0)
		if p := n.Status.Allocatable.Pods(); p != nil {
			alloc = p.Value()
		}
		if alloc > 0 {
			pct := float64(used) / float64(alloc) * 100
			checks = append(checks, usageCheck(ItemPod, pct, th, fmt.Sprintf("%d/%d", used, alloc), ""))
		} else {
			checks = append(checks, unavailableCheck(ItemPod, "节点可分配 Pod 数为空"))
		}
	}

	// 节点压力条件
	if th, ok := plan[ItemPressure]; ok && th.Enabled {
		var bad []string
		for _, c := range n.Status.Conditions {
			switch c.Type {
			case v1.NodeMemoryPressure, v1.NodeDiskPressure, v1.NodePIDPressure, v1.NodeNetworkUnavailable:
				if c.Status == v1.ConditionTrue {
					bad = append(bad, string(c.Type))
				}
			}
		}
		if len(bad) > 0 {
			checks = append(checks, CheckResult{Type: ItemPressure, Name: metaName(ItemPressure), Status: StatusWarning, Value: "异常", Detail: strings.Join(bad, "、") + " 为 True"})
		} else {
			checks = append(checks, CheckResult{Type: ItemPressure, Name: metaName(ItemPressure), Status: StatusOK, Value: "正常"})
		}
	}

	item.Checks = checks
	item.Status = worstStatusOfChecks(checks)
	return item
}

// worstStatusOfChecks 取所有检查中的最坏等级
func worstStatusOfChecks(checks []CheckResult) string {
	level := map[string]int{StatusOK: 0, StatusWarning: 1, StatusCritical: 2}
	cur := StatusOK
	for _, c := range checks {
		if level[c.Status] > level[cur] {
			cur = c.Status
		}
	}
	return cur
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
			AvailBytes:    summary.Node.Fs.CapacityBytes - used,
			UsedPercent:   round1(float64(used) / float64(summary.Node.Fs.CapacityBytes) * 100),
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
			AvailBytes:    summary.Node.Runtime.ImageFs.CapacityBytes - used,
			UsedPercent:   round1(float64(used) / float64(summary.Node.Runtime.ImageFs.CapacityBytes) * 100),
		})
	}
	return disks
}

// isNodeReady 判断节点是否 Ready
func isNodeReady(n *v1.Node) bool {
	for _, c := range n.Status.Conditions {
		if c.Type == v1.NodeReady {
			return c.Status == v1.ConditionTrue
		}
	}
	return false
}

// normInst 归一化 instance（去 :port、转小写）以便与节点名匹配
func normInst(inst string) string {
	return strings.ToLower(strings.Split(inst, ":")[0])
}

// metaName 取巡检项 i18n key
func metaName(t InspectItemType) string {
	for _, it := range ItemRegistry {
		if it.Type == t {
			return it.Name
		}
	}
	return string(t)
}

// usageCheck 使用率型项目的评级（值越大越危险）
func usageCheck(t InspectItemType, percent float64, th ItemThreshold, value, detail string) CheckResult {
	status := StatusOK
	if th.Warning > 0 && percent >= th.Warning {
		status = StatusWarning
	}
	if th.Critical > 0 && percent >= th.Critical {
		status = StatusCritical
	}
	return CheckResult{Type: t, Name: metaName(t), Status: status, Percent: round1(percent), Value: value, Detail: detail}
}

// invertCheck 阈值语义反转的项目（如证书剩余天数，值越小越危险）
func invertCheck(t InspectItemType, daysLeft float64, th ItemThreshold, value, detail string) CheckResult {
	status := StatusOK
	if th.Warning > 0 && daysLeft <= th.Warning {
		status = StatusWarning
	}
	if th.Critical > 0 && daysLeft <= th.Critical {
		status = StatusCritical
	}
	return CheckResult{Type: t, Name: metaName(t), Status: status, Percent: round1(daysLeft), Value: value, Detail: detail}
}

// unavailableCheck 数据不可用时统一标记为 warning
func unavailableCheck(t InspectItemType, detail string) CheckResult {
	return CheckResult{Type: t, Name: metaName(t), Status: StatusWarning, Value: "不可用", Detail: detail}
}

// daysBetween 返回 b 相对 now 的剩余天数（可为负）
func daysBetween(now, b time.Time) float64 {
	return math.Round(b.Sub(now).Hours()/24*10) / 10
}

// humanBytes 人类可读字节
func humanBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}

// inspectAPIServerCert 集群级 API Server 证书巡检
func inspectAPIServerCert(restCfg *rest.Config, th ItemThreshold) CheckResult {
	if restCfg == nil || restCfg.Host == "" {
		return invertCheck(ItemCertAPIServer, 9999, th, "未知", "无法获取 API Server 地址")
	}
	host := APIServerHostPort(restCfg.Host)
	na, subj, err := GetCertExpiry(host)
	if err != nil {
		return invertCheck(ItemCertAPIServer, 9999, th, "获取失败", err.Error())
	}
	days := daysBetween(time.Now(), na)
	var value string
	if days >= 0 {
		value = fmt.Sprintf("还有 %.0f 天", days)
	} else {
		value = fmt.Sprintf("已过期 %.0f 天", -days)
	}
	detail := fmt.Sprintf("subject=%s, NotAfter=%s", subj, na.Format("2006-01-02 15:04:05"))
	return invertCheck(ItemCertAPIServer, days, th, value, detail)
}

// round1 保留一位小数
func round1(f float64) float64 {
	return math.Round(f*10) / 10
}
