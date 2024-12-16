package namespace

import (
	"github.com/JLPAY/gwayne/pkg/kubernetes/client"
	"github.com/JLPAY/gwayne/pkg/kubernetes/resources/namespace"
	"github.com/gin-gonic/gin"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
	"net/http"
)

// @Title List/
// @Description get all id and names
// @Param	deleted		query 	bool	false		"is deleted,default false."
// @Success 200 {object} []models.Namespace success
// @router /names [get]
func GetNames(c *gin.Context) {

}

// @Title Create
// @Description create the namespace
// @router /:name/clusters/:cluster [post]
func Create(c *gin.Context) {
	// 获取路径参数
	cluster := c.Param("cluster") // 集群名称
	name := c.Param("name")       // 节点名称

	// 创建 Kubernetes Namespace 对象
	namespacev1 := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}

	client, err := client.Client(cluster)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 创建命名空间
	result, err := namespace.CreateNotExitNamespace(client, namespacev1)
	if err != nil {
		klog.Errorf("Failed to create namespace %s in cluster %s: %v", name, cluster, err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to create namespace",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": result})
}

// @Title Get namespace resource statistics
// @Description Get namespace resource statistics
// @Param	app	query 	string	false	"The app Name"
// @Param	nid	path 	string	true	"The namespace id"
// @Success 200 return ok success
// @router /:namespaceid([0-9]+)/resources [get]
func Resources(c *gin.Context) {

}

func Statistics(c *gin.Context) {

}
