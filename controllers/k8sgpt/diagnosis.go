package k8sgpt

import (
	"context"
	"net/http"
	"strconv"

	"github.com/JLPAY/gwayne/pkg/k8sgpt"
	"github.com/gin-gonic/gin"
	"k8s.io/klog/v2"
)

// AnalyzeRequest 诊断分析请求
type AnalyzeRequest struct {
	ClusterName string   `json:"clusterName" binding:"required"`
	Namespace   string   `json:"namespace,omitempty"`   // 命名空间，为空则分析所有命名空间
	Filters     []string `json:"filters,omitempty"`       // 分析器过滤器，为空则使用所有分析器
	BackendID   int64    `json:"backendID,omitempty"`    // AI后端ID，为空则使用默认后端
}

// Analyze 执行集群诊断分析
func Analyze(c *gin.Context) {
	var req AnalyzeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		klog.Errorf("解析请求体失败: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "error",
			"data": gin.H{
				"message": "请求参数错误: " + err.Error(),
			},
		})
		return
	}

	// 创建K8sGPT服务
	service, err := k8sgpt.NewK8sGPTService(req.BackendID)
	if err != nil {
		klog.Errorf("创建K8sGPT服务失败: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "error",
			"data": gin.H{
				"message": "创建K8sGPT服务失败: " + err.Error(),
			},
		})
		return
	}
	defer service.Close()

	// 执行分析
	ctx := context.Background()
	results, err := service.Analyze(ctx, req.ClusterName, req.Namespace, req.Filters)
	if err != nil {
		klog.Errorf("执行诊断分析失败: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "error",
			"data": gin.H{
				"message": "执行诊断分析失败: " + err.Error(),
			},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "success",
		"data": gin.H{
			"results": results,
			"count":   len(results),
		},
	})
}

// ListAnalyzers 列出所有可用的分析器
func ListAnalyzers(c *gin.Context) {
	coreAnalyzers, additionalAnalyzers, integrationAnalyzers := k8sgpt.ListAnalyzers()

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "success",
		"data": gin.H{
			"core":        coreAnalyzers,
			"additional":  additionalAnalyzers,
			"integration": integrationAnalyzers,
		},
	})
}

// AnalyzeByCluster 根据集群名称执行诊断（从URL参数获取）
func AnalyzeByCluster(c *gin.Context) {
	clusterName := c.Param("cluster")
	if clusterName == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "error",
			"data": gin.H{
				"message": "集群名称不能为空",
			},
		})
		return
	}

	// 从查询参数获取可选参数
	namespace := c.Query("namespace")
	backendIDStr := c.Query("backendID")
	var backendID int64
	if backendIDStr != "" {
		var err error
		backendID, err = strconv.ParseInt(backendIDStr, 10, 64)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"code":    400,
				"message": "error",
				"data": gin.H{
					"message": "无效的backendID参数",
				},
			})
			return
		}
	}

	// 从查询参数获取过滤器
	filters := c.QueryArray("filters")

	// 创建K8sGPT服务
	service, err := k8sgpt.NewK8sGPTService(backendID)
	if err != nil {
		klog.Errorf("创建K8sGPT服务失败: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "error",
			"data": gin.H{
				"message": "创建K8sGPT服务失败: " + err.Error(),
			},
		})
		return
	}
	defer service.Close()

	// 执行分析
	ctx := context.Background()
	results, err := service.Analyze(ctx, clusterName, namespace, filters)
	if err != nil {
		klog.Errorf("执行诊断分析失败: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "error",
			"data": gin.H{
				"message": "执行诊断分析失败: " + err.Error(),
			},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "success",
		"data": gin.H{
			"clusterName": clusterName,
			"namespace":   namespace,
			"results":     results,
			"count":       len(results),
		},
	})
}

