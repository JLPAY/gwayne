package node

import (
	"fmt"
	"net/http"
	"regexp"
	"sync"

	"github.com/JLPAY/gwayne/pkg/kubernetes/client"
	"github.com/JLPAY/gwayne/pkg/kubernetes/resources/node"
	"github.com/gin-gonic/gin"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/klog/v2"
)

// 用于表示标签的结构
type Label struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type LabelSet struct {
	Labels []Label `json:"Labels"`
}

// 获取节点统计信息
func NodeStatistics(c *gin.Context) {
	cluster := c.Query("cluster")
	total := 0
	countSyncMap := sync.Map{}
	countMap := make(map[string]int)

	if cluster == "" {
		managers := client.Managers()
		var errs []error
		wg := sync.WaitGroup{}

		managers.Range(func(key, value interface{}) bool {
			manager := value.(*client.ClusterManager)
			clu := key.(string)
			// 检查 CacheFactory 是否已初始化
			if manager.CacheFactory == nil {
				klog.Warningf("CacheFactory is nil for cluster %s, skipping", clu)
				return true
			}
			wg.Add(1)
			go func(clu string, manager *client.ClusterManager) {
				defer wg.Done()
				count, err := node.GetNodeCounts(manager.CacheFactory)
				if err != nil {
					klog.Errorf("Failed to get node count for cluster %s: %v", clu, err)
					errs = append(errs, err)
				} else {
					total += count
					countSyncMap.Store(clu, count)
				}
			}(clu, manager)
			return true
		})

		wg.Wait()

		if len(errs) > 0 {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": errors.NewAggregate(errs).Error(),
			})
			return
		}

		countSyncMap.Range(func(key, value interface{}) bool {
			countMap[key.(string)] = value.(int)
			return true
		})
	} else {
		manager, err := client.Manager(cluster)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid cluster"})
			return
		}

		// 检查 CacheFactory 是否已初始化
		if manager.CacheFactory == nil {
			klog.Errorf("CacheFactory is nil for cluster %s", cluster)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "CacheFactory is not initialized for this cluster"})
			return
		}

		count, err := node.GetNodeCounts(manager.CacheFactory)
		if err != nil {
			klog.Errorf("Failed to get node count for cluster %s: %v", cluster, err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		total += count
		countMap[cluster] = count
	}

	c.JSON(http.StatusOK, gin.H{"data": node.NodeStatistics{
		Total:   total,
		Details: countMap,
	}})
}

// @router /:name/clusters/:cluster [get]
// 获取单个节点的信息
func Get(c *gin.Context) {
	name := c.Param("name")
	cluster := c.Param("cluster")

	client, err := client.Client(cluster)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	result, err := node.GetNodeByName(client, name)
	if err != nil {
		klog.Errorf("Failed to get node %s: %v", name, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data": result,
	})
}

