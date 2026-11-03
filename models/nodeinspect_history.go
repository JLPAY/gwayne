package models

import (
	"time"
)

const TableNameNodeInspectHistory = "node_inspect_history"

// NodeInspectHistory 节点巡检历史记录（每次定时巡检按 节点×巡检项 落行，RunAt 标识一次运行）
type NodeInspectHistory struct {
	Id        int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	Cluster   string    `gorm:"type:varchar(64);index:idx_nih_cluster" json:"cluster"`
	Node      string    `gorm:"type:varchar(128);index" json:"node"`
	CheckType string    `gorm:"type:varchar(32);index" json:"checkType"`
	Status    string    `gorm:"type:varchar(16)" json:"status"`
	Percent   float64   `json:"percent"`
	Value     string    `gorm:"type:varchar(128)" json:"value"`
	Detail    string    `gorm:"type:text" json:"detail,omitempty"`
	RunAt     time.Time `gorm:"index:idx_nih_cluster" json:"runAt"`

	CreateTime *time.Time `gorm:"autoCreateTime" json:"createTime,omitempty"`
}

// TableName 表名
func (*NodeInspectHistory) TableName() string {
	return TableNameNodeInspectHistory
}

// NodeInspectRunSummary 一次巡检运行的汇总（按 cluster+run_at 聚合）
type NodeInspectRunSummary struct {
	Cluster  string    `json:"cluster" gorm:"column:cluster"`
	RunAt    time.Time `json:"runAt" gorm:"column:run_at"`
	Total    int       `json:"total" gorm:"column:total"`
	Warning  int       `json:"warning" gorm:"column:warning"`
	Critical int       `json:"critical" gorm:"column:critical"`
}

// QueryNodeInspectRuns 查询历次巡检运行汇总（按 run_at 倒序）
func QueryNodeInspectRuns(cluster string, limit int) ([]NodeInspectRunSummary, error) {
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	q := DB.Model(&NodeInspectHistory{}).
		Select("cluster, run_at, COUNT(*) as total, "+
			"SUM(CASE WHEN status = 'warning' THEN 1 ELSE 0 END) as warning, "+
			"SUM(CASE WHEN status = 'critical' THEN 1 ELSE 0 END) as critical").
		Group("cluster, run_at").
		Order("run_at DESC").
		Limit(limit)
	if cluster != "" {
		q = q.Where("cluster = ?", cluster)
	}
	var rows []NodeInspectRunSummary
	if err := q.Scan(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

// QueryNodeInspectHistory 查询巡检历史明细（可按集群/节点/巡检项/时间范围过滤）
func QueryNodeInspectHistory(cluster, node, checkType string, from, to time.Time, limit int) ([]NodeInspectHistory, error) {
	if limit <= 0 || limit > 20000 {
		limit = 5000
	}
	q := DB.Model(&NodeInspectHistory{})
	if cluster != "" {
		q = q.Where("cluster = ?", cluster)
	}
	if node != "" {
		q = q.Where("`node` = ?", node)
	}
	if checkType != "" {
		q = q.Where("check_type = ?", checkType)
	}
	if !from.IsZero() {
		q = q.Where("run_at >= ?", from)
	}
	if !to.IsZero() {
		q = q.Where("run_at <= ?", to)
	}
	var rows []NodeInspectHistory
	if err := q.Order("run_at DESC").Limit(limit).Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

// DeleteNodeInspectHistoryBefore 清理保留期之前的历史记录
func DeleteNodeInspectHistoryBefore(t time.Time) (int64, error) {
	res := DB.Where("run_at < ?", t).Delete(&NodeInspectHistory{})
	return res.RowsAffected, res.Error
}
