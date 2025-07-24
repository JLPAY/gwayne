package routers

import (
	"github.com/JLPAY/gwayne/controllers/configs"
	"github.com/gin-gonic/gin"
)

func SetupConfigRoutes(rg *gin.RouterGroup) {
	// 定义 /api/v1/clusters 路由
	configGroup := rg.Group("/configs")
	{
		configGroup.GET("/base", configs.ListBase)
		configGroup.GET("/system", configs.ListSystem)
	}
}
