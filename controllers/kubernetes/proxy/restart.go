package proxy

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/JLPAY/gwayne/pkg/kubernetes/client"
	"github.com/gin-gonic/gin"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
	"k8s.io/kubectl/pkg/polymorphichelpers"
)

// RestartRequest 重启请求结构体
type RestartRequest struct {
	Force bool `json:"force"` // 是否强制重启
}

// RestartResponse 重启响应结构体
type RestartResponse struct {
	Message   string    `json:"message"`
	Timestamp time.Time `json:"timestamp"`
	Success   bool      `json:"success"`
}

// RestartWorkload 使用polymorphichelpers重启工作负载
// @Title Restart Workload
// @Description Restart a workload using polymorphichelpers
// @Param	cluster		path 	string	true		"the cluster name"
// @Param	namespace		path 	string	true		"the namespace name"
// @Param	kind		path 	string	true		"the resource kind"
// @Param	name		path 	string	true		"the resource name"
// @Param	force		query 	bool	false		"force restart"
// @Success 200 {object} RestartResponse success
// @router /namespaces/:namespaceName/:kind/:name/restart [put]
func RestartWorkload(c *gin.Context) {
	// 获取路径参数
	cluster := c.Param("cluster")
	namespace := c.Param("namespaceName")
	name := c.Param("name")
	kind := c.Param("kind")

	// 解析请求体
	var restartReq RestartRequest
	if err := c.ShouldBindJSON(&restartReq); err != nil {
		// 如果没有请求体，使用默认值
		restartReq.Force = false
	}

	// 获取具体的kubernetes客户端
	clientset, err := client.Client(cluster)
	if err != nil {
		klog.Errorf("Failed to get clientset for cluster: %s", cluster)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get clientset"})
		return
	}

	// 根据资源类型获取对应的资源对象
	var obj runtime.Object
	switch kind {
	case "deployments":
		obj, err = clientset.AppsV1().Deployments(namespace).Get(context.Background(), name, metav1.GetOptions{})
	case "statefulsets":
		obj, err = clientset.AppsV1().StatefulSets(namespace).Get(context.Background(), name, metav1.GetOptions{})
	case "daemonsets":
		obj, err = clientset.AppsV1().DaemonSets(namespace).Get(context.Background(), name, metav1.GetOptions{})
	default:
		klog.Errorf("Unsupported resource kind: %s", kind)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Unsupported resource type"})
		return
	}

	if err != nil {
		klog.Errorf("Failed to get %s %s/%s: %v", kind, namespace, name, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 使用polymorphichelpers生成重启patch
	patchBytes, err := polymorphichelpers.ObjectRestarterFn(obj)
	if err != nil {
		klog.Errorf("Failed to generate restart patch for %s %s/%s: %v", kind, namespace, name, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 应用patch来执行重启
	switch kind {
	case "deployments":
		_, err = clientset.AppsV1().Deployments(namespace).Patch(
			context.Background(),
			name,
			types.StrategicMergePatchType,
			patchBytes,
			metav1.PatchOptions{},
		)
	case "statefulsets":
		_, err = clientset.AppsV1().StatefulSets(namespace).Patch(
			context.Background(),
			name,
			types.StrategicMergePatchType,
			patchBytes,
			metav1.PatchOptions{},
		)
	case "daemonsets":
		_, err = clientset.AppsV1().DaemonSets(namespace).Patch(
			context.Background(),
			name,
			types.StrategicMergePatchType,
			patchBytes,
			metav1.PatchOptions{},
		)
	}

	if err != nil {
		klog.Errorf("Failed to apply restart patch to %s %s/%s in cluster %s: %v", kind, namespace, name, cluster, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	response := RestartResponse{
		Message:   fmt.Sprintf("%s %s/%s restarted successfully", kind, namespace, name),
		Timestamp: time.Now(),
		Success:   true,
	}

	c.JSON(http.StatusOK, gin.H{"data": response})
}
