package configs

import (
	"net/http"

	"github.com/JLPAY/gwayne/models"
	"github.com/JLPAY/gwayne/pkg/rsakey"
	"github.com/gin-gonic/gin"
	"k8s.io/klog/v2"
)

// GetRsaKey 查询 JWT 签名密钥状态（仅返回 kid 指纹，不含任何私钥材料）
// GET /api/v1/configs/rsa-key
func GetRsaKey(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"data": rsakey.GetRsaKeyInfo()})
}

// RotateRsaKey 手动轮换 JWT 签名密钥（仅管理员）：
// 旧 current 降级为 previous，在宽限窗口（≥ TokenLifeTime）内存量令牌仍可验签，
// 不会导致在线用户掉线；其他副本通过惰性同步自动感知，无需重启。
// POST /api/v1/configs/rsa-key/rotate
func RotateRsaKey(c *gin.Context) {
	user, ok := c.MustGet("User").(*models.User)
	if !ok || user == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未识别到登录用户"})
		return
	}
	if !user.Admin {
		klog.Warningf("非管理员尝试轮换签名密钥被拒绝: user=%s ip=%s", user.Name, c.ClientIP())
		c.JSON(http.StatusForbidden, gin.H{"error": "仅管理员可执行签名密钥轮换"})
		return
	}

	info, err := rsakey.Rotate("管理员手动轮换")
	if err != nil {
		klog.Errorf("管理员 %s 轮换签名密钥失败: %s", user.Name, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 审计日志：轮换属于高危操作，记录操作人与来源
	klog.Infof("管理员 %s(ip=%s) 手动轮换 JWT 签名密钥: 新 kid=%s 旧 kid 进入宽限期", user.Name, c.ClientIP(), info.CurrentKid)
	c.JSON(http.StatusOK, gin.H{"data": info})
}
