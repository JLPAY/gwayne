package routers

import (
	"github.com/JLPAY/gwayne/controllers/kubernetes/namespace"
	"github.com/JLPAY/gwayne/middleware"
	"github.com/gin-gonic/gin"
)

func SetupKubernetesNSRoutes(rg *gin.RouterGroup) {
	// 定义 /api/v1/kubernetes/nodes 路由
	namespaceGroup := rg.Group("/kubernetes/nodes").Use(middleware.JWTauth())
	{
		// 创建 namespace
		namespaceGroup.POST("/:name/clusters/:cluster", namespace.Create)

		// 获取 Namespace 的资源
		namespaceGroup.GET("/:namespaceid/resources", namespace.Resources)

		// 获取 Namespace 的统计信息
		namespaceGroup.GET("/:namespaceid/statistics", namespace.Statistics)
	}
}
