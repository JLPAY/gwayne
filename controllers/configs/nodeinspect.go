package configs

import (
	"net/http"

	"github.com/JLPAY/gwayne/models"
	k8snode "github.com/JLPAY/gwayne/pkg/kubernetes/resources/node"
	"github.com/gin-gonic/gin"
	"k8s.io/klog/v2"
)

// GetNodeInspectConfig 查询节点巡检配置（含可配置项注册表，供前端动态渲染表单）
// GET /api/v1/configs/node-inspect
func GetNodeInspectConfig(c *gin.Context) {
	cfg := models.GetNodeInspectConfig()
	// items 为空时回退注册表默认值，保证前端首次打开即有完整可编辑列表
	items := cfg.ParseItems()
	if len(items) == 0 {
		for _, meta := range k8snode.ItemRegistry {
			items = append(items, models.NodeInspectItemConfig{
				Type:     string(meta.Type),
				Enabled:  meta.EnabledByDefault,
				Warning:  meta.DefaultWarning,
				Critical: meta.DefaultCritical,
			})
		}
	}
	c.JSON(http.StatusOK, gin.H{
		"data":     cfg,
		"items":    items,
		"registry": k8snode.ItemRegistry,
	})
}

// UpdateNodeInspectConfig 更新节点巡检配置（仅管理员）
// PUT /api/v1/configs/node-inspect
// body: {"items":[{"type":"memory","enabled":true,"warning":80,"critical":90},...],
//        "scheduleEnabled":true,"scheduleInterval":60,"retentionDays":90}
func UpdateNodeInspectConfig(c *gin.Context) {
	user, ok := c.MustGet("User").(*models.User)
	if !ok || user == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未识别到登录用户"})
		return
	}
	if !user.Admin {
		klog.Warningf("非管理员尝试修改节点巡检配置被拒绝: user=%s ip=%s", user.Name, c.ClientIP())
		c.JSON(http.StatusForbidden, gin.H{"error": "仅管理员可修改节点巡检配置"})
		return
	}

	var body struct {
		Items            []models.NodeInspectItemConfig `json:"items"`
		ScheduleEnabled  bool                           `json:"scheduleEnabled"`
		ScheduleInterval int                            `json:"scheduleInterval"`
		RetentionDays    int                            `json:"retentionDays"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数格式错误"})
		return
	}

	// 校验巡检项：类型必须在注册表中；阈值非负；使用率型 warning<critical，证书型 critical<warning
	known := map[string]k8snode.ItemMeta{}
	for _, meta := range k8snode.ItemRegistry {
		known[string(meta.Type)] = meta
	}
	seen := map[string]bool{}
	for _, it := range body.Items {
		meta, ok := known[it.Type]
		if !ok {
			c.JSON(http.StatusBadRequest, gin.H{"error": "未知巡检项类型: " + it.Type})
			return
		}
		if seen[it.Type] {
			c.JSON(http.StatusBadRequest, gin.H{"error": "巡检项类型重复: " + it.Type})
			return
		}
		seen[it.Type] = true
		if it.Warning < 0 || it.Critical < 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": it.Type + " 阈值不能为负"})
			return
		}
		if !meta.Invert && it.Warning >= it.Critical {
			c.JSON(http.StatusBadRequest, gin.H{"error": it.Type + " 需满足 warning < critical"})
			return
		}
		if meta.Invert && it.Critical >= it.Warning {
			c.JSON(http.StatusBadRequest, gin.H{"error": it.Type + " 为证书剩余天数，需满足 critical(天数) < warning(天数)"})
			return
		}
	}
	if body.ScheduleInterval < 0 || body.RetentionDays < 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "调度间隔与保留天数不能为负"})
		return
	}

	cfg, err := models.SaveNodeInspectConfig(body.Items, body.ScheduleEnabled, body.ScheduleInterval, body.RetentionDays)
	if err != nil {
		klog.Errorf("更新节点巡检配置失败: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	klog.Infof("管理员 %s(ip=%s) 更新节点巡检配置: %d 项, 调度=%v/%d分钟, 保留=%d天",
		user.Name, c.ClientIP(), len(body.Items), cfg.ScheduleEnabled, cfg.ScheduleInterval, cfg.RetentionDays)
	c.JSON(http.StatusOK, gin.H{"data": cfg})
}
