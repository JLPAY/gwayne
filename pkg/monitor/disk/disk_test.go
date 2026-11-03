package disk

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
)

// 模拟 Prometheus 返回的原始样本（size/avail 共用，便于按查询里的 fstype 排除列表过滤）。
var mockSamples = []struct {
	metric map[string]string
	size   string
	avail  string
}{
	{map[string]string{"instance": "node-a", "mountpoint": "/", "fstype": "xfs", "device": "/dev/sda1"}, "100", "10"},
	{map[string]string{"instance": "node-a", "mountpoint": "/data", "fstype": "xfs", "device": "/dev/sdb1"}, "200", "20"},
	{map[string]string{"instance": "node-a", "mountpoint": "/run", "fstype": "tmpfs", "device": "tmpfs"}, "50", "45"},
	{map[string]string{"instance": "node-a", "mountpoint": "/data/sub", "fstype": "xfs", "device": "/dev/sdb1"}, "200", "20"},
	{map[string]string{"instance": "node-b:9100", "mountpoint": "/", "fstype": "ext4", "device": "/dev/vda1"}, "100", "5"},
}

// writeMock 按查询里的 fstype!~ 排除列表过滤样本后写出（模拟 Prometheus 服务端过滤）。
func writeMock(w http.ResponseWriter, q, kind string) {
	exclude := ""
	if m := regexp.MustCompile(`fstype!~"([^"]+)"`).FindStringSubmatch(q); m != nil {
		exclude = m[1]
	}
	var sb strings.Builder
	sb.WriteString(`{"status":"success","data":{"resultType":"vector","result":[`)
	first := true
	keys := []string{"instance", "mountpoint", "fstype", "device"}
	for _, s := range mockSamples {
		skipSample := false
		if exclude != "" {
			for _, ex := range strings.Split(exclude, "|") {
				if s.metric["fstype"] == ex {
					skipSample = true
					break
				}
			}
		}
		if skipSample {
			continue
		}
		val := s.size
		if kind == "avail" {
			val = s.avail
		}
		if !first {
			sb.WriteString(",")
		}
		first = false
		sb.WriteString(`{"metric":{`)
		for i, k := range keys {
			if i > 0 {
				sb.WriteString(",")
			}
			sb.WriteString(`"` + k + `":"` + s.metric[k] + `"`)
		}
		sb.WriteString(`},"value":[1,"` + val + `"]}`)
	}
	sb.WriteString(`]}}`)
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(sb.String()))
}

// TestGetNodeDiskUsage 通过 httptest 模拟 Prometheus，验证：
// 1) size/avail 按 (instance,mountpoint) 正确聚合计算使用率；
// 2) tmpfs 等虚拟文件系统被过滤；
// 3) 同设备 bind 挂载去重（保留挂载点最短者）；
// 4) instance 带 :port 时仍能匹配节点名；
// 5) 结果按使用率降序。
func TestGetNodeDiskUsage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("query")
		if strings.Contains(q, "avail_bytes") {
			writeMock(w, q, "avail")
			return
		}
		writeMock(w, q, "size")
	}))
	defer srv.Close()

	got, err := GetNodeDiskUsage(srv.URL, "", []string{"node-a", "node-b"})
	if err != nil {
		t.Fatalf("GetNodeDiskUsage error: %v", err)
	}

	// node-a 应有 2 个分区（/ 与 /data；/run 的 tmpfs 被过滤；/data/sub 与 /data 同设备去重）
	a := got["node-a"]
	if len(a) != 2 {
		t.Fatalf("node-a 期望 2 个分区，实际 %d: %+v", len(a), a)
	}
	if a[0].Mountpoint != "/" || a[1].Mountpoint != "/data" {
		t.Fatalf("node-a 分区未按使用率降序或未去重: %+v", a)
	}
	// / 100-10=90 -> 90%；/data 200-20=180 -> 90%
	if a[0].UsedPercent != 90 || a[1].UsedPercent != 90 {
		t.Fatalf("node-a 使用率计算错误: %+v", a)
	}
	if a[0].Device != "/dev/sda1" || a[1].Device != "/dev/sdb1" {
		t.Fatalf("node-a 设备信息错误: %+v", a)
	}

	// node-b: instance 带 :9100，仍应匹配到 node-b，且 ext4 未被过滤
	b := got["node-b"]
	if len(b) != 1 {
		t.Fatalf("node-b 期望 1 个分区，实际 %d: %+v", len(b), b)
	}
	if b[0].UsedPercent != 95 { // 100-5=95
		t.Fatalf("node-b 使用率应为 95，实际 %v", b[0].UsedPercent)
	}
}

// TestGetNodeDiskUsagePromError 验证 Prometheus 不可用时返回错误（触发 kubelet 回退）。
func TestGetNodeDiskUsagePromError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	_, err := GetNodeDiskUsage(srv.URL, "", []string{"node-a"})
	if err == nil {
		t.Fatal("期望 Prometheus 返回错误，实际 nil")
	}
}

// TestWorstDisk 验证取使用率最高的分区。
func TestWorstDisk(t *testing.T) {
	disks := []DiskUsage{
		{Mountpoint: "/", UsedPercent: 30, CapacityBytes: 100},
		{Mountpoint: "/data", UsedPercent: 85, CapacityBytes: 200},
		{Mountpoint: "/mnt", UsedPercent: 0, CapacityBytes: 0}, // 容量 0 忽略
	}
	w := WorstDisk(disks)
	if w.Mountpoint != "/data" || w.UsedPercent != 85 {
		t.Fatalf("WorstDisk 错误: %+v", w)
	}
}

// TestDedupeAndSort 验证同设备 bind 挂载去重与降序。
func TestDedupeAndSort(t *testing.T) {
	in := []DiskUsage{
		{Device: "/dev/sdb1", Mountpoint: "/data/sub", UsedPercent: 10, CapacityBytes: 100},
		{Device: "/dev/sdb1", Mountpoint: "/data", UsedPercent: 80, CapacityBytes: 100},
		{Device: "/dev/sda1", Mountpoint: "/", UsedPercent: 50, CapacityBytes: 100},
	}
	out := dedupeAndSort(in)
	if len(out) != 2 {
		t.Fatalf("期望去重为 2 个，实际 %d: %+v", len(out), out)
	}
	// /dev/sdb1 保留 /data（最短），且按使用率降序：/data(80) 在前，/(50) 在后
	if out[0].Mountpoint != "/data" || out[1].Mountpoint != "/" {
		t.Fatalf("去重/排序错误: %+v", out)
	}
}
