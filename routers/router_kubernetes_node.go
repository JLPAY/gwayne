package routers

import (
	"github.com/JLPAY/gwayne/controllers/kubernetes/node"
	"github.com/JLPAY/gwayne/middleware"
	"github.com/gin-gonic/gin"
)

func SetupKubernetesNodeRoutes(rg *gin.RouterGroup) {
	// 定义 /api/v1/kubernetes/nodes 路由
	nodeGroup := rg.Group("/kubernetes/nodes").Use(middleware.JWTauth())
	{
		// 获取节点列表
		nodeGroup.GET("/clusters/:cluster", node.List)

		// 获取节点信息
		nodeGroup.GET("/:name/clusters/:cluster", node.Get)

		// 更新节点信息
		nodeGroup.PUT("/:name/clusters/:cluster", node.Update)

		// 删除节点
		nodeGroup.DELETE("/:name/clusters/:cluster", node.Delete)

		// 添加标签
		nodeGroup.POST("/:name/clusters/:cluster/label", node.AddLabel)

		// 删除标签
		nodeGroup.DELETE("/:name/clusters/:cluster/label", node.DeleteLabel)

		// 获取节点标签
		nodeGroup.GET("/:name/clusters/:cluster/labels", node.GetLabels)

		// 添加多个标签
		nodeGroup.POST("/:name/clusters/:cluster/labels", node.AddLabels)

		// 删除多个标签
		nodeGroup.DELETE("/:name/clusters/:cluster/labels", node.DeleteLabels)

		// 设置 Taint
		nodeGroup.POST("/:name/clusters/:cluster/taint", node.SetTaint)

		// 删除 Taint
		nodeGroup.DELETE("/:name/clusters/:cluster/taint", node.DeleteTaint)

		// cordon node
		nodeGroup.PUT("/:name/clusters/:cluster/cordon", node.Cordon)
		nodeGroup.PUT("/:name/clusters/:cluster/uncordon", node.UnCordon)

		// 节点驱逐
		nodeGroup.POST("/:name/clusters/:cluster/drain", node.DrainNode)

		// 获取节点统计信息
		nodeGroup.GET("/statistics", node.NodeStatistics)

		// 节点巡检：按可配置巡检项执行（内存/磁盘/CPU/负载/压力/FD/Pod/证书等）
		nodeGroup.GET("/inspect", node.Inspect)

		// 节点巡检历史：历次运行汇总 + 明细
		nodeGroup.GET("/inspect/history", node.History)
		nodeGroup.GET("/inspect/history/runs", node.HistoryRuns)
	}
}
