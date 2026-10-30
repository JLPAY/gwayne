package routers

import (
	"github.com/JLPAY/gwayne/controllers/kubernetes/pod"
	"github.com/JLPAY/gwayne/middleware"
	"github.com/JLPAY/gwayne/pkg/config"
	"github.com/gin-gonic/gin"
)

func InitRouter() *gin.Engine {
	r := gin.New()

	// 允许路径参数中出现被转义的 "/"（%2F）。集群名等业务标识可能包含 "/"，
	// 默认按已解码的 URL.Path 匹配会被拆成多段导致路由 404。
	// 开启 RawPath 匹配、关闭 gin 的内置解码，由 middleware.UnescapePathParams 统一解码。
	r.UseRawPath = true
	r.UnescapePathValues = false

	// 记录日志和恢复
	r.Use(gin.Logger())
	r.Use(gin.Recovery())

	// 路径参数统一解码（需在业务 handler 之前）
	r.Use(middleware.UnescapePathParams())

	// 配置 CORS 中间件
	r.Use(middleware.Cors())

	gin.SetMode(config.Conf.App.RunMode)

	// 健康检查：只暴露探活结果，不暴露数据库等内部组件状态（避免未授权探测内网拓扑）
	r.GET("/healthz", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"status": "ok",
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

		// LLM 模型接入路由
		SetupLLMModelRoutes(apiV1)

		// Pod/资源 AI 排查诊断路由
		SetupDiagnosisRoutes(apiV1)

		// 终端命令规则路由
		SetupTerminalRoutes(apiV1)

	}

	return r
}
