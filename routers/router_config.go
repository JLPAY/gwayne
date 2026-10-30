package routers

import (
	"github.com/JLPAY/gwayne/controllers/configs"
	"github.com/JLPAY/gwayne/middleware"
	"github.com/gin-gonic/gin"
)

func SetupConfigRoutes(rg *gin.RouterGroup) {
	configGroup := rg.Group("/configs")
	{
		// 登录页需要读取登录方式开关与 OAuth2 provider 展示信息（无敏感字段），保持匿名
		configGroup.GET("/base", configs.ListBase)
		// 全量系统配置含签名密钥、私钥路径、数据库与 OAuth2 凭据等敏感信息，必须登录后方可访问
		configGroup.GET("/system", middleware.JWTauth(), configs.ListSystem)
	}
}
