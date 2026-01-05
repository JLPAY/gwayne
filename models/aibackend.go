package models

import "time"

const (
	TableNameAIBackend = "ai_backend"
)

// AIBackend AI后端配置模型
type AIBackend struct {
	ID          int64      `gorm:"primary_key;auto_increment" json:"id,omitempty"`
	Name        string     `gorm:"unique;index;size:128" json:"name,omitempty"` // 后端名称
	Provider    string     `gorm:"size:64" json:"provider,omitempty"`           // 提供商 (openai, ollama, azureopenai等)
	APIKey      string     `gorm:"type:text" json:"apiKey,omitempty"`           // API密钥
	BaseURL     string     `gorm:"size:512" json:"baseURL,omitempty"`           // 基础URL（可选）
	Model       string     `gorm:"size:128" json:"model,omitempty"`             // 模型名称
	Temperature float64    `gorm:"default:0.7" json:"temperature,omitempty"`    // 温度参数
	Enabled     bool       `gorm:"default:true" json:"enabled,omitempty"`       // 是否启用
	IsDefault   bool       `gorm:"default:false" json:"isDefault,omitempty"`    // 是否为默认后端
	Description string     `gorm:"size:512" json:"description,omitempty"`       // 描述
	Config      string     `gorm:"type:text" json:"config,omitempty"`           // 额外配置（JSON格式）
	CreateTime  *time.Time `gorm:"autoCreateTime" json:"createTime,omitempty"`  // 创建时间
	UpdateTime  *time.Time `gorm:"autoUpdateTime" json:"updateTime,omitempty"`  // 更新时间
	User        string     `gorm:"size:128" json:"user,omitempty"`              // 创建用户
	Deleted     bool       `gorm:"default:false" json:"deleted,omitempty"`      // 是否删除
}

// TableName 返回表名
func (AIBackend) TableName() string {
	return TableNameAIBackend
}

// GetAllAIBackends 获取所有未删除的AI后端
func GetAllAIBackends() ([]AIBackend, error) {
	var backends []AIBackend
	err := DB.Where("deleted = ?", false).Order("is_default DESC, create_time DESC").Find(&backends).Error
	return backends, err
}

// GetAIBackendByName 根据名称获取AI后端
func GetAIBackendByName(name string) (*AIBackend, error) {
	var backend AIBackend
	err := DB.Where("name = ? AND deleted = ?", name, false).First(&backend).Error
	if err != nil {
		return nil, err
	}
	return &backend, nil
}

// GetAIBackendByID 根据ID获取AI后端
func GetAIBackendByID(id int64) (*AIBackend, error) {
	var backend AIBackend
	err := DB.Where("id = ? AND deleted = ?", id, false).First(&backend).Error
	if err != nil {
		return nil, err
	}
	return &backend, nil
}

// GetDefaultAIBackend 获取默认的AI后端
func GetDefaultAIBackend() (*AIBackend, error) {
	var backend AIBackend
	err := DB.Where("is_default = ? AND enabled = ? AND deleted = ?", true, true, false).First(&backend).Error
	if err != nil {
		return nil, err
	}
	return &backend, nil
}

// AddAIBackend 添加AI后端
func AddAIBackend(backend *AIBackend) (int64, error) {
	// 如果设置为默认，需要先取消其他默认后端
	if backend.IsDefault {
		DB.Model(&AIBackend{}).Where("is_default = ?", true).Update("is_default", false)
	}
	err := DB.Create(backend).Error
	if err != nil {
		return 0, err
	}
	return backend.ID, nil
}

// UpdateAIBackend 更新AI后端
func UpdateAIBackend(backend *AIBackend) error {
	// 如果设置为默认，需要先取消其他默认后端
	if backend.IsDefault {
		DB.Model(&AIBackend{}).Where("is_default = ? AND id != ?", true, backend.ID).Update("is_default", false)
	}
	return DB.Save(backend).Error
}

// DeleteAIBackend 删除AI后端（软删除）
func DeleteAIBackend(id int64) error {
	var backend AIBackend
	err := DB.Where("id = ?", id).First(&backend).Error
	if err != nil {
		return err
	}
	backend.Deleted = true
	return DB.Save(&backend).Error
}

// SetDefaultAIBackend 设置默认AI后端
func SetDefaultAIBackend(id int64) error {
	// 先取消所有默认
	DB.Model(&AIBackend{}).Where("is_default = ?", true).Update("is_default", false)
	// 设置新的默认
	return DB.Model(&AIBackend{}).Where("id = ?", id).Update("is_default", true).Error
}
