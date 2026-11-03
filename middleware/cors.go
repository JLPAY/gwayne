package middleware

import (
	"github.com/gin-gonic/gin"
	"net/http"
)

// Cors 放行跨域请求并放行所有 OPTIONS 方法。
// 注意：Access-Control-Allow-Credentials 为 true 时，Allow-Origin 不得为 "*"（浏览器会拒绝带凭据的请求），
// 因此回显请求方 Origin，既满足凭据场景，又避免对任意第三方站点开放。
func Cors() gin.HandlerFunc {
	return func(context *gin.Context) {
		method := context.Request.Method

		origin := context.Request.Header.Get("Origin")
		if origin != "" {
			context.Header("Access-Control-Allow-Origin", origin)
		} else {
			context.Header("Access-Control-Allow-Origin", "*")
		}
		context.Header("Access-Control-Allow-Credentials", "true")
		context.Header("Access-Control-Allow-Headers", "Origin,Authorization,Access-Control-Allow-Origin,Access-Control-Allow-Headers,Content-Type")
		context.Header("Access-Control-Allow-Methods", "GET,HEAD,POST,PUT,DELETE,OPTIONS")
		context.Header("Access-Control-Expose-Headers", "Content-Length,Access-Control-Allow-Origin,Access-Control-Allow-Headers,Content-Type")

		if method == "OPTIONS" {
			context.AbortWithStatus(http.StatusNoContent)
		}
		context.Next()
	}
}
