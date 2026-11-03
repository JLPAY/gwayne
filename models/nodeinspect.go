package models

import "time"

const TableNameNodeInspectConfig = "node_inspect_config"

// NodeInspectConfig 节点巡检阈值配置（单例，id 固定为 1）
// warning/critical 为百分比阈值，用于判定节点内存/磁盘使用率的健康等级。
type NodeInspectConfig struct {
	Id         int64      `gorm:"primaryKey;autoIncrement" json:"id"`
	Warning    int        `gorm:"not null;default:80" json:"warning"`   // 告警阈值（%）
	Critical   int        `gorm:"not null;default:90" json:"critical"` // 严重阈值（%）
	CreateTime *time.Time `gorm:"autoCreateTime" json:"createTime,omitempty"`
	UpdateTime *time.Time `gorm:"autoUpdateTime" json:"updateTime,omitempty"`
}

// TableName 表名
func (*NodeInspectConfig) TableName() string {
	return TableNameNodeInspectConfig
}

const (
	// NodeInspectDefaultWarning 默认告警阈值（%）
	NodeInspectDefaultWarning = 80
	// NodeInspectDefaultCritical 默认严重阈值（%）
	NodeInspectDefaultCritical = 90
)

// GetNodeInspectConfig 读取单例配置；记录不存在时返回默认阈值（不报错）。
func GetNodeInspectConfig() *NodeInspectConfig {
	cfg := &NodeInspectConfig{}
	if err := DB.First(cfg, 1).Error; err != nil {
		return &NodeInspectConfig{
			Id:       1,
			Warning:  NodeInspectDefaultWarning,
			Critical: NodeInspectDefaultCritical,
		}
	}
	return cfg
}

// SaveNodeInspectConfig 写入单例配置（upsert id=1）。调用方需先校验 warning/critical 合法性。
func SaveNodeInspectConfig(warning, critical int) (*NodeInspectConfig, error) {
	cfg := &NodeInspectConfig{Id: 1, Warning: warning, Critical: critical}
	if err := DB.Save(cfg).Error; err != nil {
		return nil, err
	}
	return cfg, nil
}
