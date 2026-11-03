package models

import (
	"encoding/json"
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
	// ScheduleInterval 定时巡检间隔（分钟）
	ScheduleInterval int `gorm:"default:60" json:"scheduleInterval"`
	// RetentionDays 历史记录保留天数（超期自动清理）
	RetentionDays int `gorm:"default:90" json:"retentionDays"`

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

// ParseItems 解析 Items JSON；空或非法返回 nil（调用方回退到注册表默认值）。
func (c *NodeInspectConfig) ParseItems() []NodeInspectItemConfig {
	if c == nil || c.Items == "" {
		return nil
	}
	var items []NodeInspectItemConfig
	if err := json.Unmarshal([]byte(c.Items), &items); err != nil {
		return nil
	}
	return items
}

// SaveNodeInspectConfig 写入单例配置（upsert id=1）。调用方需先校验 items/阈值合法性。
func SaveNodeInspectConfig(items []NodeInspectItemConfig, scheduleEnabled bool, scheduleInterval, retentionDays int) (*NodeInspectConfig, error) {
	raw := "[]"
	if items != nil {
		b, err := json.Marshal(items)
		if err != nil {
			return nil, err
		}
		raw = string(b)
	}
	if scheduleInterval <= 0 {
		scheduleInterval = DefaultScheduleInterval
	}
	if retentionDays <= 0 {
		retentionDays = DefaultRetentionDays
	}
	cfg := &NodeInspectConfig{
		Id:               1,
		Items:            raw,
		ScheduleEnabled:  scheduleEnabled,
		ScheduleInterval: scheduleInterval,
		RetentionDays:    retentionDays,
	}
	if err := DB.Save(cfg).Error; err != nil {
		return nil, err
	}
	return cfg, nil
}
