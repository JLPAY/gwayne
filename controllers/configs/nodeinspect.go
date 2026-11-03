package configs

import (
	"net/http"

	"github.com/JLPAY/gwayne/models"
	"github.com/gin-gonic/gin"
	"k8s.io/klog/v2"
)

// GetNodeInspectConfig 查询节点巡检阈值配置
// GET /api/v1/configs/node-inspect
func GetNodeInspectConfig(c *gin.Context) {
	cfg := models.GetNodeInspectConfig()
	c.JSON(http.StatusOK, gin.H{"data": cfg})
}

// UpdateNodeInspectConfig 更新节点巡检阈值配置（仅管理员）
// PUT /api/v1/configs/node-inspect  body: {"warning":80,"critical":90}
func UpdateNodeInspectConfig(c *gin.Context) {
	user, ok := c.MustGet("User").(*models.User)
	if !ok || user == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未识别到登录用户"})
		return
	}
	if !user.Admin {
		klog.Warningf("非管理员尝试修改节点巡检阈值被拒绝: user=%s ip=%s", user.Name, c.ClientIP())
		c.JSON(http.StatusForbidden, gin.H{"error": "仅管理员可修改节点巡检阈值"})
		return
	}

	var body struct {
		Warning  int `json:"warning"`
		Critical int `json:"critical"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数格式错误"})
		return
	}
	if body.Warning <= 0 || body.Warning >= 100 || body.Critical <= body.Warning || body.Critical > 100 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "阈值非法：需满足 0 < warning < critical <= 100"})
		return
	}

	cfg, err := models.SaveNodeInspectConfig(body.Warning, body.Critical)
	if err != nil {
		klog.Errorf("更新节点巡检阈值失败: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	klog.Infof("管理员 %s(ip=%s) 更新节点巡检阈值: warning=%d critical=%d", user.Name, c.ClientIP(), cfg.Warning, cfg.Critical)
	c.JSON(http.StatusOK, gin.H{"data": cfg})
}
