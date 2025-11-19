package k8sgpt

import (
	"net/http"

	"github.com/JLPAY/gwayne/pkg/k8sgpt"
	"github.com/gin-gonic/gin"
	"k8s.io/klog/v2"
)

// ListProviders 列出所有 AI 提供者
// @router /api/v1/k8sgpt/ai/providers [get]
func ListProviders(c *gin.Context) {
	manager := k8sgpt.GetAIConfigManager()
	config, err := manager.ListProviders()
	if err != nil {
		klog.Errorf("Failed to list AI providers: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data": config,
	})
}

// AddProvider 添加 AI 提供者
// @router /api/v1/k8sgpt/ai/providers [post]
func AddProvider(c *gin.Context) {
	var provider k8sgpt.AIProviderConfig
	if err := c.ShouldBindJSON(&provider); err != nil {
		klog.Errorf("Failed to bind provider config: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{
			"error": err.Error(),
		})
		return
	}

	// 验证配置
	if err := provider.ValidateProvider(); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": err.Error(),
		})
		return
	}

	manager := k8sgpt.GetAIConfigManager()
	if err := manager.AddProvider(provider); err != nil {
		klog.Errorf("Failed to add AI provider: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "AI provider added successfully",
		"data":    provider,
	})
}

// RemoveProvider 删除 AI 提供者
// @router /api/v1/k8sgpt/ai/providers/:name [delete]
func RemoveProvider(c *gin.Context) {
	name := c.Param("name")
	if name == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "provider name is required",
		})
		return
	}

	manager := k8sgpt.GetAIConfigManager()
	if err := manager.RemoveProvider(name); err != nil {
		klog.Errorf("Failed to remove AI provider: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "AI provider removed successfully",
	})
}

// SetDefaultProvider 设置默认 AI 提供者
// @router /api/v1/k8sgpt/ai/providers/:name/default [put]
func SetDefaultProvider(c *gin.Context) {
	name := c.Param("name")
	if name == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "provider name is required",
		})
		return
	}

	manager := k8sgpt.GetAIConfigManager()
	if err := manager.SetDefaultProvider(name); err != nil {
		klog.Errorf("Failed to set default AI provider: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Default AI provider set successfully",
		"data":    name,
	})
}

// GetAvailableBackends 获取可用的 AI 后端列表
// @router /api/v1/k8sgpt/ai/backends [get]
func GetAvailableBackends(c *gin.Context) {
	manager := k8sgpt.GetAIConfigManager()
	backends := manager.GetAvailableBackends()

	c.JSON(http.StatusOK, gin.H{
		"data": backends,
	})
}


