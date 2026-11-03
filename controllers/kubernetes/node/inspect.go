package node

import (
	"context"
	"net/http"
	"sync"

	"github.com/JLPAY/gwayne/models"
	"github.com/JLPAY/gwayne/pkg/config"
	"github.com/JLPAY/gwayne/pkg/inspect"
	"github.com/JLPAY/gwayne/pkg/kubernetes/client"
	k8snode "github.com/JLPAY/gwayne/pkg/kubernetes/resources/node"
	"github.com/gin-gonic/gin"
	"k8s.io/klog/v2"
)

// Inspect 巡检节点（按 DB 中可配置巡检项执行）
// GET /api/v1/kubernetes/nodes/inspect?cluster=UAT
//
// cluster 为空时遍历所有已注册集群。巡检项与阈值取 /api/v1/configs/node-inspect 配置。
func Inspect(c *gin.Context) {
	cluster := c.Query("cluster")
	cfg := models.GetNodeInspectConfig()
	plan := inspect.BuildPlan(cfg)

	if cluster == "" {
		managers := client.Managers()
		var (
			wg      sync.WaitGroup
			mu      sync.Mutex
			results []k8snode.NodeInspectionResult
		)
		managers.Range(func(key, value interface{}) bool {
			clu, ok := key.(string)
			if !ok {
				return true
			}
			wg.Add(1)
			go func(clu string) {
				defer wg.Done()
				r, err := inspectCluster(context.TODO(), clu, plan)
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

	r, err := inspectCluster(context.TODO(), cluster, plan)
	if err != nil {
		klog.Errorf("巡检集群 %s 失败: %v", cluster, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": []k8snode.NodeInspectionResult{*r}})
}

// inspectCluster 巡检单个集群；需要 manager 同时提供 clientset 与 rest.Config（API Server 证书巡检）。
func inspectCluster(ctx context.Context, cluster string, plan map[k8snode.InspectItemType]k8snode.ItemThreshold) (*k8snode.NodeInspectionResult, error) {
	mgr, err := client.Manager(cluster)
	if err != nil {
		return nil, err
	}
	r, err := k8snode.InspectNodes(ctx, mgr.Client,
		config.Conf.Monitor.PrometheusURL,
		config.Conf.Monitor.PrometheusToken,
		mgr.Config, plan)
	if err != nil {
		return nil, err
	}
	r.Cluster = cluster
	return r, nil
}
