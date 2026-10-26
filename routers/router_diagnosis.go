package routers

import (
	"github.com/JLPAY/gwayne/controllers/diagnosis"
	"github.com/JLPAY/gwayne/middleware"
	"github.com/JLPAY/gwayne/models"
	"github.com/gin-gonic/gin"
	"k8s.io/klog/v2"
)

// SetupDiagnosisRoutes Pod/资源 AI 排查等诊断路由
func SetupDiagnosisRoutes(rg *gin.RouterGroup) {
	// 启动时清理上次运行遗留的 running 报告（goroutine 已随重启中断）
	if n, err := models.MarkStaleRunningReportsFailed(); err != nil {
		klog.Warningf("清理遗留排查报告失败: %v", err)
	} else if n > 0 {
		klog.Infof("已将 %d 条遗留的 running 排查报告标记为 failed", n)
	}

	g := rg.Group("/diagnosis").Use(middleware.JWTauth())
	{
		// 查询是否存在可用的 LLM 模型（用于前端隐藏/展示 AI 排查入口）
		g.GET("/available", diagnosis.DiagnosisAvailable)
		// 提交 Pod 异常 AI 排查任务（异步，立即返回报告 ID）
		g.POST("/pod", diagnosis.DiagnosePodAsync)
		// 排查报告列表（支持 cluster/namespace/pod/status 过滤）
		g.GET("/reports", diagnosis.ListPodDiagnosisReports)
		// 排查报告详情
		g.GET("/reports/:id", diagnosis.GetPodDiagnosisReport)
		// 删除排查报告
		g.DELETE("/reports/:id", diagnosis.DeletePodDiagnosisReport)
	}
}
