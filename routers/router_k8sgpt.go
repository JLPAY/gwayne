package routers

import (
	"github.com/JLPAY/gwayne/controllers/k8sgpt"
	"github.com/JLPAY/gwayne/middleware"
	"github.com/gin-gonic/gin"
)

// SetupK8sGPTRoutes 设置 K8sGPT 相关路由
func SetupK8sGPTRoutes(rg *gin.RouterGroup) {
	// AI 引擎管理路由
	aiGroup := rg.Group("/k8sgpt/ai").Use(middleware.JWTauth())
	{
		// 列出所有 AI 提供者
		aiGroup.GET("/providers", k8sgpt.ListProviders)

		// 添加 AI 提供者
		aiGroup.POST("/providers", k8sgpt.AddProvider)

		// 删除 AI 提供者
		aiGroup.DELETE("/providers/:name", k8sgpt.RemoveProvider)

		// 设置默认 AI 提供者
		aiGroup.PUT("/providers/:name/default", k8sgpt.SetDefaultProvider)

		// 获取可用的 AI 后端列表
		aiGroup.GET("/backends", k8sgpt.GetAvailableBackends)
	}

	// 诊断路由
	diagnosticGroup := rg.Group("/k8sgpt/diagnose").Use(middleware.JWTauth())
	{
		// 通用诊断接口
		diagnosticGroup.POST("", k8sgpt.Diagnose)

		// 诊断节点
		diagnosticGroup.GET("/node/:cluster/:name", k8sgpt.DiagnoseNode)

		// 诊断 Pod
		diagnosticGroup.GET("/pod/:cluster/:namespace/:name", k8sgpt.DiagnosePod)

		// 诊断事件
		diagnosticGroup.GET("/event/:cluster/:namespace", k8sgpt.DiagnoseEvent)
	}

	// AI 解释路由
	explainGroup := rg.Group("/k8sgpt").Use(middleware.JWTauth())
	{
		// AI 解释接口
		explainGroup.POST("/explain", k8sgpt.Explain)
	}
}


