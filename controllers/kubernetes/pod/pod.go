package pod

import (
	"context"
	"net/http"

	"github.com/JLPAY/gwayne/controllers/base"
	"github.com/JLPAY/gwayne/pkg/k8sgpt"
	"github.com/JLPAY/gwayne/pkg/kubernetes/client"
	pod "github.com/JLPAY/gwayne/pkg/kubernetes/resources/pod"
	"github.com/gin-gonic/gin"
	"k8s.io/klog/v2"
)

// @Title List
// @Description find pods by resource type
// @Param	pageNo		query 	int	false		"the page current no"
// @Param	pageSize		query 	int	false		"the page size"
// @Param	type		query 	string	true		"the query type. deployment, statefulset, daemonSet, job, pod"
// @Param	name		query 	string	true		"the query resource name."
// @Success 200 {object} models.Deployment success
// @router /namespaces/:namespace/clusters/:cluster [get]
func List(c *gin.Context) {
	// 获取 URL 参数
	cluster := c.Param("cluster")
	namespace := c.Param("namespace")

	// 获取查询参数
	resourceType := c.DefaultQuery("type", "")
	resourceName := c.DefaultQuery("name", "")

	// 构建 Kubernetes 查询参数
	param := base.BuildQueryParam(c)

	// 获取 Kubernetes 客户端
	kubeClient, err := client.KubeClient(cluster)
	if kubeClient == nil || err != nil {
		klog.Errorf("Failed to get kubeClient for cluster: %s", cluster)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get kubeClient"})
		return
	}

	// 获取 pods 列表
	//result, err := pod.GetPodListPageByType(kubeClient, namespace, resourceName, resourceType, param)
	result, err := pod.GetPodListPageByType(kubeClient, namespace, resourceName, resourceType, param)
	if err != nil {
		// 错误日志
		klog.Errorf("Get kubernetes pod by type error. Cluster: %s, Namespace: %s, Type: %s, Name: %s, Error: %v",
			cluster, namespace, resourceType, resourceName, err)

		// 返回错误响应
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	// 返回成功响应
	c.JSON(200, gin.H{"data": result})

}

// Diagnose 诊断 Pod
// @router /namespaces/:namespace/clusters/:cluster/diagnose [get]
func Diagnose(c *gin.Context) {
	cluster := c.Param("cluster")
	namespace := c.Param("namespace")
	podName := c.Query("name")
	explain := c.DefaultQuery("explain", "true") == "true"

	if cluster == "" || namespace == "" || podName == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "cluster, namespace and pod name are required",
		})
		return
	}

	diagnosticService := k8sgpt.NewDiagnosticService()
	result, err := diagnosticService.DiagnosePod(context.Background(), cluster, namespace, podName, explain)
	if err != nil {
		klog.Errorf("Failed to diagnose pod %s/%s: %v", namespace, podName, err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data": result,
	})
}
