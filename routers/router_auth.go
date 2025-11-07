package routers

import (
	"github.com/JLPAY/gwayne/controllers/auth"
	"github.com/gin-gonic/gin"
)

func AuthRoutes(router *gin.Engine) {

	// 定义 AuthController 相关路由
	authGroup := router.Group("")
	{
		// 获取当前用户
		authGroup.GET("/currentuser", auth.CurrentUser)
		// 用户登录
		authGroup.GET("/login/:type", auth.Login)
		authGroup.POST("/login/:type", auth.Login)
		authGroup.POST("/login/:type/:name", auth.Login)
		// oauth2 回调 ,:name 是回调参数
		authGroup.GET("/login/:type/:name", auth.Login)
		// 用户退出
		authGroup.GET("/logout", auth.Logout)
	}

}
