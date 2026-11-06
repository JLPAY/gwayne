package inspect

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/JLPAY/gwayne/models"
	"github.com/JLPAY/gwayne/pkg/notify"
	"github.com/robfig/cron/v3"
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

// nextRunTime 计算下一次巡检触发时间：优先 crontab 表达式，非法/为空时回退到固定间隔。
func nextRunTime(cfg *models.NodeInspectConfig, now time.Time) time.Time {
	if spec := strings.TrimSpace(cfg.ScheduleCron); spec != "" {
		s, err := cron.ParseStandard(spec)
		if err == nil {
			return s.Next(now)
		}
		klog.Warningf("非法 cron 表达式 %q: %v，回退到固定间隔", spec, err)
	}
	interval := cfg.ScheduleInterval
	if interval < 1 {
		interval = models.DefaultScheduleInterval
	}
	return now.Add(time.Duration(interval) * time.Minute)
}

// StartScheduler 启动定时巡检调度器（后台 goroutine；每轮触发前重新读取配置以响应变更）。
// 调度方式按 crontab 设计：配置了合法 cron 表达式（如 "0 */6 * * *"）时按表达式触发，
// 否则按固定间隔（分钟）。未启用时低频轮询配置，开启后下一轮即生效。
func StartScheduler() {
	go func() {
		for {
			cfg := models.GetNodeInspectConfig()
			if !cfg.ScheduleEnabled {
				time.Sleep(30 * time.Second)
				continue
			}
			next := nextRunTime(cfg, time.Now())
			klog.Infof("定时巡检将在 %s 触发（cron=%q, 间隔=%d分钟）",
				next.Format("2006-01-02 15:04:05"), cfg.ScheduleCron, cfg.ScheduleInterval)
			time.Sleep(time.Until(next))

			// 触发前重读配置：可能已被关闭
			if latest := models.GetNodeInspectConfig(); !latest.ScheduleEnabled {
				klog.Info("定时巡检已被关闭，本轮跳过")
				continue
			}
			klog.Info("定时巡检开始")
			RunAllClusters(context.Background())
		}
	}()
	klog.Info("节点巡检定时调度器已启动")
}

// RunAllClusters 对所有已注册集群执行一次巡检并落历史记录（并发逐集群）。
func RunAllClusters(ctx context.Context) {
	cfg := models.GetNodeInspectConfig()
	plan := BuildPlan(cfg)
	promURL, promToken := models.ResolvePrometheus()

	allClusters := make([]string, 0, 8)
	client.Managers().Range(func(key, value interface{}) bool {
		if name, ok := key.(string); ok {
			allClusters = append(allClusters, name)
		}
		return true
	})
	sort.Strings(allClusters)

	// 集群范围：配置了 scheduleClusters 时只巡检指定集群（忽略未注册的名字）
	scope := cfg.ParseScheduleClusters()
	var clusters []string
	if len(scope) == 0 {
		clusters = allClusters
	} else {
		set := make(map[string]bool, len(scope))
		for _, s := range scope {
			set[s] = true
		}
		for _, c := range allClusters {
			if set[c] {
				clusters = append(clusters, c)
			}
		}
	}

	var wg sync.WaitGroup
	results := make([]k8snode.NodeInspectionResult, 0, len(clusters))
	var mu sync.Mutex
	for _, c := range clusters {
		wg.Add(1)
		go func(cluster string) {
			defer wg.Done()
			if res := RunOneCluster(ctx, cluster, plan, promURL, promToken); res != nil {
				mu.Lock()
				results = append(results, *res)
				mu.Unlock()
			}
		}(c)
	}
	wg.Wait()

	// 企业微信机器人推送巡检报告
	if cfg.WebhookEnabled && cfg.WebhookURL != "" {
		hasAbnormal := false
		for _, r := range results {
			if r.Warning > 0 || r.Critical > 0 || r.Error != "" {
				hasAbnormal = true
				break
			}
		}
		if !cfg.WebhookOnlyWarning || hasAbnormal {
			md := BuildInspectReportMarkdown(results, time.Now())
			if err := notify.SendWeComMarkdown(cfg.WebhookURL, md); err != nil {
				klog.Errorf("推送企业微信巡检报告失败: %v", err)
			} else {
				klog.Info("企业微信巡检报告推送成功")
			}
		}
	}

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

// RunOneCluster 巡检单个集群并保存历史记录，返回巡检结果（失败时返回 nil）。
func RunOneCluster(ctx context.Context, cluster string, plan map[k8snode.InspectItemType]k8snode.ItemThreshold, promURL, promToken string) *k8snode.NodeInspectionResult {
	mgr, err := client.Manager(cluster)
	if err != nil {
		klog.Warningf("巡检集群 %s 获取客户端失败: %v", cluster, err)
		return nil
	}
	res, err := k8snode.InspectNodes(ctx, mgr.Client, promURL, promToken, mgr.Config, plan)
	if err != nil {
		klog.Errorf("巡检集群 %s 失败: %v", cluster, err)
		return nil
	}
	runAt := time.Now().Truncate(time.Minute)
	if err := SaveInspectHistory(cluster, runAt, res); err != nil {
		klog.Errorf("保存集群 %s 巡检历史失败: %v", cluster, err)
		return nil
	}
	klog.Infof("集群 %s 巡检完成: 节点 %d, 告警 %d, 严重 %d", cluster, res.Total, res.Warning, res.Critical)
	return res
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
