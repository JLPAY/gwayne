package routers

import (
	"github.com/JLPAY/gwayne/controllers/kubernetes/persistentvolume"
	"github.com/JLPAY/gwayne/middleware"
	"github.com/gin-gonic/gin"
)

func SetupKubernetesPVRoutes(rg *gin.RouterGroup) {
	// 定义 /api/v1/kubernetes/persistentvolumes 路由（PV 为集群级资源，必须登录鉴权）
	persistentvolumeGroup := rg.Group("/kubernetes/persistentvolumes").Use(middleware.JWTauth())
	{
		persistentvolumeGroup.GET("/clusters/:cluster", persistentvolume.List)
		persistentvolumeGroup.POST("/clusters/:cluster", persistentvolume.Create)
		persistentvolumeGroup.GET("/:name/clusters/:cluster", persistentvolume.Get)
		persistentvolumeGroup.PUT("/:name/clusters/:cluster", persistentvolume.Update)
		persistentvolumeGroup.DELETE("/:name/clusters/:cluster", persistentvolume.Delete)
	}
}
