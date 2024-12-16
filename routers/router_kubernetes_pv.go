package routers

import (
	"github.com/JLPAY/gwayne/controllers/kubernetes/persistentvolume"
	"github.com/gin-gonic/gin"
)

func SetupKubernetesPVRoutes(rg *gin.RouterGroup) {
	// 定义 /api/v1/kubernetes/nodes 路由
	persistentvolumeGroup := rg.Group("/kubernetes/persistentvolumes")
	{
		persistentvolumeGroup.GET("/clusters/:cluster", persistentvolume.List)
		persistentvolumeGroup.POST("/clusters/:cluster", persistentvolume.Create)
		persistentvolumeGroup.GET("/:name/clusters/:cluster", persistentvolume.Get)
		persistentvolumeGroup.PUT("/:name/clusters/:cluster", persistentvolume.Update)
		persistentvolumeGroup.DELETE("/:name/clusters/:cluster", persistentvolume.Delete)
	}
}
