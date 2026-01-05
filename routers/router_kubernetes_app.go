package routers

import (
	"github.com/JLPAY/gwayne/controllers/app"
	"github.com/JLPAY/gwayne/controllers/kubernetes/pod"
	"github.com/JLPAY/gwayne/middleware"
	"github.com/gin-gonic/gin"
)

func SetupKubernetesAppRoutes(rg *gin.RouterGroup) {
	StatisticsGroup := rg.Group("").Use(middleware.JWTauth())
	{
		StatisticsGroup.GET("/apps/statistics", app.AppStatistics)
		StatisticsGroup.GET("/kubernetes/pods/statistics", app.AppStatistics)
		StatisticsGroup.GET("/users/statistics", app.UserStatistics)
	}

	appGroup := rg.Group("/kubernetes/apps/:appid([0-9]+)").Use(middleware.JWTauth())
	{
		// /kubernetes/apps/0/pods/namespaces/account/clusters/UAT
		appGroup.GET("/cronjobs")
		appGroup.GET("/deployments")
		appGroup.GET("/statefulsets")
		appGroup.GET("/daemonsets")
		appGroup.GET("/configmaps")
		appGroup.GET("/services")
		appGroup.GET("/ingresses")
		appGroup.GET("/hpas")
		appGroup.GET("/secrets")
		appGroup.GET("/persistentvolumeclaims")
		appGroup.GET("/jobs")

		appGroup.GET("/pods/namespaces/:namespace/clusters/:cluster", pod.List)
		// 容器终端
		appGroup.POST("/pods/:pod/terminal/namespaces/:namespace/clusters/:cluster", pod.Terminal)

		appGroup.GET("/podlogs/:pod/containers/:container/namespaces/:namespace/clusters/:cluster", pod.ListLogs)
		appGroup.GET("/events")
	}

	appNamesGroup := rg.Group("/namespaces/0/apps").Use(middleware.JWTauth())
	{
		// /api/v1/namespaces/0/apps/names
		// 返回空，只为wayne兼容前端
		appNamesGroup.GET("/names", app.Names)
	}
}
