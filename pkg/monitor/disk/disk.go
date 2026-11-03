package disk

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/JLPAY/gwayne/pkg/monitor"
)

// DiskUsage 单个分区的使用率（来自 node-exporter 的 node_filesystem_*）。
type DiskUsage struct {
	Mountpoint    string  `json:"mountpoint"`
	Device        string  `json:"device"`
	Fstype        string  `json:"fstype"`
	CapacityBytes int64   `json:"capacityBytes"`
	UsedBytes     int64   `json:"usedBytes"`
	AvailBytes    int64   `json:"availBytes"`
	UsedPercent   float64 `json:"usedPercent"`
}

// fsExcludeFstypes 过滤掉虚拟/容器层文件系统，只保留真实磁盘分区。
var fsExcludeFstypes = []string{
	"tmpfs", "devtmpfs", "proc", "sysfs", "cgroup", "cgroup2", "debugfs",
	"tracefs", "securityfs", "pstore", "bpf", "configfs", "fusectl",
	"hugetlbfs", "ramfs", "autofs", "binfmt_misc", "mqueue", "rpc_pipefs",
	"overlay", "squashfs", "rootfs",
}
var fsExcludeRegex = strings.Join(fsExcludeFstypes, "|")

// GetNodeDiskUsage 通过 Prometheus node-exporter 的 node_filesystem_size_bytes / avail_bytes，
// 一次性拉取所有节点的全部真实分区使用率，按节点名聚合。
// instance 标签通常为节点名（可能带 :port），与 k8s Node.Name 对应。
func GetNodeDiskUsage(promURL, promToken string, nodeNames []string) (map[string][]DiskUsage, error) {
	if len(nodeNames) == 0 {
		return map[string][]DiskUsage{}, nil
	}
	regex := buildInstanceRegex(nodeNames)
	selector := fmt.Sprintf(`{instance=~"%s",fstype!~"%s"}`, regex, fsExcludeRegex)
	sizeQ := `node_filesystem_size_bytes` + selector
	availQ := `node_filesystem_avail_bytes` + selector

	sizeSamples, err := monitor.InstantQuery(promURL, promToken, sizeQ)
	if err != nil {
		return nil, err
	}
	availSamples, err := monitor.InstantQuery(promURL, promToken, availQ)
	if err != nil {
		return nil, err
	}

	type key struct{ inst, mp string }
	sizeMap := map[key]int64{}
	availMap := map[key]int64{}
	meta := map[key]struct{ device, fstype string }{}
	for _, s := range sizeSamples {
		inst := normalizeInstance(s.Metric["instance"])
		k := key{inst, s.Metric["mountpoint"]}
		sizeMap[k] = int64(s.FloatValue)
		meta[k] = struct{ device, fstype string }{s.Metric["device"], s.Metric["fstype"]}
	}
	for _, s := range availSamples {
		inst := normalizeInstance(s.Metric["instance"])
		k := key{inst, s.Metric["mountpoint"]}
		availMap[k] = int64(s.FloatValue)
		m := meta[k]
		if m.device == "" {
			m.device = s.Metric["device"]
		}
		if m.fstype == "" {
			m.fstype = s.Metric["fstype"]
		}
		meta[k] = m
	}

	nameMap := map[string]string{}
	for _, n := range nodeNames {
		nameMap[strings.ToLower(n)] = n
	}

	result := map[string][]DiskUsage{}
	for k, size := range sizeMap {
		nodeName, ok := nameMap[strings.ToLower(k.inst)]
		if !ok {
			continue
		}
		avail := availMap[k]
		used := size - avail
		if used < 0 {
			used = 0
		}
		result[nodeName] = append(result[nodeName], DiskUsage{
			Mountpoint:    k.mp,
			Device:        meta[k].device,
			Fstype:        meta[k].fstype,
			CapacityBytes: size,
			UsedBytes:     used,
			AvailBytes:    avail,
			UsedPercent:   pct(used, size),
		})
	}

	// 按设备去重（保留挂载点最短的真实挂载，剔除 bind 挂载重复项），再按使用率降序
	for node, disks := range result {
		result[node] = dedupeAndSort(disks)
	}
	return result, nil
}

// WorstDisk 返回使用率最高的分区（仅统计有效容量>0 的分区）。
func WorstDisk(disks []DiskUsage) DiskUsage {
	var worst DiskUsage
	for _, d := range disks {
		if d.CapacityBytes <= 0 {
			continue
		}
		if d.UsedPercent > worst.UsedPercent {
			worst = d
		}
	}
	return worst
}

// dedupeAndSort 按设备去重（保留挂载点最短者），并按使用率降序排列。
func dedupeAndSort(disks []DiskUsage) []DiskUsage {
	best := map[string]DiskUsage{}
	for _, d := range disks {
		dev := d.Device
		if dev == "" {
			dev = d.Mountpoint
		}
		if existing, ok := best[dev]; !ok || len(d.Mountpoint) < len(existing.Mountpoint) {
			best[dev] = d
		}
	}
	out := make([]DiskUsage, 0, len(best))
	for _, d := range best {
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].UsedPercent > out[j].UsedPercent
	})
	return out
}

// buildInstanceRegex 将节点名列表拼成 Prometheus 正则（^name1|name2$），并转义点号。
func buildInstanceRegex(nodeNames []string) string {
	repl := strings.NewReplacer(".", `\.`)
	parts := make([]string, 0, len(nodeNames))
	for _, n := range nodeNames {
		parts = append(parts, repl.Replace(n))
	}
	return "^(" + strings.Join(parts, "|") + ")$"
}

// normalizeInstance 去掉 instance 可能携带的 :port 后缀并转小写，便于与节点名匹配。
func normalizeInstance(inst string) string {
	inst = strings.ToLower(inst)
	if i := strings.Index(inst, ":"); i != -1 {
		inst = inst[:i]
	}
	return inst
}

// pct 计算使用率百分比（保留一位小数），分母为 0 时返回 0。
func pct(used, total int64) float64 {
	if total <= 0 {
		return 0
	}
	return round1(float64(used) / float64(total) * 100)
}

func round1(f float64) float64 {
	return math.Round(f*10) / 10
}
