package persistentvolume

import (
	"fmt"
	"github.com/JLPAY/gwayne/pkg/kubernetes/client"
	"github.com/JLPAY/gwayne/pkg/kubernetes/resources/persistentvolume"
	"github.com/gin-gonic/gin"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
	"net/http"
)

// @Title List pv
// @Description find pv by cluster
// @router /clusters/:cluster [get]
func List(c *gin.Context) {
	// 获取查询参数
	cluster := c.Param("cluster")

	// 获取 Kubernetes 客户端
	manager, err := client.Manager(cluster)
	if err != nil {
		klog.Errorf("list cluster %s error: %v", cluster, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 调用 proxy.GetPage 来获取资源列表
	result, err := persistentvolume.ListPersistentVolume(manager.Client, metav1.ListOptions{})
	if err != nil {
		klog.Errorf("list pv by cluster (%s) error: %v", cluster, err)
		c.JSON(http.StatusInternalServerError, fmt.Errorf("list pv by cluster (%s) error: %v", cluster, err))
		return
	}

	// 返回成功的响应
	c.JSON(http.StatusOK, gin.H{"data": result})

}

// @Title get pv
// @Description find pv by cluster
// @router /:name/clusters/:cluster [get]
func Get(c *gin.Context) {
	// 获取查询参数
	cluster := c.Param("cluster")
	name := c.Param("name")

	// 获取 Kubernetes 客户端
	manager, err := client.Manager(cluster)
	if err != nil {
		klog.Errorf("list cluster %s error: %v", cluster, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	result, err := persistentvolume.GetPersistentVolumeByName(manager.Client, name)
	if err != nil {
		klog.Errorf("get pv by cluster (%s) error: %v", cluster, err)
		c.JSON(http.StatusInternalServerError, fmt.Errorf("get pv by cluster (%s) error: %v", cluster, err))
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": result})
}

// @Title Create
// @Description create PersistentVolume
// @router /clusters/:cluster [post]
func Create(c *gin.Context) {
	cluster := c.Param("cluster")

	// 获取请求体并解码为 PersistentVolume 对象
	var pvTpl corev1.PersistentVolume
	if err := c.ShouldBindJSON(&pvTpl); err != nil {
		klog.Errorf("bind pv error: %v", err)
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

	result, err := persistentvolume.CreatePersistentVolume(manager.Client, &pvTpl)
	if err != nil {
		klog.Errorf("create pv error: %v", err)
		c.JSON(http.StatusInternalServerError, fmt.Errorf("create pv error: %v", err))
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": result})
}

// @Title Update
// @Description update the PersistentVolume
// @router /:name/clusters/:cluster [put]
func Update(c *gin.Context) {
	cluster := c.Param("cluster")
	name := c.Param("name")

	var pvTpl corev1.PersistentVolume
	if err := c.ShouldBindJSON(&pvTpl); err != nil {
		klog.Errorf("bind pv error: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if name != pvTpl.Name {
		klog.Errorf("name not match, expect: %s, actual: %s", pvTpl.Name, name)
		c.JSON(http.StatusBadRequest, gin.H{"error": "name not match"})
		return
	}

	manager, err := client.Manager(cluster)
	if err != nil {
		klog.Errorf("list cluster %s error: %v", cluster, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	result, err := persistentvolume.UpdatePersistentVolume(manager.Client, &pvTpl)
	if err != nil {
		klog.Errorf("update pv error: %v", err)
		c.JSON(http.StatusInternalServerError, fmt.Errorf("update pv error: %v", err))
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": result})
}

// @Title Delete
// @Description delete the PersistentVolume
// @Success 200 {string} delete success!
// @router /:name/clusters/:cluster [delete]
func Delete(c *gin.Context) {
	cluster := c.Param("cluster")
	name := c.Param("name")

	manager, err := client.Manager(cluster)
	if err != nil {
		klog.Errorf("list cluster %s error: %v", cluster, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	err = persistentvolume.DeletePersistentVolume(manager.Client, name)
	if err != nil {
		klog.Errorf("delete pv by cluster (%s) error: %v", cluster, err)
		c.JSON(http.StatusInternalServerError, fmt.Errorf("delete pv by cluster (%s) error: %v", cluster, err))
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": "ok!"})
}
