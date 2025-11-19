package event

import (
	"context"
	"net/http"

	"github.com/JLPAY/gwayne/pkg/k8sgpt"
	"github.com/gin-gonic/gin"
	"k8s.io/klog/v2"
)

// Diagnose 诊断事件
// @router /namespaces/:namespace/clusters/:cluster/diagnose [get]
func Diagnose(c *gin.Context) {
	cluster := c.Param("cluster")
	namespace := c.Param("namespace")
	explain := c.DefaultQuery("explain", "true") == "true"

	if cluster == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "cluster is required",
		})
		return
	}

	diagnosticService := k8sgpt.NewDiagnosticService()
	result, err := diagnosticService.DiagnoseEvent(context.Background(), cluster, namespace, explain)
	if err != nil {
		klog.Errorf("Failed to diagnose events in namespace %s: %v", namespace, err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data": result,
	})
}


