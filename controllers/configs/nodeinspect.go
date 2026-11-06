package configs

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/JLPAY/gwayne/models"
	"github.com/JLPAY/gwayne/pkg/kubernetes/client"
	"github.com/JLPAY/gwayne/pkg/notify"
	"github.com/robfig/cron/v3"
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
		Items               []models.NodeInspectItemConfig `json:"items"`
		ScheduleEnabled     bool                           `json:"scheduleEnabled"`
		ScheduleInterval    int                            `json:"scheduleInterval"`
		ScheduleCron        string                         `json:"scheduleCron"`
		ScheduleClusters    string                         `json:"scheduleClusters"`
		RetentionDays       int                            `json:"retentionDays"`
		WebhookEnabled      bool                           `json:"webhookEnabled"`
		WebhookURL          string                         `json:"webhookUrl"`
		WebhookOnlyWarning  bool                           `json:"webhookOnlyWarning"`
		PrometheusURL       string                         `json:"prometheusUrl"`
		PrometheusToken     string                         `json:"prometheusToken"`
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
		// 布尔型巡检项（Unit 为空，如压力状态）无阈值语义，跳过阈值比较
		if meta.Unit == "" {
			continue
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
	// crontab 表达式：非空时必须是合法的 5 段标准表达式（如 "0 */6 * * *"）
	scheduleCron := strings.TrimSpace(body.ScheduleCron)
	if scheduleCron != "" {
		if _, err := cron.ParseStandard(scheduleCron); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "非法 crontab 表达式 \"" + scheduleCron + "\": " + err.Error()})
			return
		}
	}
	// 定时巡检集群范围：名字必须已注册；规范化为逗号分隔（去空格、去重）
	scope := (&models.NodeInspectConfig{ScheduleClusters: body.ScheduleClusters}).ParseScheduleClusters()
	if len(scope) > 0 {
		registered := map[string]bool{}
		client.Managers().Range(func(k, v interface{}) bool {
			if name, ok := k.(string); ok {
				registered[name] = true
			}
			return true
		})
		for _, s := range scope {
			if !registered[s] {
				c.JSON(http.StatusBadRequest, gin.H{"error": "未注册的集群: " + s})
				return
			}
		}
	}
	scheduleClusters := strings.Join(scope, ",")
	webhookURL := strings.TrimSpace(body.WebhookURL)
	if body.WebhookEnabled {
		if webhookURL == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "已启用企微机器人推送，但 Webhook 地址为空"})
			return
		}
		if !strings.HasPrefix(webhookURL, "https://") && !strings.HasPrefix(webhookURL, "http://") {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Webhook 地址需以 http(s):// 开头"})
			return
		}
	}
	// Prometheus 数据源：非空时必须是合法 http(s) 地址
	promURL := strings.TrimSpace(body.PrometheusURL)
	if promURL != "" && !strings.HasPrefix(promURL, "https://") && !strings.HasPrefix(promURL, "http://") {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Prometheus 地址需以 http(s):// 开头"})
		return
	}

	cfg, err := models.SaveNodeInspectConfig(body.Items, &models.NodeInspectConfig{
		ScheduleEnabled:    body.ScheduleEnabled,
		ScheduleInterval:   body.ScheduleInterval,
		ScheduleCron:       scheduleCron,
		ScheduleClusters:   scheduleClusters,
		RetentionDays:      body.RetentionDays,
		WebhookEnabled:     body.WebhookEnabled,
		WebhookURL:         webhookURL,
		WebhookOnlyWarning: body.WebhookOnlyWarning,
		PrometheusURL:      promURL,
		PrometheusToken:    strings.TrimSpace(body.PrometheusToken),
	})
	if err != nil {
		klog.Errorf("更新节点巡检配置失败: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	klog.Infof("管理员 %s(ip=%s) 更新节点巡检配置: %d 项, 调度=%v cron=%q 间隔=%d分钟, 保留=%d天, 企微推送=%v, Prometheus=%q",
		user.Name, c.ClientIP(), len(body.Items), cfg.ScheduleEnabled, cfg.ScheduleCron, cfg.ScheduleInterval, cfg.RetentionDays, cfg.WebhookEnabled, promURL)
	c.JSON(http.StatusOK, gin.H{"data": cfg})
}

// TestNodeInspectWebhook 向企业微信机器人发送测试消息（仅管理员）
// POST /api/v1/configs/node-inspect/webhook-test
// body: {"webhookUrl":"..."}；为空时使用已保存配置中的地址。
func TestNodeInspectWebhook(c *gin.Context) {
	user, ok := c.MustGet("User").(*models.User)
	if !ok || user == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未识别到登录用户"})
		return
	}
	if !user.Admin {
		c.JSON(http.StatusForbidden, gin.H{"error": "仅管理员可发送测试消息"})
		return
	}
	var body struct {
		WebhookURL string `json:"webhookUrl"`
	}
	_ = c.ShouldBindJSON(&body)
	webhookURL := strings.TrimSpace(body.WebhookURL)
	if webhookURL == "" {
		webhookURL = models.GetNodeInspectConfig().WebhookURL
	}
	if webhookURL == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请先填写并保存 Webhook 地址"})
		return
	}
	content := fmt.Sprintf("## gwayne 巡检机器人\n> 这是一条<font color=\"info\">测试消息</font>，收到即表示 Webhook 配置成功。\n> 发送人：%s · %s",
		user.Name, time.Now().Format("2006-01-02 15:04:05"))
	if err := notify.SendWeComMarkdown(webhookURL, content); err != nil {
		klog.Errorf("发送企微测试消息失败: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	klog.Infof("管理员 %s(ip=%s) 发送企微巡检测试消息成功", user.Name, c.ClientIP())
	c.JSON(http.StatusOK, gin.H{"data": "测试消息已发送，请在企业微信群中确认"})
}
