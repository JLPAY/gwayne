package node

import (
	"context"
	"net/http"
	"strconv"
	"sync"

	"github.com/JLPAY/gwayne/models"
	"github.com/JLPAY/gwayne/pkg/config"
	"github.com/JLPAY/gwayne/pkg/kubernetes/client"
	k8snode "github.com/JLPAY/gwayne/pkg/kubernetes/resources/node"
	"github.com/gin-gonic/gin"
	"k8s.io/klog/v2"
)

// Inspect 巡检节点内存与磁盘使用率
// GET /api/v1/kubernetes/nodes/inspect?cluster=UAT&warning=80&critical=90
//
// cluster 为空时遍历所有已注册集群；warning/critical 不传则取 DB 中持久化配置，
// 传参则临时覆盖（仅本次请求生效，不写库）。
func Inspect(c *gin.Context) {
	cluster := c.Query("cluster")
	warning, critical := resolveThresholds(c)

	if cluster == "" {
		managers := client.Managers()
		var (
			wg      sync.WaitGroup
			mu      sync.Mutex
			results []k8snode.NodeInspectionResult
		)
		managers.Range(func(key, value interface{}) bool {
			clu := key.(string)
			wg.Add(1)
			go func(clu string) {
				defer wg.Done()
				r, err := inspectCluster(context.TODO(), clu, warning, critical)
				if err != nil {
					klog.Errorf("巡检集群 %s 失败: %v", clu, err)
					mu.Lock()
					results = append(results, k8snode.NodeInspectionResult{Cluster: clu, Error: err.Error()})
					mu.Unlock()
					return
				}
				mu.Lock()
				results = append(results, *r)
				mu.Unlock()
			}(clu)
			return true
		})
		wg.Wait()
		c.JSON(http.StatusOK, gin.H{"data": results})
		return
	}

	r, err := inspectCluster(context.TODO(), cluster, warning, critical)
	if err != nil {
		klog.Errorf("巡检集群 %s 失败: %v", cluster, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": []k8snode.NodeInspectionResult{*r}})
}

// inspectCluster 巡检单个集群，返回该集群结果；集群客户端不可用时返回错误。
func inspectCluster(ctx context.Context, cluster string, warning, critical int) (*k8snode.NodeInspectionResult, error) {
	clientset, err := client.Client(cluster)
	if err != nil {
		return nil, err
	}
	r, err := k8snode.InspectNodes(ctx, clientset, config.Conf.Monitor.PrometheusURL, config.Conf.Monitor.PrometheusToken, warning, critical)
	if err != nil {
		return nil, err
	}
	r.Cluster = cluster
	return r, nil
}

// resolveThresholds 读取 DB 持久化阈值，query 参数可临时覆盖。
func resolveThresholds(c *gin.Context) (warning, critical int) {
	cfg := models.GetNodeInspectConfig()
	warning, critical = cfg.Warning, cfg.Critical
	if w := c.Query("warning"); w != "" {
		if v, err := strconv.Atoi(w); err == nil && v > 0 && v < 100 {
			warning = v
		}
	}
	if cr := c.Query("critical"); cr != "" {
		if v, err := strconv.Atoi(cr); err == nil && v > warning && v <= 100 {
			critical = v
		}
	}
	return
}
