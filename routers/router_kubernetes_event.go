package routers

import (
	"github.com/JLPAY/gwayne/controllers/kubernetes/event"
	"github.com/JLPAY/gwayne/middleware"
	"github.com/gin-gonic/gin"
)

// SetupKubernetesEventRoutes 设置 Kubernetes Event 相关路由
func SetupKubernetesEventRoutes(rg *gin.RouterGroup) {
	eventGroup := rg.Group("/kubernetes/events").Use(middleware.JWTauth())
	{
		// 诊断事件
		eventGroup.GET("/namespaces/:namespace/clusters/:cluster/diagnose", event.Diagnose)
	}
}


