package crd

import (
	"net/http"

	"github.com/JLPAY/gwayne/controllers/base"
	"github.com/JLPAY/gwayne/pkg/kubernetes/client"
	"github.com/JLPAY/gwayne/pkg/kubernetes/resources/crd"
	"github.com/gin-gonic/gin"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/klog/v2"
)

// @Title List CRD
// @Description find CRD by cluster
// @Param	namespace		path 	string	true		"the namespace name"
// @router / [get]
func CRDList(c *gin.Context) {
	// 构建 Kubernetes 查询参数
	param := base.BuildQueryParam(c)
	cluster := c.Param("cluster")
	group := c.Param("group")
	kind := c.Param("kind")
	namespace := c.Param("namespacesName")

	// 获取 Kubernetes 客户端
	manager, err := client.Manager(cluster)
	if err != nil {
		klog.Errorf("list cluster %s error: %v", cluster, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	result, err := crd.GetCustomCRDPage(manager.CrdClient, manager.DynamicClient, group, kind, namespace, param)
	if err != nil {
		klog.Errorf("list cluster %s error: %v", cluster, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": result})
}

// @Title get CRD
// @Description find CRD by cluster
// @Param	namespace		path 	string	true		"the namespace name"
// @router /:name [get]
func CRDGet(c *gin.Context) {
	cluster := c.Param("cluster")
	namespace := c.Param("namespacesName")
	name := c.Param("name")
	group := c.Param("group")
	version := c.Param("version")
	kind := c.Param("kind")

	// 获取 Kubernetes 客户端
	manager, err := client.Manager(cluster)
	if err != nil {
		klog.Errorf("list cluster %s error: %v", cluster, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if version == "" || version == "undefined" {
		crdVersion, err := crd.GetBestCRDVersionByGroupKind(manager.CrdClient, group, kind)
		if err != nil {
			klog.Errorf("list cluster %s error: %v", cluster, err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		version = crdVersion.Name
	}

	klog.V(2).Infof("crd version: %s, namespace: %s", version, namespace)

	result, err := crd.GetCustomCRDInstanceByName(manager.DynamicClient, group, version, kind, namespace, name)
	if err != nil {
		klog.Errorf("list cluster %s error: %v", cluster, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": result})
}

// @Title Create
// @Description create CustomResourceDefinition instance (cluster-scoped)
// @router / [post]
func CRDCreate(c *gin.Context) {
	cluster := c.Param("cluster")
	group := c.Param("group")
	version := c.Param("version")
	kind := c.Param("kind")

	// 获取 Kubernetes 客户端
	manager, err := client.Manager(cluster)
	if err != nil {
		klog.Errorf("list cluster %s error: %v", cluster, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 处理版本为undefined的情况
	if version == "" || version == "undefined" {
		crdVersion, err := crd.GetBestCRDVersionByGroupKind(manager.CrdClient, group, kind)
		if err != nil {
			klog.Errorf("list cluster %s error: %v", cluster, err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		version = crdVersion.Name
	}

	klog.V(2).Infof("creating cluster-scoped CRD instance group: %s, version: %s, kind: %s", group, version, kind)

	result, err := crd.CreateCustomCRDInstanceClusterScoped(manager.DynamicClient, group, version, kind, c.Request.Body)
	if err != nil {
		klog.Errorf("create cluster %s error: %v", cluster, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": result})
}

// @Title CreateWithNamespace
// @Description create CustomResourceDefinition with namespace
// @Param	namespace		path 	string	true		"the namespace name"
// @router / [post]
func CRDCreateWithNamespace(c *gin.Context) {
	cluster := c.Param("cluster")
	namespace := c.Param("namespacesName")
	group := c.Param("group")
	version := c.Param("version")
	kind := c.Param("kind")

	// 获取 Kubernetes 客户端
	manager, err := client.Manager(cluster)
	if err != nil {
		klog.Errorf("list cluster %s error: %v", cluster, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 处理版本为undefined的情况
	if version == "" || version == "undefined" {
		crdVersion, err := crd.GetBestCRDVersionByGroupKind(manager.CrdClient, group, kind)
		if err != nil {
			klog.Errorf("list cluster %s error: %v", cluster, err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		version = crdVersion.Name
	}

	klog.V(2).Infof("crd version: %s, namespace: %s", version, namespace)

	result, err := crd.CreateCustomCRDInstance(manager.DynamicClient, group, version, kind, namespace, c.Request.Body)
	if err != nil {
		klog.Errorf("create cluster %s error: %v", cluster, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": result})
}

// @Title Update
// @Description update the CustomResourceDefinition
// @Param	namespace		path 	string	true		"the namespace name"
// @router /:name [put]
func CRDUpdate(c *gin.Context) {
	cluster := c.Param("cluster")
	namespace := c.Param("namespacesName")
	name := c.Param("name")
	group := c.Param("group")
	version := c.Param("version")
	kind := c.Param("kind")

	var object runtime.Unknown
	err := c.BindJSON(&object)
	if err != nil {
		klog.Errorf("create cluster %s error: %v", cluster, err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 获取 Kubernetes 客户端
	manager, err := client.Manager(cluster)
	if err != nil {
		klog.Errorf("list cluster %s error: %v", cluster, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if version == "" || version == "undefined" {
		crdVersion, err := crd.GetBestCRDVersionByGroupKind(manager.CrdClient, group, kind)
		if err != nil {
			klog.Errorf("list cluster %s error: %v", cluster, err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		version = crdVersion.Name
	}

	klog.V(2).Infof("crd version: %s, namespace: %s", version, namespace)

	result, err := crd.UpdateCustomCRD(manager.DynamicClient, group, version, kind, namespace, name, &object)
	if err != nil {
		klog.Errorf("create cluster %s error: %v", cluster, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": result})
}

// @Title Delete
// @Description delete the CustomResourceDefinition
// @Param	namespace		path 	string	true		"the namespace name"
// @Success 200 {string} delete success!
// @router /:name [delete]
func CRDDelete(c *gin.Context) {
	cluster := c.Param("cluster")
	namespace := c.Param("namespacesName")
	name := c.Param("name")
	group := c.Param("group")
	version := c.Param("version")
	kind := c.Param("kind")

	manager, err := client.Manager(cluster)
	if err != nil {
		klog.Errorf("list cluster %s error: %v", cluster, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if version == "" || version == "undefined" {
		crdVersion, err := crd.GetBestCRDVersionByGroupKind(manager.CrdClient, group, kind)
		if err != nil {
			klog.Errorf("list cluster %s error: %v", cluster, err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		version = crdVersion.Name
	}

	err = crd.DeleteCustomCRD(manager.DynamicClient, group, version, kind, namespace, name)
	if err != nil {
		klog.Errorf("delete cluster %s error: %v", cluster, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": "ok!"})
}
