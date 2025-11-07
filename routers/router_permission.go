package routers

import (
	"github.com/JLPAY/gwayne/controllers/permission"
	"github.com/JLPAY/gwayne/middleware"
	"github.com/gin-gonic/gin"
)

func SetupPermissionRoutes(rg *gin.RouterGroup) {
	// 定义用户路由
	userGroup := rg.Group("/users").Use(middleware.JWTauth())
	{
		userGroup.GET("", permission.UsersList)
		userGroup.POST("", permission.UserCreate)
		userGroup.GET("/:id", permission.UserGet)
		userGroup.PUT("/:id", permission.UserUpdate)
		userGroup.DELETE("/:id", permission.UserDelete)

		// 修改密码
		userGroup.PUT("/:id/resetpassword", permission.ResetPassword)

		// 更改admin属性
		userGroup.PUT("/:id/admin", permission.UpdateAdmin)

	}

}
