package routers

import (
	"github.com/JLPAY/gwayne/controllers/k8sgpt"
	"github.com/JLPAY/gwayne/middleware"
	"github.com/gin-gonic/gin"
)

// SetupK8sGPTRoutes 设置K8sGPT相关路由
func SetupK8sGPTRoutes(rg *gin.RouterGroup) {
	// AI后端管理路由
	aiBackendGroup := rg.Group("/k8sgpt/ai-backends").Use(middleware.JWTauth())
	{
		// 获取所有AI后端列表
		aiBackendGroup.GET("", k8sgpt.ListAIBackends)

		// 获取单个AI后端详情
		aiBackendGroup.GET("/:id", k8sgpt.GetAIBackend)

		// 创建AI后端
		aiBackendGroup.POST("", k8sgpt.CreateAIBackend)

		// 更新AI后端
		aiBackendGroup.PUT("/:id", k8sgpt.UpdateAIBackend)

		// 删除AI后端
		aiBackendGroup.DELETE("/:id", k8sgpt.DeleteAIBackend)

		// 设置默认AI后端
		aiBackendGroup.PUT("/:id/default", k8sgpt.SetDefaultAIBackend)
	}

	// 诊断服务路由
	diagnosisGroup := rg.Group("/k8sgpt/diagnosis").Use(middleware.JWTauth())
	{
		// 执行诊断分析（POST方式，支持复杂参数）
		diagnosisGroup.POST("/analyze", k8sgpt.Analyze)

		// 根据集群执行诊断（GET方式，从URL参数获取）
		diagnosisGroup.GET("/clusters/:cluster", k8sgpt.AnalyzeByCluster)

		// 列出所有可用的分析器
		diagnosisGroup.GET("/analyzers", k8sgpt.ListAnalyzers)
	}
}

