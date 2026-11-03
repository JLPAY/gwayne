package node

import (
	"testing"
	"time"
)

func TestUsageCheck(t *testing.T) {
	th := ItemThreshold{Enabled: true, Warning: 80, Critical: 90}
	c := usageCheck(ItemMemory, 85, th, "85%", "")
	if c.Status != StatusWarning {
		t.Errorf("85%% 应为 warning，实际 %s", c.Status)
	}
	c = usageCheck(ItemMemory, 95, th, "95%", "")
	if c.Status != StatusCritical {
		t.Errorf("95%% 应为 critical，实际 %s", c.Status)
	}
	c = usageCheck(ItemMemory, 50, th, "50%", "")
	if c.Status != StatusOK {
		t.Errorf("50%% 应为 ok，实际 %s", c.Status)
	}
}

func TestInvertCheck(t *testing.T) {
	th := ItemThreshold{Enabled: true, Warning: 30, Critical: 7}
	// 剩余天数越少越危险
	c := invertCheck(ItemCertAPIServer, 5, th, "还有 5 天", "")
	if c.Status != StatusCritical {
		t.Errorf("剩余5天应为 critical，实际 %s", c.Status)
	}
	c = invertCheck(ItemCertAPIServer, 20, th, "还有 20 天", "")
	if c.Status != StatusWarning {
		t.Errorf("剩余20天应为 warning，实际 %s", c.Status)
	}
	c = invertCheck(ItemCertAPIServer, 100, th, "还有 100 天", "")
	if c.Status != StatusOK {
		t.Errorf("剩余100天应为 ok，实际 %s", c.Status)
	}
}

func TestWorstStatusOfChecks(t *testing.T) {
	checks := []CheckResult{
		{Status: StatusOK},
		{Status: StatusWarning},
		{Status: StatusOK},
	}
	if s := worstStatusOfChecks(checks); s != StatusWarning {
		t.Errorf("应为 warning，实际 %s", s)
	}
	checks = append(checks, CheckResult{Status: StatusCritical})
	if s := worstStatusOfChecks(checks); s != StatusCritical {
		t.Errorf("应为 critical，实际 %s", s)
	}
}

func TestNormInst(t *testing.T) {
	if got := normInst("NodeA:9100"); got != "nodea" {
		t.Errorf("normInst(nodea:9100) 应为 nodea，实际 %s", got)
	}
	if got := normInst("NodeB"); got != "nodeb" {
		t.Errorf("normInst(NodeB) 应为 nodeb，实际 %s", got)
	}
}

func TestDaysBetween(t *testing.T) {
	now := time.Now()
	got := daysBetween(now, now.AddDate(0, 0, 23))
	if got != 23 {
		t.Errorf("剩余 23 天，实际 %.0f", got)
	}
}

func TestInspectResultErrorField(t *testing.T) {
	// 确保单集群巡检失败时 Error 字段可被正确填充（handler 使用它区分失败集群）
	r := NodeInspectionResult{Cluster: "test", Error: "boom"}
	if r.Error != "boom" {
		t.Errorf("expected Error field set, got %q", r.Error)
	}
}
