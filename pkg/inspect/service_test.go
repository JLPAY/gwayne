package inspect

import (
	"testing"

	"github.com/JLPAY/gwayne/models"
	k8snode "github.com/JLPAY/gwayne/pkg/kubernetes/resources/node"
)

func TestBuildPlanDefaults(t *testing.T) {
	plan := BuildPlan(nil)
	if len(plan) != len(k8snode.ItemRegistry) {
		t.Fatalf("plan 项数应等于注册表 %d，实际 %d", len(k8snode.ItemRegistry), len(plan))
	}
	th, ok := plan[k8snode.ItemMemory]
	if !ok || !th.Enabled || th.Warning != 80 || th.Critical != 90 {
		t.Errorf("memory 默认值错误: %+v", th)
	}
	cert, ok := plan[k8snode.ItemCertAPIServer]
	if !ok || !(cert.Warning > cert.Critical) {
		t.Errorf("API Server 证书默认阈值应为 warning(30) > critical(7): %+v", cert)
	}
}

func TestBuildPlanOverride(t *testing.T) {
	cfg := &models.NodeInspectConfig{
		Items: `[{"type":"memory","enabled":false,"warning":70,"critical":85},
		         {"type":"cert_apiserver","enabled":true,"warning":60,"critical":14}]`,
	}
	plan := BuildPlan(cfg)
	m := plan[k8snode.ItemMemory]
	if m.Enabled || m.Warning != 70 || m.Critical != 85 {
		t.Errorf("memory 覆盖失败: %+v", m)
	}
	c := plan[k8snode.ItemCertAPIServer]
	if !c.Enabled || c.Warning != 60 || c.Critical != 14 {
		t.Errorf("cert_apiserver 覆盖失败: %+v", c)
	}
	// 未覆盖的项保持默认
	d := plan[k8snode.ItemDisk]
	if !d.Enabled || d.Warning != 80 {
		t.Errorf("disk 应保持默认: %+v", d)
	}
	// 未知类型应被忽略
	cfg2 := &models.NodeInspectConfig{Items: `[{"type":"not_exist","enabled":true,"warning":1,"critical":2}]`}
	plan2 := BuildPlan(cfg2)
	if len(plan2) != len(k8snode.ItemRegistry) {
		t.Errorf("未知巡检项应被忽略")
	}
}
