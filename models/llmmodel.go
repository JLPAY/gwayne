package models

import (
	"strings"
	"time"
)

const (
	TableNameLLMModel = "llm_model"
)

// LLMModel LLM 模型接入配置模型。
// 平台内所有 AI 能力（如后续的 Pod AI 排查等）统一通过本模块获取可用的模型，
// 支持接入任意 OpenAI 兼容接口的服务（deepseek、openai、moonshot、zhipu、ollama 等）。
type LLMModel struct {
	ID       int64  `gorm:"primary_key;auto_increment" json:"id,omitempty"`
	Name     string `gorm:"unique;index;size:128" json:"name,omitempty"` // 名称
	Provider string `gorm:"size:64" json:"provider,omitempty"`           // 提供商 (deepseek, openai, ollama, azure 等)
	APIKey   string `gorm:"type:text" json:"apiKey,omitempty"`           // API 密钥
	// APIKeySet 标记是否已配置 API 密钥（列表/详情接口返回，用于前端判断是否展示掩码占位），不持久化
	APIKeySet   bool       `gorm:"-" json:"apiKeySet,omitempty"`
	BaseURL     string     `gorm:"size:512" json:"baseURL,omitempty"`          // 基础 URL（可选，留空使用提供商默认地址）
	Model       string     `gorm:"size:128" json:"model,omitempty"`            // 模型名称
	Temperature float64    `gorm:"default:0.7" json:"temperature,omitempty"`   // 温度参数
	Enabled     bool       `gorm:"default:true" json:"enabled,omitempty"`      // 是否启用
	IsDefault   bool       `gorm:"default:false" json:"isDefault,omitempty"`   // 是否为默认模型
	Description string     `gorm:"size:512" json:"description,omitempty"`      // 描述
	Config      string     `gorm:"type:text" json:"config,omitempty"`          // 额外配置（JSON 格式，预留）
	CreateTime  *time.Time `gorm:"autoCreateTime" json:"createTime,omitempty"` // 创建时间
	UpdateTime  *time.Time `gorm:"autoUpdateTime" json:"updateTime,omitempty"` // 更新时间
	User        string     `gorm:"size:128" json:"user,omitempty"`             // 创建用户
	Deleted     bool       `gorm:"default:false" json:"deleted,omitempty"`     // 软删除标记
}

// TableName 返回表名
func (LLMModel) TableName() string {
	return TableNameLLMModel
}

// MaskAPIKey 返回掩码后的密钥，仅保留前后少量字符，避免明文泄露
func (a *LLMModel) MaskAPIKey() string {
	return maskSecret(a.APIKey)
}

// maskSecret 将敏感密钥掩码处理：保留前 4 位与后 4 位，中间以 **** 代替
func maskSecret(s string) string {
	r := []rune(s)
	if len(r) == 0 {
		return ""
	}
	if len(r) <= 8 {
		return "****"
	}
	return string(r[:4]) + "****" + string(r[len(r)-4:])
}

// IsMaskedSecret 判断给定的密钥字符串是否为掩码占位（即用户未重新填写的新密钥）
func IsMaskedSecret(s string) bool {
	return strings.Contains(s, "****")
}

// GetAllLLMModels 获取所有未删除的 LLM 模型
func GetAllLLMModels() ([]LLMModel, error) {
	var ms []LLMModel
	err := DB.Where("deleted = ?", false).Order("is_default DESC, create_time DESC").Find(&ms).Error
	return ms, err
}

// GetAllEnabledLLMModels 获取所有启用中的 LLM 模型
func GetAllEnabledLLMModels() ([]LLMModel, error) {
	var ms []LLMModel
	err := DB.Where("deleted = ? AND enabled = ?", false, true).Order("is_default DESC, create_time DESC").Find(&ms).Error
	return ms, err
}

// GetLLMModelByName 根据名称获取 LLM 模型
func GetLLMModelByName(name string) (*LLMModel, error) {
	var m LLMModel
	err := DB.Where("name = ? AND deleted = ?", name, false).First(&m).Error
	if err != nil {
		return nil, err
	}
	return &m, nil
}

// GetLLMModelByID 根据 ID 获取 LLM 模型
func GetLLMModelByID(id int64) (*LLMModel, error) {
	var m LLMModel
	err := DB.Where("id = ? AND deleted = ?", id, false).First(&m).Error
	if err != nil {
		return nil, err
	}
	return &m, nil
}

// GetDefaultLLMModel 获取默认的 LLM 模型（必须启用）
func GetDefaultLLMModel() (*LLMModel, error) {
	var m LLMModel
	err := DB.Where("is_default = ? AND enabled = ? AND deleted = ?", true, true, false).First(&m).Error
	if err != nil {
		return nil, err
	}
	return &m, nil
}

// AddLLMModel 添加 LLM 模型
func AddLLMModel(m *LLMModel) (int64, error) {
	// 如果设置为默认，先取消其他默认模型
	if m.IsDefault {
		DB.Model(&LLMModel{}).Where("is_default = ?", true).Update("is_default", false)
	}
	err := DB.Create(m).Error
	if err != nil {
		return 0, err
	}
	return m.ID, nil
}

// UpdateLLMModel 更新 LLM 模型
func UpdateLLMModel(m *LLMModel) error {
	// 如果设置为默认，先取消其他默认模型
	if m.IsDefault {
		DB.Model(&LLMModel{}).Where("is_default = ? AND id != ?", true, m.ID).Update("is_default", false)
	}
	return DB.Save(m).Error
}

// DeleteLLMModel 删除 LLM 模型（软删除）
func DeleteLLMModel(id int64) error {
	var m LLMModel
	err := DB.Where("id = ?", id).First(&m).Error
	if err != nil {
		return err
	}
	m.Deleted = true
	return DB.Save(&m).Error
}

// SetDefaultLLMModel 设置默认 LLM 模型
func SetDefaultLLMModel(id int64) error {
	// 先取消所有默认
	DB.Model(&LLMModel{}).Where("is_default = ?", true).Update("is_default", false)
	// 设置新的默认
	return DB.Model(&LLMModel{}).Where("id = ?", id).Update("is_default", true).Error
}
