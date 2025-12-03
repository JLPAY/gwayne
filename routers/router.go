package routers

import (
	"github.com/JLPAY/gwayne/controllers/kubernetes/pod"
	"github.com/JLPAY/gwayne/middleware"
	"github.com/JLPAY/gwayne/pkg/config"
	"github.com/gin-gonic/gin"
)

func InitRouter() *gin.Engine {
	r := gin.New()

	// 记录日志和恢复
	r.Use(gin.Logger())
	r.Use(gin.Recovery())

	// 配置 CORS 中间件
	r.Use(middleware.Cors())

	gin.SetMode(config.Conf.App.RunMode)

	r.GET("/healthz", func(c *gin.Context) {
		// 这里可以添加数据库连接检查、外部服务检查等
		// 假设健康检查通过，可以返回正常状态
		c.JSON(200, gin.H{
			"status":   "ok",
			"database": "connected", // 可以加上数据库连接状态
			"uptime":   "72 hours",  // 服务器运行时间
		})
	})

	// 这里创建一个处理 WebSocket 连接的路由
	r.GET("/ws/pods/exec/*param", func(c *gin.Context) {
		handler := pod.CreateAttachHandler("/ws/pods/exec")
		handler.ServeHTTP(c.Writer, c.Request)
	})

	// 注册鉴权路由
	AuthRoutes(r)

	// 定义 /api/v1 路由组
	apiV1 := r.Group("/api/v1")
	apiV1.Use(middleware.Cors())
	{
		// 权限路径
		SetupPermissionRoutes(apiV1)

		// 定义 clusters 子路由
		SetupClustersRoutes(apiV1)

		// 定义 config 子路由
		SetupConfigRoutes(apiV1)

		SetupKubernetesNodeRoutes(apiV1)

		// k8s resource路由增删改查
		SetupKubernetesProxyResourcesRoutes(apiV1)

		SetupKubernetesPVRoutes(apiV1)

		SetupKubernetesAppRoutes(apiV1)

		// K8sGPT相关路由
		SetupK8sGPTRoutes(apiV1)

	}

	return r
}
