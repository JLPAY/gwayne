package routers

import (
	"github.com/JLPAY/gwayne/controllers/cluster"
	"github.com/JLPAY/gwayne/middleware"
	"github.com/gin-gonic/gin"
)

func SetupClustersRoutes(rg *gin.RouterGroup) {
	// 定义 /api/v1/clusters 路由
	clusterGroup := rg.Group("/clusters").Use(middleware.JWTauth())
	{
		// 获取所有集群
		clusterGroup.GET("", cluster.List)
		// 创建集群
		clusterGroup.POST("", cluster.Create)
		// 获取指定集群
		clusterGroup.GET("/:name", cluster.Get)
		// 更新集群
		clusterGroup.PUT("/:name", cluster.Update)
		// 删除集群
		clusterGroup.DELETE("/:name", cluster.Delete)
		// 获取集群名称列表
		clusterGroup.GET("/names", cluster.GetNames)
	}
}
