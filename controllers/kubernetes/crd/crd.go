package crd

import (
	"context"
	"github.com/JLPAY/gwayne/controllers/base"
	"github.com/JLPAY/gwayne/pkg/kubernetes/client"
	"github.com/JLPAY/gwayne/pkg/kubernetes/resources/crd"
	"github.com/gin-gonic/gin"
	apiextensions "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
	"net/http"
)

// @Title List CRD
// @Description find CRD by cluster
// @router / [get]
func List(c *gin.Context) {
	// 构建 Kubernetes 查询参数
	param := base.BuildQueryParam(c)
	cluster := c.Param("cluster")

	// 获取 Kubernetes 客户端
	manager, err := client.Manager(cluster)
	if err != nil {
		klog.Errorf("list cluster %s error: %v", cluster, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	result, err := crd.GetCRDPage(manager.CrdClient, param)
	if err != nil {
		klog.Errorf("list cluster %s error: %v", cluster, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": result})
}

// @Title get CRD
// @Description find CRD by cluster
// @router /:name [get]
func Get(c *gin.Context) {
	cluster := c.Param("cluster")
	name := c.Param("name")

	// 获取 Kubernetes 客户端
	manager, err := client.Manager(cluster)
	if err != nil {
		klog.Errorf("list cluster %s error: %v", cluster, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	result, err := manager.CrdClient.ApiextensionsV1().CustomResourceDefinitions().Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		klog.Errorf("list cluster %s error: %v", cluster, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": result})
}

// @Title Create
// @Description create CustomResourceDefinition
// @router / [post]
func Create(c *gin.Context) {
	cluster := c.Param("cluster")

	var tpl apiextensions.CustomResourceDefinition
	err := c.ShouldBindJSON(&tpl)
	if err != nil {
		klog.Errorf("create cluster %s error: %v", cluster, err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	manager, err := client.Manager(cluster)
	if err != nil {
		klog.Errorf("list cluster %s error: %v", cluster, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	result, err := manager.CrdClient.ApiextensionsV1().CustomResourceDefinitions().Create(context.TODO(), &tpl, metav1.CreateOptions{})
	if err != nil {
		klog.Errorf("create cluster %s error: %v", cluster, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": result})
}

// @Title Update
// @Description update the CustomResourceDefinition
// @router /:name [put]
func Update(c *gin.Context) {
	cluster := c.Param("cluster")
	name := c.Param("name")
	var tpl apiextensions.CustomResourceDefinition
	err := c.BindJSON(&tpl)
	if err != nil {
		klog.Errorf("update crd bind crd %s error: %v", name, err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	manager, err := client.Manager(cluster)
	if err != nil {
		klog.Errorf("list cluster %s error: %v", cluster, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	result, err := manager.CrdClient.ApiextensionsV1().CustomResourceDefinitions().Update(context.TODO(), &tpl, metav1.UpdateOptions{})
	if err != nil {
		klog.Errorf("update crd %s error: %v", name, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": result})
}

// @Title Delete
// @Description delete the CustomResourceDefinition
// @Success 200 {string} delete success!
// @router /:name [delete]
func Delete(c *gin.Context) {
	cluster := c.Param("cluster")
	name := c.Param("name")

	manager, err := client.Manager(cluster)
	if err != nil {
		klog.Errorf("list cluster %s error: %v", cluster, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	err = manager.CrdClient.ApiextensionsV1().CustomResourceDefinitions().Delete(context.TODO(), name, metav1.DeleteOptions{})
	if err != nil {
		klog.Errorf("delete crd %s error: %v", name, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": "ok!"})
}
