package proxy

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/JLPAY/gwayne/controllers/base"
	"github.com/JLPAY/gwayne/pkg/kubernetes/client"
	"github.com/JLPAY/gwayne/pkg/kubernetes/resources/proxy"
	"github.com/gin-gonic/gin"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/klog/v2"
)

// @Title Get all resource names
// @Description get all names
// @Param	cluster		path 	string	true		"the cluster name"
// @Param	namespace		path 	string	true		"the namespace name"
// @Param	kind		path 	string	true		"the resource kind"
// @Success 200 {object} []response.NamesObject success
// @router /names [get]
func GetNames(c *gin.Context) {
	// 获取路径参数
	cluster := c.Param("cluster")
	namespace := c.Param("namespace")
	kind := "namespaces"

	// 获取 Kubernetes 客户端
	kubeClient, err := client.KubeClient(cluster)
	if kubeClient == nil || err != nil {
		klog.Errorf("Failed to get kubeClient for cluster: %s", cluster)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get kubeClient"})
		return
	}

	result, err := proxy.GetNames(kubeClient, kind, namespace)
	if err != nil {
		klog.Errorf("Failed to get names for cluster: %s, kind: %s, namespace: %s", cluster, kind, namespace)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": result})

}

// @Title Get
// @Description Find Object by name
// @Param	cluster		path 	string	true		"the cluster name"
// @Param	namespace		path 	string	true		"the namespace name"
// @Param	kind		path 	string	true		"the resource kind"
// @Param	name		path 	string	true		"the resource name"
// @Success 200 {object}  success
// @router /:name [get]
func Get(c *gin.Context) {
	// 从路由参数中获取变量
	cluster := c.Param("cluster")
	namespace := c.Param("namespaceName")
	name := c.Param("name")
	kind := c.Param("kind")

	klog.Infof("Get cluster: %s, namespace: %s, name: %s, kind: %s", cluster, namespace, name, kind)

	// 获取 Kubernetes 客户端
	kubeClient, err := client.KubeClient(cluster)
	if kubeClient == nil || err != nil {
		klog.Errorf("Failed to get kubeClient for cluster: %s", cluster)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get kubeClient"})
		return
	}

	result, err := kubeClient.Get(kind, namespace, name)
	if err != nil {
		klog.Errorf("Failed to get %s %s/%s from cluster: %s", kind, namespace, name, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": result})
}

func List(c *gin.Context) {
	// 获取查询参数
	param := base.BuildQueryParam(c)
	cluster := c.Param("cluster")
	namespace := c.Param("namespaceName")
	kind := c.Param("kind")

	// 获取 Kubernetes 客户端
	kubeClient, err := client.KubeClient(cluster)
	if kubeClient == nil || err != nil {
		klog.Errorf("Failed to get kubeClient for cluster: %s", cluster)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get kubeClient"})
		return
	}

	// 调用 proxy.GetPage 来获取资源列表
	result, err := proxy.GetPage(kubeClient, kind, namespace, param)
	if err != nil {
		klog.Errorf("List kubernetes resource (%s:%s) from cluster (%s) error: %v", kind, namespace, cluster, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 返回成功的响应
	c.JSON(http.StatusOK, gin.H{"data": result})
}

// @Title Create
// @Description Create the resource
// @Param	cluster		path 	string	true		"the cluster name"
// @Param	kind		path 	string	true		"the resource kind"
// @Param	namespace		path 	string	true		"the namespace name"
// @Param	name		path 	string	true		"the resource name"
// @Param	resource		body 	string	false		"the kubernetes resource"
// @Success 200 {string} delete success!
// @router / [post]
func Create(c *gin.Context) {
	// 获取路径参数
	cluster := c.Param("cluster")
	namespace := c.Param("namespaceName")
	kind := c.Param("kind")

	// 解析请求体
	var object runtime.Unknown
	if err := c.BindJSON(&object); err != nil {
		// 处理解析错误
		klog.Errorf("Create kubernetes resource (%s:%s) from cluster (%s) error. %v", kind, namespace, cluster, err)
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("Invalid request body: %v", err)})
		return
	}

	// 获取 Kubernetes 客户端
	kubeClient, err := client.KubeClient(cluster)
	if kubeClient == nil || err != nil {
		klog.Errorf("Failed to get kubeClient for cluster: %s", cluster)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get kubeClient"})
		return
	}

	// 调用 Kubernetes 客户端的 Create 方法
	result, err := kubeClient.Create(kind, namespace, &object)
	if err != nil {
		// 处理错误
		klog.Errorf("Error creating resource (%s:%s) in cluster (%s): %v", kind, namespace, cluster, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": result})
}

// @Title Update
// @Description Update the resource
// @Param	cluster		path 	string	true		"the cluster name"
// @Param	kind		path 	string	true		"the resource kind"
// @Param	namespace		path 	string	true		"the namespace name"
// @Param	name		path 	string	true		"the resource name"
// @Param	resource		body 	string	false		"the kubernetes resource"
// @Success 200 {string} delete success!
// @router /:name [put]
func Put(c *gin.Context) {
	// 获取查询参数
	cluster := c.Param("cluster")
	namespace := c.Param("namespaceName")
	name := c.Param("name")
	kind := c.Param("kind")

	// 解析请求体中的资源对象
	var object runtime.Unknown
	if err := c.ShouldBindJSON(&object); err != nil {
		// 请求体无法解析为 JSON 时，返回 400 错误
		klog.Errorf("Update kubernetes resource (%s:%s:%s) from cluster (%s) error. %v", kind, namespace, name, cluster, err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid resource object"})
		return
	}

	// 获取 Kubernetes 客户端
	kubeClient, err := client.KubeClient(cluster)
	if kubeClient == nil || err != nil {
		klog.Errorf("Failed to get kubeClient for cluster: %s", cluster)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get kubeClient"})
		return
	}

	result, err := kubeClient.Update(kind, namespace, name, &object)
	if err != nil {
		// 记录错误日志并返回 500 错误
		klog.Errorf("Update kubernetes resource (%s:%s:%s) from cluster (%s) error: %v", kind, namespace, name, cluster, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update resource"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": result})
}

// @Title Delete
// @Description delete the resource
// @Param	cluster		path 	string	true		"the cluster want to delete"
// @Param	kind		path 	string	true		"the resource kind"
// @Param	namespace		path 	string	true		"the namespace want to delete"
// @Param	name		path 	string	true		"the name want to delete"
// @Param	force		query 	bool	false		"force to delete the resource from etcd."
// @Success 200 {string} delete success!
// @router /:name [delete]
func Delete(c *gin.Context) {
	// 获取路径参数
	cluster := c.Param("cluster")
	namespace := c.Param("namespaceName")
	name := c.Param("name")
	kind := c.Param("kind")

	// 获取查询参数 force
	force := c.DefaultQuery("force", "")
	// 默认删除选项
	defaultPropagationPolicy := metav1.DeletePropagationBackground
	defaultDeleteOptions := metav1.DeleteOptions{
		PropagationPolicy: &defaultPropagationPolicy,
	}

	// 如果 force 参数存在且为 true，则设置强制删除
	if force != "" {
		forceBool, err := strconv.ParseBool(force)
		if err != nil {
			// 返回400错误，表示 force 参数格式错误
			c.JSON(http.StatusBadRequest, gin.H{"error": "force 参数格式错误"})
			return
		}
		if forceBool {
			// 强制删除时设置为 0
			var gracePeriodSeconds int64 = 0
			defaultDeleteOptions.GracePeriodSeconds = &gracePeriodSeconds
		}
	}

	// 获取 Kubernetes 客户端
	kubeClient, err := client.KubeClient(cluster)
	if kubeClient == nil || err != nil {
		klog.Errorf("Failed to get kubeClient for cluster: %s", cluster)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get kubeClient"})
		return
	}

	err = kubeClient.Delete(kind, namespace, name, &defaultDeleteOptions)
	if err != nil {
		klog.Errorf("Delete kubernetes resource (%s:%s:%s) from cluster (%s) error: %v", kind, namespace, name, cluster, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": "ok!"})
}

func NamespacesList(c *gin.Context) {
	// 获取查询参数
	param := base.BuildQueryParam(c)
	cluster := c.Param("cluster")
	namespace := c.Param("namespace")
	kind := "namespaces"

	// 获取 Kubernetes 客户端
	kubeClient, err := client.KubeClient(cluster)
	if kubeClient == nil || err != nil {
		klog.Errorf("Failed to get kubeClient for cluster: %s", cluster)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get kubeClient"})
		return
	}

	// 调用 proxy.GetPage 来获取资源列表
	result, err := proxy.GetPage(kubeClient, kind, namespace, param)
	if err != nil {
		klog.Errorf("List kubernetes resource (%s:%s) from cluster (%s) error: %v", kind, namespace, cluster, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 返回成功的响应
	c.JSON(http.StatusOK, gin.H{"data": result})
}

func NamespacesGet(c *gin.Context) {
	// 从路由参数中获取变量
	cluster := c.Param("cluster")
	namespace := c.Param("namespace")
	name := c.Param("namespaceName")
	kind := "namespaces"

	klog.Infof("Get cluster: %s, namespace: %s, name: %s, kind: %s", cluster, namespace, name, kind)

	// 获取 Kubernetes 客户端
	kubeClient, err := client.KubeClient(cluster)
	if kubeClient == nil || err != nil {
		klog.Errorf("Failed to get kubeClient for cluster: %s", cluster)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get kubeClient"})
		return
	}

	result, err := kubeClient.Get(kind, namespace, name)
	if err != nil {
		klog.Errorf("Failed to get %s %s/%s from cluster: %s", kind, namespace, name, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": result})
}

func NamespacesCreate(c *gin.Context) {
	// 获取路径参数
	cluster := c.Param("cluster")
	namespace := c.Param("namespace")
	kind := "namespaces"

	// 解析请求体
	var object runtime.Unknown
	if err := c.BindJSON(&object); err != nil {
		// 处理解析错误
		klog.Errorf("Create kubernetes resource (%s:%s) from cluster (%s) error. %v", kind, namespace, cluster, err)
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("Invalid request body: %v", err)})
		return
	}

	// 获取 Kubernetes 客户端
	kubeClient, err := client.KubeClient(cluster)
	if kubeClient == nil || err != nil {
		klog.Errorf("Failed to get kubeClient for cluster: %s", cluster)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get kubeClient"})
		return
	}

	// 调用 Kubernetes 客户端的 Create 方法
	result, err := kubeClient.Create(kind, namespace, &object)
	if err != nil {
		// 处理错误
		klog.Errorf("Error creating resource (%s:%s) in cluster (%s): %v", kind, namespace, cluster, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": result})
}

func NamespacesPut(c *gin.Context) {
	// 获取查询参数
	cluster := c.Param("cluster")
	namespace := c.Param("namespace")
	name := c.Param("namespaceName")
	kind := "namespaces"

	// 解析请求体中的资源对象
	var object runtime.Unknown
	if err := c.ShouldBindJSON(&object); err != nil {
		// 请求体无法解析为 JSON 时，返回 400 错误
		klog.Errorf("Update kubernetes resource (%s:%s:%s) from cluster (%s) error. %v", kind, namespace, name, cluster, err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid resource object"})
		return
	}

	// 获取 Kubernetes 客户端
	kubeClient, err := client.KubeClient(cluster)
	if kubeClient == nil || err != nil {
		klog.Errorf("Failed to get kubeClient for cluster: %s", cluster)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get kubeClient"})
		return
	}

	result, err := kubeClient.Update(kind, namespace, name, &object)
	if err != nil {
		// 记录错误日志并返回 500 错误
		klog.Errorf("Update kubernetes resource (%s:%s:%s) from cluster (%s) error: %v", kind, namespace, name, cluster, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update resource"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": result})
}

func NamespacesDelete(c *gin.Context) {
	// 获取路径参数
	cluster := c.Param("cluster")
	namespace := c.Param("namespace")
	name := c.Param("namespaceName")
	kind := "namespaces"

	// 获取查询参数 force
	force := c.DefaultQuery("force", "")
	// 默认删除选项
	defaultPropagationPolicy := metav1.DeletePropagationBackground
	defaultDeleteOptions := metav1.DeleteOptions{
		PropagationPolicy: &defaultPropagationPolicy,
	}

	// 如果 force 参数存在且为 true，则设置强制删除
	if force != "" {
		forceBool, err := strconv.ParseBool(force)
		if err != nil {
			// 返回400错误，表示 force 参数格式错误
			c.JSON(http.StatusBadRequest, gin.H{"error": "force 参数格式错误"})
			return
		}
		if forceBool {
			// 强制删除时设置为 0
			var gracePeriodSeconds int64 = 0
			defaultDeleteOptions.GracePeriodSeconds = &gracePeriodSeconds
		}
	}

	// 获取 Kubernetes 客户端
	kubeClient, err := client.KubeClient(cluster)
	if kubeClient == nil || err != nil {
		klog.Errorf("Failed to get kubeClient for cluster: %s", cluster)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get kubeClient"})
		return
	}

	err = kubeClient.Delete(kind, namespace, name, &defaultDeleteOptions)
	if err != nil {
		klog.Errorf("Delete kubernetes resource (%s:%s:%s) from cluster (%s) error: %v", kind, namespace, name, cluster, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": "ok!"})
}
