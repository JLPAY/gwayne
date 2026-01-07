package models

import (
	"time"
)

const (
	TableNameTerminalCommandRule = "terminal_command_rule"
)

// RuleType 规则类型
type RuleType int

const (
	RuleTypeBlacklist RuleType = iota // 黑名单：禁止执行
	RuleTypeWhitelist                 // 白名单：仅允许执行
)

// TerminalCommandRule 终端命令规则
type TerminalCommandRule struct {
	Id          int64     `gorm:"primaryKey;autoIncrement" json:"id,omitempty"`
	Role        string    `gorm:"size:100;index" json:"role,omitempty"`        // 角色：admin, user 等
	Cluster     string    `gorm:"size:100;index" json:"cluster,omitempty"`     // 集群名称，空字符串表示所有集群
	RuleType    RuleType `gorm:"type:int;default:0" json:"ruleType"`         // 规则类型：0-黑名单，1-白名单
	Command     string    `gorm:"size:500;index" json:"command,omitempty"`    // 命令模式（支持正则）
	Description string    `gorm:"type:text" json:"description,omitempty"`     // 描述
	Enabled     bool      `gorm:"default:true" json:"enabled"`               // 是否启用
	CreateTime  *time.Time `gorm:"autoCreateTime" json:"createTime,omitempty"`
	UpdateTime  *time.Time `gorm:"autoUpdateTime" json:"updateTime,omitempty"`
}

// TableName 表名
func (*TerminalCommandRule) TableName() string {
	return TableNameTerminalCommandRule
}

// GetRuleTypeName 获取规则类型名称
func (r *TerminalCommandRule) GetRuleTypeName() string {
	if r.RuleType == RuleTypeBlacklist {
		return "黑名单"
	}
	return "白名单"
}

// AddTerminalCommandRule 新增命令规则
func AddTerminalCommandRule(rule *TerminalCommandRule) (id int64, err error) {
	if err := DB.Create(rule).Error; err != nil {
		return 0, err
	}
	return rule.Id, nil
}

// GetTerminalCommandRuleById 根据ID获取规则
func GetTerminalCommandRuleById(id int64) (*TerminalCommandRule, error) {
	var rule TerminalCommandRule
	if err := DB.First(&rule, id).Error; err != nil {
		return nil, err
	}
	return &rule, nil
}

// GetAllTerminalCommandRules 获取所有规则
func GetAllTerminalCommandRules() ([]TerminalCommandRule, error) {
	var rules []TerminalCommandRule
	err := DB.Order("role, cluster, rule_type, id").Find(&rules).Error
	return rules, err
}

// GetTerminalCommandRulesByRole 根据角色获取规则
func GetTerminalCommandRulesByRole(role string) ([]TerminalCommandRule, error) {
	var rules []TerminalCommandRule
	err := DB.Where("role = ? AND enabled = ?", role, true).Order("rule_type, id").Find(&rules).Error
	return rules, err
}

// GetTerminalCommandRulesByRoleAndCluster 根据角色和集群获取规则
func GetTerminalCommandRulesByRoleAndCluster(role string, cluster string) ([]TerminalCommandRule, error) {
	var rules []TerminalCommandRule
	// 查询条件：角色匹配 AND 启用 AND (集群为空字符串 OR 集群匹配)
	err := DB.Where("role = ? AND enabled = ? AND (cluster = '' OR cluster = ?)", role, true, cluster).Order("rule_type, id").Find(&rules).Error
	return rules, err
}

// UpdateTerminalCommandRule 更新规则
func UpdateTerminalCommandRule(rule *TerminalCommandRule) error {
	return DB.Save(rule).Error
}

// DeleteTerminalCommandRule 删除规则
func DeleteTerminalCommandRule(id int64) error {
	return DB.Delete(&TerminalCommandRule{}, id).Error
}

// GetEnabledRulesByRole 获取指定角色启用的规则（兼容旧接口，不按集群过滤）
func GetEnabledRulesByRole(role string) ([]TerminalCommandRule, error) {
	var rules []TerminalCommandRule
	err := DB.Where("role = ? AND enabled = ?", role, true).Find(&rules).Error
	return rules, err
}

// GetEnabledRulesByRoleAndCluster 获取指定角色和集群启用的规则
// cluster 为空字符串时，只返回 cluster 为空字符串的规则（全局规则）
// cluster 不为空时，返回 cluster 为空字符串（全局规则）或 cluster 匹配的规则
func GetEnabledRulesByRoleAndCluster(role string, cluster string) ([]TerminalCommandRule, error) {
	var rules []TerminalCommandRule
	// 查询条件：角色匹配 AND 启用 AND (集群为空字符串 OR 集群匹配)
	err := DB.Where("role = ? AND enabled = ? AND (cluster = '' OR cluster = ?)", role, true, cluster).Find(&rules).Error
	return rules, err
}

