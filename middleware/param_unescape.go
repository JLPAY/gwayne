package middleware

import (
	"net/url"

	"github.com/gin-gonic/gin"
)

// UnescapePathParams 配合 Engine.UseRawPath = true、Engine.UnescapePathValues = false 使用。
//
// 背景：gin 默认用已解码的 URL.Path 做路由匹配，路径参数（如集群名 "云店-dev/fat"）
// 中的 "/" 会被当成路径分隔符，导致 /api/v1/clusters/:name 匹配不上而返回 404。
// 开启 UseRawPath 后路由改用未解码的 RawPath 匹配，"%2F" 不再算作分隔符，
// 但此时 c.Params 里的值是未解码的（形如 "%E4%BA%91%E5%BA%97-dev%2Ffat"）。
//
// 本中间件在路由匹配完成后统一解码一次，使所有 handler 拿到的参数与以往一致；
// 对不含转义字符的普通参数是幂等的，不改变原有行为。
func UnescapePathParams() gin.HandlerFunc {
	return func(c *gin.Context) {
		for i := range c.Params {
			raw := c.Params[i].Value
			// 与 gin 内置行为保持一致，使用 QueryUnescape
			if decoded, err := url.QueryUnescape(raw); err == nil && decoded != raw {
				c.Params[i].Value = decoded
			}
		}
		c.Next()
	}
}
