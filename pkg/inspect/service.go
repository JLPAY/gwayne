package inspect

import (
	"context"
	"sync"
	"time"

	"github.com/JLPAY/gwayne/models"
	"github.com/JLPAY/gwayne/pkg/config"
	k8snode "github.com/JLPAY/gwayne/pkg/kubernetes/resources/node"
	"github.com/JLPAY/gwayne/pkg/kubernetes/client"
	"k8s.io/klog/v2"
)

// BuildPlan 将 DB 配置与代码内注册表默认值合并为生效的巡检项阈值表。
func BuildPlan(cfg *models.NodeInspectConfig) map[k8snode.InspectItemType]k8snode.ItemThreshold {
	plan := k8snode.DefaultItemThresholds()
	if cfg == nil {
		return plan
	}
	for _, it := range cfg.ParseItems() {
		t := k8snode.InspectItemType(it.Type)
		if th, ok := plan[t]; ok {
			th.Enabled = it.Enabled
			th.Warning = it.Warning
			th.Critical = it.Critical
			plan[t] = th
		}
	}
	return plan
}

// StartScheduler 启动定时巡检调度器（后台 goroutine，每轮重新读取配置以响应变更）。
func StartScheduler() {
	go func() {
		for {
			cfg := models.GetNodeInspectConfig()
			interval := cfg.ScheduleInterval
			if interval < 1 {
				interval = models.DefaultScheduleInterval
			}
			if !cfg.ScheduleEnabled {
				time.Sleep(time.Duration(interval) * time.Minute)
				continue
			}
			klog.Infof("定时巡检开始（间隔 %d 分钟）", interval)
			RunAllClusters(context.Background())
			time.Sleep(time.Duration(interval) * time.Minute)
		}
	}()
	klog.Info("节点巡检定时调度器已启动")
}

// RunAllClusters 对所有已注册集群执行一次巡检并落历史记录（并发逐集群）。
func RunAllClusters(ctx context.Context) {
	cfg := models.GetNodeInspectConfig()
	plan := BuildPlan(cfg)
	promURL := config.Conf.Monitor.PrometheusURL
	promToken := config.Conf.Monitor.PrometheusToken

	clusters := make([]string, 0, 8)
	client.Managers().Range(func(key, value interface{}) bool {
		if name, ok := key.(string); ok {
			clusters = append(clusters, name)
		}
		return true
	})

	var wg sync.WaitGroup
	for _, c := range clusters {
		wg.Add(1)
		go func(cluster string) {
			defer wg.Done()
			RunOneCluster(ctx, cluster, plan, promURL, promToken)
		}(c)
	}
	wg.Wait()

	// 历史保留期清理
	if cfg.RetentionDays > 0 {
		cut := time.Now().AddDate(0, 0, -cfg.RetentionDays)
		if n, err := models.DeleteNodeInspectHistoryBefore(cut); err != nil {
			klog.Warningf("清理巡检历史失败: %v", err)
		} else if n > 0 {
			klog.Infof("清理 %d 天前的巡检历史 %d 行", cfg.RetentionDays, n)
		}
	}
}

// RunOneCluster 巡检单个集群并保存历史记录。
func RunOneCluster(ctx context.Context, cluster string, plan map[k8snode.InspectItemType]k8snode.ItemThreshold, promURL, promToken string) {
	mgr, err := client.Manager(cluster)
	if err != nil {
		klog.Warningf("巡检集群 %s 获取客户端失败: %v", cluster, err)
		return
	}
	res, err := k8snode.InspectNodes(ctx, mgr.Client, promURL, promToken, mgr.Config, plan)
	if err != nil {
		klog.Errorf("巡检集群 %s 失败: %v", cluster, err)
		return
	}
	runAt := time.Now().Truncate(time.Minute)
	if err := SaveInspectHistory(cluster, runAt, res); err != nil {
		klog.Errorf("保存集群 %s 巡检历史失败: %v", cluster, err)
		return
	}
	klog.Infof("集群 %s 巡检完成: 节点 %d, 告警 %d, 严重 %d", cluster, res.Total, res.Warning, res.Critical)
}

// SaveInspectHistory 将一次巡检结果落历史表（节点×巡检项 逐行；集群级项 node 为空）。
func SaveInspectHistory(cluster string, runAt time.Time, res *k8snode.NodeInspectionResult) error {
	if res == nil {
		return nil
	}
	rows := make([]models.NodeInspectHistory, 0, len(res.Items)*4+len(res.ClusterChecks))
	for _, it := range res.Items {
		for _, ch := range it.Checks {
			rows = append(rows, models.NodeInspectHistory{
				Cluster:   cluster,
				Node:      it.Name,
				CheckType: string(ch.Type),
				Status:    ch.Status,
				Percent:   ch.Percent,
				Value:     ch.Value,
				Detail:    ch.Detail,
				RunAt:     runAt,
			})
		}
	}
	for _, ch := range res.ClusterChecks {
		rows = append(rows, models.NodeInspectHistory{
			Cluster:   cluster,
			Node:      "",
			CheckType: string(ch.Type),
			Status:    ch.Status,
			Percent:   ch.Percent,
			Value:     ch.Value,
			Detail:    ch.Detail,
			RunAt:     runAt,
		})
	}
	if len(rows) == 0 {
		return nil
	}
	return models.DB.CreateInBatches(rows, 500).Error
}
