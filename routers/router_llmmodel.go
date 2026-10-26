package routers

import (
	"github.com/JLPAY/gwayne/controllers/llmmodel"
	"github.com/JLPAY/gwayne/middleware"
	"github.com/gin-gonic/gin"
)

// SetupLLMModelRoutes LLM 模型管理路由
func SetupLLMModelRoutes(rg *gin.RouterGroup) {
	modelGroup := rg.Group("/llm-models").Use(middleware.JWTauth())
	{
		// 获取模型列表
		modelGroup.GET("", llmmodel.ListLLMModels)

		// 获取模型详情
		modelGroup.GET("/:id", llmmodel.GetLLMModel)

		// 创建模型
		modelGroup.POST("", llmmodel.CreateLLMModel)

		// 更新模型
		modelGroup.PUT("/:id", llmmodel.UpdateLLMModel)

		// 删除模型
		modelGroup.DELETE("/:id", llmmodel.DeleteLLMModel)

		// 设置默认模型
		modelGroup.PUT("/:id/default", llmmodel.SetDefaultLLMModel)

		// 保存前连通性测试（使用表单配置，无需先保存）
		modelGroup.POST("/test", llmmodel.TestLLMModelConfig)

		// 连通性测试
		modelGroup.POST("/:id/test", llmmodel.TestLLMModel)
	}
}
