package cluster

import (
	"github.com/JLPAY/gwayne/controllers/base"
	"github.com/JLPAY/gwayne/models"
	"github.com/gin-gonic/gin"
	"k8s.io/klog/v2"
	"net/http"
)

// 获取所有集群的 id 和名称
func GetNames(c *gin.Context) {
	// 处理列出集群的逻辑
	deleted := c.DefaultQuery("deleted", "false") == "true"

	clusters, err := models.GetClusterNames(deleted)

	if err != nil {
		klog.Error("get names error. %v, delete-status %v", err, deleted)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Unable to fetch cluster names"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": clusters})
}

func Create(c *gin.Context) {
	var cluster models.Cluster

	if err := c.ShouldBind(&cluster); err != nil {
		klog.Error("Invalid param body, create cluster error. %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid JSON format for create cluster"})
		return
	}

	user := c.MustGet("User").(*models.User)
	cluster.User = user.Name

	objectid, err := models.AddCluster(&cluster)
	if err != nil {
		klog.Errorf("create cluster error: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": objectid})
}

// Update handles the update of a cluster by name
func Update(c *gin.Context) {
	name := c.Param("name")

	var cluster models.Cluster
	if err := c.ShouldBind(&cluster); err != nil {
		klog.Error("Invalid param body, update cluster error. %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	cluster.Name = name
	err := models.UpdateClusterByName(&cluster)
	if err != nil {
		klog.Error("UpdateClusterByName error. %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": cluster})
}

// Get handles getting a cluster by name
func Get(c *gin.Context) {
	name := c.Param("name")

	cluster, err := models.GetClusterByName(name)
	if err != nil {
		klog.Error("get GetClusterByName error. %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	user := c.MustGet("User").(*models.User)
	// 假设 User.Admin 是用户的权限
	if !user.Admin {
		cluster.KubeConfig = ""
	}

	c.JSON(http.StatusOK, gin.H{"data": cluster})
}

// 获取集群列表
func List(c *gin.Context) {
	param := base.BuildQueryParam(c)

	clusters := []models.Cluster{}
	err := models.GetAll(new(models.Cluster), &clusters, param)
	if err != nil {
		klog.Errorf("list by param (%v) error. %v", param, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Unable to get total count"})
		return
	}

	// 非 admin 用户不允许查看 kubeconfig 配置

	if !c.MustGet("User").(*models.User).Admin {
		klog.V(2).Infof("User: %s 不是admin，没有权限。", c.MustGet("User").(*models.User).Name)
		for _, cluster := range clusters {
			cluster.KubeConfig = ""
		}
	}

	// 返回分页结果
	c.JSON(http.StatusOK, gin.H{"data": param.NewPage(int64(len(clusters)), clusters)})

}

// Delete handles the deletion of a cluster by name
func Delete(c *gin.Context) {
	name := c.Param("name")

	// 如果 logical= true 则执行软删除，默认为 true，否则执行物理删除
	logical := c.DefaultQuery("logical", "true") == "true"

	// 处理删除集群的逻辑
	err := models.DeleteClusterByName(name, logical)
	if err != nil {
		klog.Error("delete cluster error. %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": nil})
}
