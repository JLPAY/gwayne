package models

import (
	"encoding/json"
	"strings"
	"time"
)

const TableNameNodeInspectConfig = "node_inspect_config"

// NodeInspectConfig 节点巡检配置（单例，id 固定为 1）。
// Items 为可配置巡检项的 JSON 数组（未配置的项由代码内注册表提供默认值）。
type NodeInspectConfig struct {
	Id int64 `gorm:"primaryKey;autoIncrement" json:"id"`
	// Items JSON: [{"type":"memory","enabled":true,"warning":80,"critical":90}, ...]
	Items string `gorm:"type:text" json:"items"`
	// ScheduleEnabled 是否开启定时巡检任务
	ScheduleEnabled bool `gorm:"default:false" json:"scheduleEnabled"`
	// ScheduleInterval 定时巡检间隔（分钟；ScheduleCron 为空时的回退方式）
	ScheduleInterval int `gorm:"default:60" json:"scheduleInterval"`
	// ScheduleCron 定时巡检 crontab 表达式（5 段标准格式，如 "0 */6 * * *"）；非空时优先于间隔
	ScheduleCron string `gorm:"type:varchar(64)" json:"scheduleCron"`
	// ScheduleClusters 定时巡检的集群范围（逗号分隔集群名，空 = 全部已注册集群）
	ScheduleClusters string `gorm:"type:varchar(512)" json:"scheduleClusters"`
	// RetentionDays 历史记录保留天数（超期自动清理）
	RetentionDays int `gorm:"default:90" json:"retentionDays"`

	// WebhookEnabled 定时巡检完成后是否推送企业微信机器人
	WebhookEnabled bool `gorm:"default:false" json:"webhookEnabled"`
	// WebhookURL 企业微信群机器人 webhook 地址
	WebhookURL string `gorm:"type:varchar(512)" json:"webhookUrl"`
	// WebhookOnlyWarning 仅存在告警/严重节点（或巡检失败）时才推送
	WebhookOnlyWarning bool `gorm:"default:false" json:"webhookOnlyWarning"`

	// PrometheusURL 巡检数据源地址（页面配置，未配置时巡检项报「地址未配置」）
	PrometheusURL string `gorm:"type:varchar(512)" json:"prometheusUrl"`
	// PrometheusToken 可选 Bearer Token（前端回显，注意权限）
	PrometheusToken string `gorm:"type:varchar(512)" json:"prometheusToken"`

	CreateTime *time.Time `gorm:"autoCreateTime" json:"createTime,omitempty"`
	UpdateTime *time.Time `gorm:"autoUpdateTime" json:"updateTime,omitempty"`
}

// TableName 表名
func (*NodeInspectConfig) TableName() string {
	return TableNameNodeInspectConfig
}

const (
	// DefaultScheduleInterval 默认定时巡检间隔（分钟）
	DefaultScheduleInterval = 60
	// DefaultRetentionDays 默认历史保留天数
	DefaultRetentionDays = 90
)

// NodeInspectItemConfig 单个巡检项的用户配置
type NodeInspectItemConfig struct {
	Type     string  `json:"type"`
	Enabled  bool    `json:"enabled"`
	Warning  float64 `json:"warning"`
	Critical float64 `json:"critical"`
}

// GetNodeInspectConfig 读取单例配置；记录不存在时返回默认值（不报错）。
func GetNodeInspectConfig() *NodeInspectConfig {
	cfg := &NodeInspectConfig{}
	if err := DB.First(cfg, 1).Error; err != nil {
		return &NodeInspectConfig{
			Id:               1,
			ScheduleEnabled:  false,
			ScheduleInterval: DefaultScheduleInterval,
			RetentionDays:    DefaultRetentionDays,
		}
	}
	if cfg.ScheduleInterval <= 0 {
		cfg.ScheduleInterval = DefaultScheduleInterval
	}
	if cfg.RetentionDays <= 0 {
		cfg.RetentionDays = DefaultRetentionDays
	}
	return cfg
}

// ParseScheduleClusters 解析定时巡检集群范围（逗号分隔）；空返回 nil 表示全部集群。
func (c *NodeInspectConfig) ParseScheduleClusters() []string {
	if c == nil || strings.TrimSpace(c.ScheduleClusters) == "" {
		return nil
	}
	seen := map[string]bool{}
	var out []string
	for _, s := range strings.Split(c.ScheduleClusters, ",") {
		s = strings.TrimSpace(s)
		if s != "" && !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

// ParseItems 解析 Items JSON；空或非法返回 nil（调用方回退到注册表默认值）。
func (c *NodeInspectConfig) ParseItems() []NodeInspectItemConfig {	if c == nil || c.Items == "" {
		return nil
	}
	var items []NodeInspectItemConfig
	if err := json.Unmarshal([]byte(c.Items), &items); err != nil {
		return nil
	}
	return items
}

// SaveNodeInspectConfig 写入单例配置（upsert id=1）。调用方需先校验 items/阈值/cron 合法性。
// opts 携带除 Items 外的全部配置字段。
func SaveNodeInspectConfig(items []NodeInspectItemConfig, opts *NodeInspectConfig) (*NodeInspectConfig, error) {
	raw := "[]"
	if items != nil {
		b, err := json.Marshal(items)
		if err != nil {
			return nil, err
		}
		raw = string(b)
	}
	if opts == nil {
		opts = &NodeInspectConfig{}
	}
	if opts.ScheduleInterval <= 0 {
		opts.ScheduleInterval = DefaultScheduleInterval
	}
	if opts.RetentionDays <= 0 {
		opts.RetentionDays = DefaultRetentionDays
	}
	cfg := &NodeInspectConfig{
		Id:                 1,
		Items:              raw,
		ScheduleEnabled:    opts.ScheduleEnabled,
		ScheduleInterval:   opts.ScheduleInterval,
		ScheduleCron:       strings.TrimSpace(opts.ScheduleCron),
		ScheduleClusters:   opts.ScheduleClusters,
		RetentionDays:      opts.RetentionDays,
		WebhookEnabled:     opts.WebhookEnabled,
		WebhookURL:         opts.WebhookURL,
		WebhookOnlyWarning: opts.WebhookOnlyWarning,
		PrometheusURL:      strings.TrimSpace(opts.PrometheusURL),
		PrometheusToken:    strings.TrimSpace(opts.PrometheusToken),
	}
	if err := DB.Save(cfg).Error; err != nil {
		return nil, err
	}
	return cfg, nil
}

// ResolvePrometheus 解析巡检数据源：仅来自巡检配置页面（node_inspect_config 单例）。
// 未配置时返回空值，由 Prometheus 客户端报「地址未配置」明确提示。
func ResolvePrometheus() (url, token string) {
	cfg := GetNodeInspectConfig()
	return cfg.PrometheusURL, cfg.PrometheusToken
}
