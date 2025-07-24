package pod

import (
	"net/http"
	"strconv"

	"github.com/JLPAY/gwayne/pkg/hack"
	"github.com/JLPAY/gwayne/pkg/kubernetes/client"
	"github.com/JLPAY/gwayne/pkg/kubernetes/resources/log"
	"github.com/gin-gonic/gin"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/klog/v2"
)

// @Title log
// @Description pod logs
// @Param	tailLines		query 	int 	true		"log tail lines."
// @Param	cluster		path 	string 	true		"cluster name."
// @Param	namespace		path 	string 	true		"namespace name."
// @Param	pod		path 	string 	true		"pod name."
// @Param	container		path 	string 	true		"container name."
// @Success 200 {object} "log text" success
// @router /:pod/containers/:container/namespaces/:namespace/clusters/:cluster [get]
func ListLogs(c *gin.Context) {
	// 获取查询参数
	tailLines := c.DefaultQuery("tailLines", "10") // 默认获取10行日志
	cluster := c.Param("cluster")
	namespace := c.Param("namespace")
	pod := c.Param("pod")
	container := c.Param("container")

	// 将 tailLines 转换为 int64 类型
	tailLinesInt, err := strconv.Atoi(tailLines)
	if err != nil {
		klog.Errorf("get %s pod %s/s logs error, Invalid tailLines parameter", cluster, pod, container)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid tailLines parameter"})
		return
	}

	tailLinesInt64 := int64(tailLinesInt)

	opt := &corev1.PodLogOptions{
		Container: container,
		TailLines: &tailLinesInt64,
	}

	manager, err := client.Manager(cluster)
	if manager == nil || err != nil {
		klog.Errorf("Failed to get manager for cluster: %s", cluster)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get manager"})
		return
	}

	// 获取 Pod 日志
	result, err := log.GetLogsByPod(manager.Client, namespace, pod, opt)
	if err != nil {
		klog.Errorf("Error getting logs for pod %s in namespace %s: %v", pod, namespace, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch logs"})
		return
	}

	klog.V(2).Infof("Get logs for pod %s in namespace %s\n%s", pod, namespace, string(result))

	c.JSON(http.StatusOK, gin.H{"data": hack.String(result)})
}
