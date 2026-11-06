package node

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/JLPAY/gwayne/models"
	"github.com/JLPAY/gwayne/pkg/inspect"
	"github.com/JLPAY/gwayne/pkg/kubernetes/client"
	"github.com/JLPAY/gwayne/pkg/notify"
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
	promURL, promToken := models.ResolvePrometheus()
	r, err := k8snode.InspectNodes(ctx, mgr.Client, promURL, promToken, mgr.Config, plan)
	if err != nil {
		return nil, err
	}
	r.Cluster = cluster
	return r, nil
}

// InspectPushWeCom 将用户手动巡检的结果推送到企业微信机器人（推送前置确认，由前端触发）
// POST /api/v1/kubernetes/nodes/inspect/notify
// body: {"results":[NodeInspectionResult,...]}；webhook 地址取已保存配置（客户端不可指定）。
func InspectPushWeCom(c *gin.Context) {
	var body struct {
		Results []k8snode.NodeInspectionResult `json:"results"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || len(body.Results) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "缺少巡检结果数据"})
		return
	}
	cfg := models.GetNodeInspectConfig()
	if !cfg.WebhookEnabled || cfg.WebhookURL == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "企微机器人推送未启用或未配置 Webhook 地址"})
		return
	}
	md := inspect.BuildInspectReportMarkdown(body.Results, time.Now())
	if err := notify.SendWeComMarkdown(cfg.WebhookURL, md); err != nil {
		klog.Errorf("手动推送巡检报告到企微失败: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	klog.Infof("手动推送巡检报告到企微成功（%d 个集群）", len(body.Results))
	c.JSON(http.StatusOK, gin.H{"data": "已推送到企业微信群"})
}