// Update 更新节点信息
// @router /:name/clusters/:cluster [put]
func Update(c *gin.Context) {
	name := c.Param("name")
	cluster := c.Param("cluster")

	// 解析请求体
	var nodeTpl corev1.Node
	if err := c.ShouldBindJSON(&nodeTpl); err != nil {
		klog.Errorf("Failed to bind request body for update node: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 校验名称是否一致
	if name != nodeTpl.Name {
		klog.Errorf("Node name mismatch: URL name (%s), body name (%s)", name, nodeTpl.Name)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Node name mismatch"})
		return
	}

	client, err := client.Client(cluster)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	result, err := node.UpdateNode(client, &nodeTpl)
	if err != nil {
		klog.Errorf("Failed to update node %s: %v", name, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	// 模拟更新节点信息
	c.JSON(http.StatusOK, gin.H{
		"data": result,
	})
}

// Delete 删除节点
func Delete(c *gin.Context) {
	name := c.Param("name")
	cluster := c.Param("cluster")

	client, err := client.Client(cluster)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 删除节点
	err = node.DeleteNode(client, name)
	if err != nil {
		klog.Errorf("Failed to delete node %s: %v", name, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": "ok!"})
}

// @Title 添加标签 node label
// @Description add a label for a node
// @router /:name/clusters/:cluster/label [post]
func AddLabel(c *gin.Context) {
	// 获取路径参数
	cluster := c.Param("cluster") // 集群名称
	name := c.Param("name")       // 节点名称

	// 解析请求体以获取标签信息
	var label Label
	if err := c.ShouldBindJSON(&label); err != nil {
		klog.Errorf("Failed to parse label data: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid label data"})
		return
	}

	// 校验标签键和值是否符合 Kubernetes 规范
	if err := validateLabel(label.Key, label.Value); err != nil {
		klog.Errorf("Invalid label key or value: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	client, err := client.Client(cluster)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 获取指定节点信息
	nodeInfo, err := node.GetNodeByName(client, name)
	if err != nil {
		klog.Errorf("Failed to get node %s: %v", name, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 初始化或更新节点标签
	if nodeInfo.ObjectMeta.Labels == nil {
		nodeInfo.ObjectMeta.Labels = map[string]string{}
	}
	nodeInfo.ObjectMeta.Labels[label.Key] = label.Value

	// 更新节点信息
	newNode, err := node.UpdateNode(client, nodeInfo)
	if err != nil {
		klog.Errorf("Failed to update node %s: %v", name, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data": newNode,
	})
}

// DeleteLabel 删除标签
func DeleteLabel(c *gin.Context) {
	// 获取路径参数
	cluster := c.Param("cluster") // 集群名称
	name := c.Param("name")       // 节点名称

	// 解析请求体以获取标签信息
	var label Label
	if err := c.ShouldBindJSON(&label); err != nil {
		klog.Errorf("Failed to parse label data: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid label data"})
		return
	}

	client, err := client.Client(cluster)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 获取指定节点信息
	nodeInfo, err := node.GetNodeByName(client, name)
	if err != nil {
		klog.Errorf("Failed to get node %s: %v", name, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 删除标签
	if _, exists := nodeInfo.ObjectMeta.Labels[label.Key]; exists {
		delete(nodeInfo.ObjectMeta.Labels, label.Key)
	} else {
		klog.Errorf("Label key does not exist: %s\n", label.Key)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Label key does not exist"})
		return
	}

	// 更新节点
	updatedNode, err := node.UpdateNode(client, nodeInfo)
	if err != nil {
		fmt.Printf("Error updating node: %v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update node"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data": updatedNode,
	})
}

// @Title 获取标签 node labels
// @Description get labels of a node
// @router /:name/clusters/:cluster/labels [get]
func GetLabels(c *gin.Context) {
	name := c.Param("name")
	cluster := c.Param("cluster")

	client, err := client.Client(cluster)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	result, err := node.GetNodeByName(client, name)
	if err != nil {
		klog.Errorf("Failed to get node %s: %v", name, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
	}

	labels := result.ObjectMeta.Labels

	c.JSON(http.StatusOK, gin.H{
		"data": labels,
	})
}

// @Title node添加多个标签
// @Description Add labels in bulk for node
// @router /:name/clusters/:cluster/labels [post]
func AddLabels(c *gin.Context) {
	// 获取路径参数
	cluster := c.Param("cluster") // 集群名称
	name := c.Param("name")       // 节点名称

	// 解析请求体以获取标签信息
	var labels LabelSet
	if err := c.ShouldBindJSON(&labels); err != nil {
		klog.Errorf("Failed to parse label data: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid label data"})
		return
	}

	client, err := client.Client(cluster)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 获取指定节点信息
	nodeInfo, err := node.GetNodeByName(client, name)
	if err != nil {
		klog.Errorf("Failed to get node %s: %v", name, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 初始化或更新节点标签
	if nodeInfo.ObjectMeta.Labels == nil {
		nodeInfo.ObjectMeta.Labels = map[string]string{}
	}
	for _, label := range labels.Labels {
		nodeInfo.ObjectMeta.Labels[label.Key] = label.Value
	}

	// 更新节点信息
	updatedNode, err := node.UpdateNode(client, nodeInfo)
	if err != nil {
		klog.Errorf("Failed to update node %s: %v", name, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data": updatedNode,
	})
}

// @Title 删除多个标签
// @Description Delete node labels in batches
// @router /:name/clusters/:cluster/labels [delete]
func DeleteLabels(c *gin.Context) {
	// 获取路径参数
	cluster := c.Param("cluster") // 集群名称
	name := c.Param("name")       // 节点名称

	// 解析请求体以获取标签信息
	var labels LabelSet
	if err := c.ShouldBindJSON(&labels); err != nil {
		klog.Errorf("Failed to parse label data: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid label data"})
		return
	}

	client, err := client.Client(cluster)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 获取指定节点信息
	nodeInfo, err := node.GetNodeByName(client, name)
	if err != nil {
		klog.Errorf("Failed to get node %s: %v", name, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 删除标签
	for _, label := range labels.Labels {
		if _, exists := nodeInfo.ObjectMeta.Labels[label.Key]; exists {
			delete(nodeInfo.ObjectMeta.Labels, label.Key)
		} else {
			klog.Errorf("Label key does not exist: %s\n", label.Key)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Label key does not exist"})
			return
		}
	}

	// 更新节点
	updatedNode, err := node.UpdateNode(client, nodeInfo)
	if err != nil {
		fmt.Printf("Error updating node: %v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update node"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data": updatedNode,
	})
}

// @Title 设置 taint
// @Description set taint for a node
// @router /:name/clusters/:cluster/taint [post]
func SetTaint(c *gin.Context) {
	// 获取路径参数
	cluster := c.Param("cluster") // 集群名称
	name := c.Param("name")       // 节点名称

	// 解析请求体以获取标签信息
	var taint corev1.Taint
	if err := c.ShouldBindJSON(&taint); err != nil {
		klog.Errorf("Failed to parse label data: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid label data"})
		return
	}

	client, err := client.Client(cluster)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 获取指定节点信息
	nodeInfo, err := node.GetNodeByName(client, name)
	if err != nil {
		klog.Errorf("Failed to get node %s: %v", name, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 设置污点
	if nodeInfo.Spec.Taints == nil {
		nodeInfo.Spec.Taints = []corev1.Taint{}
	}
	nodeInfo.Spec.Taints = append(nodeInfo.Spec.Taints, taint)

	// 更新节点
	updatedNode, err := node.UpdateNode(client, nodeInfo)
	if err != nil {
		fmt.Printf("Error updating node: %v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update node"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data": updatedNode,
	})
}

// @Title 删除 taint
// @Description delete a taint from node
// @router /:name/clusters/:cluster/taint [delete]
func DeleteTaint(c *gin.Context) {
	// 获取路径参数
	cluster := c.Param("cluster") // 集群名称
	name := c.Param("name")       // 节点名称

	// 解析请求体以获取标签信息
	var taint corev1.Taint
	if err := c.ShouldBindJSON(&taint); err != nil {
		klog.Errorf("Failed to parse label data: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid label data"})
		return
	}

	client, err := client.Client(cluster)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 获取指定节点信息
	nodeInfo, err := node.GetNodeByName(client, name)
	if err != nil {
		klog.Errorf("Failed to get node %s: %v", name, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 检查并删除污点
	taints := nodeInfo.Spec.Taints
	if len(taints) == 0 {
		klog.Errorf("No taints found on the node")
		c.JSON(http.StatusBadRequest, gin.H{"error": "No taints found on the node"})
		return
	}

	newTaints := []corev1.Taint{}
	for _, t := range taints {
		// 仅保留不匹配的污点
		if t.Key != taint.Key || t.Value != taint.Value || t.Effect != taint.Effect {
			newTaints = append(newTaints, t)
		}
	}

	// 更新污点
	nodeInfo.Spec.Taints = newTaints

	updatedNode, err := node.UpdateNode(client, nodeInfo)
	if err != nil {
		fmt.Printf("Error updating node: %v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update node"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data": updatedNode,
	})
}

// List 获取节点列表
func List(c *gin.Context) {
	// 获取 URL 参数中的 cluster 名称
	cluster := c.Param("cluster")

	// 获取集群管理器
	manager, err := client.Manager(cluster)
	if err != nil {
		klog.Errorf("Failed to get cluster manager for cluster: %s, error: %v", cluster, err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": err,
		})
		return
	}

	result, err := node.ListNode(manager.CacheFactory)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": err,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data": result,
	})
}

// validateLabel 校验标签键和值是否符合 Kubernetes 的规范
func validateLabel(key, value string) error {
	// 校验 Key 的格式
	keyRegex := `^([A-Za-z0-9][-A-Za-z0-9_.]*)?[A-Za-z0-9]$`
	if len(key) > 253 || !regexp.MustCompile(keyRegex).MatchString(key) {
		return fmt.Errorf("invalid label key: %s", key)
	}

	// 校验 Value 的格式
	valueRegex := `^([A-Za-z0-9][-A-Za-z0-9_.]*)?[A-Za-z0-9]$`
	if len(value) > 63 || !regexp.MustCompile(valueRegex).MatchString(value) {
		return fmt.Errorf("invalid label value: %s", value)
	}

	return nil
}

// Cordon 隔离节点
// @router /:name/clusters/:cluster/cordon [put]
func Cordon(c *gin.Context) {
	name := c.Param("name")
	cluster := c.Param("cluster")

	client, err := client.Client(cluster)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "error",
			"data": gin.H{
				"message": "获取集群客户端失败: " + err.Error(),
			},
		})
		return
	}

	// 获取当前节点信息
	currentNode, err := node.GetNodeByName(client, name)
	if err != nil {
		klog.Errorf("Failed to get node %s: %v", name, err)
		c.JSON(http.StatusNotFound, gin.H{
			"code":    404,
			"message": "error",
			"data": gin.H{
				"message": "节点不存在: " + err.Error(),
			},
		})
		return
	}

	// 检查节点是否已经被隔离
	if currentNode.Spec.Unschedulable {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "error",
			"data": gin.H{
				"message": "节点已经被隔离",
			},
		})
		return
	}

	// 设置节点为不可调度（隔离）
	currentNode.Spec.Unschedulable = true

	// 更新节点
	result, err := node.UpdateNode(client, currentNode)
	if err != nil {
		klog.Errorf("Failed to cordon node %s: %v", name, err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "error",
			"data": gin.H{
				"message": "节点隔离失败: " + err.Error(),
			},
		})
		return
	}

	// 返回成功响应
	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "success",
		"data": gin.H{
			"message": "节点隔离成功",
			"node":    result,
		},
	})
}

// Cordon 隔离节点
// @router /:name/clusters/:cluster/cordon [put]
func UnCordon(c *gin.Context) {
	name := c.Param("name")
	cluster := c.Param("cluster")

	client, err := client.Client(cluster)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "error",
			"data": gin.H{
				"message": "获取集群客户端失败: " + err.Error(),
			},
		})
		return
	}

	// 获取当前节点信息
	currentNode, err := node.GetNodeByName(client, name)
	if err != nil {
		klog.Errorf("Failed to get node %s: %v", name, err)
		c.JSON(http.StatusNotFound, gin.H{
			"code":    404,
			"message": "error",
			"data": gin.H{
				"message": "节点不存在: " + err.Error(),
			},
		})
		return
	}

	// 检查节点是否已经被隔离
	if !currentNode.Spec.Unschedulable {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "error",
			"data": gin.H{
				"message": "节点未被隔离，无需解除隔离",
			},
		})
		return
	}

	// 设置节点为不可调度（隔离）
	currentNode.Spec.Unschedulable = false

	// 更新节点
	result, err := node.UpdateNode(client, currentNode)
	if err != nil {
		klog.Errorf("Failed to cordon node %s: %v", name, err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "error",
			"data": gin.H{
				"message": "节点解除隔离失败: " + err.Error(),
			},
		})
		return
	}

	// 返回成功响应
	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "success",
		"data": gin.H{
			"message": "节点解除隔离成功",
			"node":    result,
		},
	})
}

// 删除重复的DrainOptions定义，使用pkg中的类型

// DrainNode 驱逐节点上的所有Pod
// @router /:name/clusters/:cluster/drain [post]
func DrainNode(c *gin.Context) {
	name := c.Param("name")
	cluster := c.Param("cluster")

	// 解析请求体
	var drainOptions node.DrainOptions
	if err := c.ShouldBindJSON(&drainOptions); err != nil {
		klog.Errorf("Failed to parse drain options: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "error",
			"data": gin.H{
				"message": "驱逐选项解析失败: " + err.Error(),
			},
		})
		return
	}

	client, err := client.Client(cluster)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "error",
			"data": gin.H{
				"message": "获取集群客户端失败: " + err.Error(),
			},
		})
		return
	}

	// 执行节点驱逐
	err = node.DrainNode(client, name, &drainOptions)
	if err != nil {
		klog.Errorf("Failed to drain node %s: %v", name, err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "error",
			"data": gin.H{
				"message": "节点驱逐失败: " + err.Error(),
			},
		})
		return
	}

	// 返回成功响应
	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "success",
		"data": gin.H{
			"message": "节点驱逐成功",
		},
	})
}
