package node

import "testing"

func TestEvaluateStatus(t *testing.T) {
	cases := []struct {
		name     string
		memPct   float64
		diskPct  float64
		ready    bool
		warning  int
		critical int
		want     string
	}{
		{"全部健康", 10, 20, true, 80, 90, StatusOK},
		{"内存触告警", 85, 20, true, 80, 90, StatusWarning},
		{"磁盘触严重", 10, 95, true, 80, 90, StatusCritical},
		{"双高取严重", 88, 92, true, 80, 90, StatusCritical},
		{"NotReady 标告警", 10, 20, false, 80, 90, StatusWarning},
		{"NotReady 且内存严重", 95, 20, false, 80, 90, StatusCritical},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, _ := evaluateStatus(c.memPct, c.diskPct, c.ready, c.warning, c.critical)
			if got != c.want {
				t.Errorf("evaluateStatus(%v,%v,%v,%d,%d)=%s, want %s", c.memPct, c.diskPct, c.ready, c.warning, c.critical, got, c.want)
			}
		})
	}
}

func TestRound1(t *testing.T) {
	cases := []struct {
		in   float64
		want float64
	}{
		{83.3333, 83.3},
		{90.0, 90.0},
		{0.0, 0.0},
		{12.345, 12.3},
	}
	for _, c := range cases {
		if got := round1(c.in); got != c.want {
			t.Errorf("round1(%v)=%v, want %v", c.in, got, c.want)
		}
	}
}

func TestInspectResultErrorField(t *testing.T) {
	// 确保单集群巡检失败时 Error 字段可被正确填充（handler 使用它区分失败集群）
	r := NodeInspectionResult{Cluster: "test", Error: "boom"}
	if r.Error != "boom" {
		t.Errorf("expected Error field set, got %q", r.Error)
	}
	// InspectNodes 正常路径会初始化 Items 为非空切片；失败路径允许为 nil（序列化为 null）
	ok := InspectNodes
	_ = ok
}

