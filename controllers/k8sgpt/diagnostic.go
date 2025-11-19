package k8sgpt

import (
	"context"
	"net/http"
	"strings"

	"github.com/JLPAY/gwayne/pkg/k8sgpt"
	"github.com/gin-gonic/gin"
	"k8s.io/klog/v2"
)

var diagnosticService = k8sgpt.NewDiagnosticService()

// Diagnose 执行诊断
// @router /api/v1/k8sgpt/diagnose [post]
func Diagnose(c *gin.Context) {
	var req k8sgpt.DiagnosticRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		klog.Errorf("Failed to bind diagnostic request: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{
			"error": err.Error(),
		})
		return
	}

	if req.Cluster == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "cluster is required",
		})
		return
	}

	result, err := diagnosticService.Diagnose(context.Background(), req)
	if err != nil {
		klog.Errorf("Failed to diagnose: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data": result,
	})
}

// DiagnoseNode 诊断节点
// @router /api/v1/k8sgpt/diagnose/node/:cluster/:name [get]
func DiagnoseNode(c *gin.Context) {
	cluster := c.Param("cluster")
	nodeName := c.Param("name")
	explain := c.DefaultQuery("explain", "true") == "true"

	if cluster == "" || nodeName == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "cluster and node name are required",
		})
		return
	}

	result, err := diagnosticService.DiagnoseNode(context.Background(), cluster, nodeName, explain)
	if err != nil {
		klog.Errorf("Failed to diagnose node: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data": result,
	})
}

// DiagnosePod 诊断 Pod
// @router /api/v1/k8sgpt/diagnose/pod/:cluster/:namespace/:name [get]
func DiagnosePod(c *gin.Context) {
	cluster := c.Param("cluster")
	namespace := c.Param("namespace")
	podName := c.Param("name")
	explain := c.DefaultQuery("explain", "true") == "true"

	if cluster == "" || namespace == "" || podName == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "cluster, namespace and pod name are required",
		})
		return
	}

	result, err := diagnosticService.DiagnosePod(context.Background(), cluster, namespace, podName, explain)
	if err != nil {
		klog.Errorf("Failed to diagnose pod: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data": result,
	})
}

// DiagnoseEvent 诊断事件
// @router /api/v1/k8sgpt/diagnose/event/:cluster/:namespace [get]
func DiagnoseEvent(c *gin.Context) {
	cluster := c.Param("cluster")
	namespace := c.Param("namespace")
	explain := c.DefaultQuery("explain", "true") == "true"

	if cluster == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "cluster is required",
		})
		return
	}

	result, err := diagnosticService.DiagnoseEvent(context.Background(), cluster, namespace, explain)
	if err != nil {
		klog.Errorf("Failed to diagnose event: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data": result,
	})
}

// Explain AI 解释诊断结果
// @router /api/v1/k8sgpt/explain [post]
func Explain(c *gin.Context) {
	var req k8sgpt.ExplainRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		klog.Errorf("Failed to bind explain request: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{
			"error": err.Error(),
		})
		return
	}

	if req.Kind == "" || req.Name == "" || len(req.Errors) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "kind, name and errors are required",
		})
		return
	}

	result, err := diagnosticService.Explain(context.Background(), req)
	if err != nil {
		klog.Errorf("Failed to explain: %v", err)
		// 根据错误类型返回适当的 HTTP 状态码
		errMsg := err.Error()
		statusCode := http.StatusInternalServerError
		if strings.Contains(errMsg, "does not exist") || strings.Contains(errMsg, "not have access") || strings.Contains(errMsg, "模型") {
			statusCode = http.StatusBadRequest // 400: 客户端配置错误
		} else if strings.Contains(errMsg, "401") || strings.Contains(errMsg, "unauthorized") || strings.Contains(errMsg, "认证") {
			statusCode = http.StatusUnauthorized // 401: 认证失败
		}
		c.JSON(statusCode, gin.H{
			"error": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data": result,
	})
}


