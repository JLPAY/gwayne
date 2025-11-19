package k8sgpt

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/k8sgpt-ai/k8sgpt/pkg/ai"
	"github.com/spf13/viper"
	"k8s.io/klog/v2"
)

// AIConfigManager 管理 AI 引擎配置
type AIConfigManager struct {
	configPath string
	mu         sync.RWMutex
}

var (
	globalAIManager *AIConfigManager
	once            sync.Once
)

// GetAIConfigManager 获取全局 AI 配置管理器实例
func GetAIConfigManager() *AIConfigManager {
	once.Do(func() {
		// 使用 XDG 配置目录
		configDir := os.Getenv("XDG_CONFIG_HOME")
		if configDir == "" {
			homeDir, _ := os.UserHomeDir()
			configDir = filepath.Join(homeDir, ".config")
		}
		configPath := filepath.Join(configDir, "gwayne", "k8sgpt.yaml")

		globalAIManager = &AIConfigManager{
			configPath: configPath,
		}

		// 确保配置目录存在
		if err := os.MkdirAll(filepath.Dir(configPath), 0755); err != nil {
			klog.Errorf("Failed to create config directory: %v", err)
		}

		// 初始化 viper
		viper.SetConfigFile(configPath)
		viper.SetConfigType("yaml")
		if err := viper.ReadInConfig(); err != nil {
			// 如果配置文件不存在，创建一个默认的
			if os.IsNotExist(err) {
				if err := viper.WriteConfigAs(configPath); err != nil {
					klog.Errorf("Failed to create default config file: %v", err)
				}
			} else {
				klog.Errorf("Failed to read config file: %v", err)
			}
		}
	})
	return globalAIManager
}

// AIProviderConfig AI 提供者配置
type AIProviderConfig struct {
	Name           string  `json:"name" yaml:"name"`
	Model          string  `json:"model" yaml:"model"`
	Password       string  `json:"password,omitempty" yaml:"password,omitempty"`
	BaseURL        string  `json:"baseurl,omitempty" yaml:"baseurl,omitempty"`
	ProxyEndpoint  string  `json:"proxyEndpoint,omitempty" yaml:"proxyEndpoint,omitempty"`
	ProxyPort      string  `json:"proxyPort,omitempty" yaml:"proxyPort,omitempty"`
	EndpointName   string  `json:"endpointname,omitempty" yaml:"endpointname,omitempty"`
	Engine         string  `json:"engine,omitempty" yaml:"engine,omitempty"`
	Temperature    float32 `json:"temperature,omitempty" yaml:"temperature,omitempty"`
	ProviderRegion string  `json:"providerregion,omitempty" yaml:"providerregion,omitempty"`
	ProviderId     string  `json:"providerid,omitempty" yaml:"providerid,omitempty"`
	CompartmentId  string  `json:"compartmentid,omitempty" yaml:"compartmentid,omitempty"`
	TopP           float32 `json:"topp,omitempty" yaml:"topp,omitempty"`
	TopK           int32   `json:"topk,omitempty" yaml:"topk,omitempty"`
	MaxTokens      int     `json:"maxtokens,omitempty" yaml:"maxtokens,omitempty"`
	OrganizationId string  `json:"organizationid,omitempty" yaml:"organizationid,omitempty"`
}

// AIConfiguration AI 配置
type AIConfiguration struct {
	Providers       []AIProviderConfig `json:"providers" yaml:"providers"`
	DefaultProvider string             `json:"defaultprovider,omitempty" yaml:"defaultprovider,omitempty"`
}

// AddProvider 添加 AI 提供者
func (m *AIConfigManager) AddProvider(provider AIProviderConfig) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	var config AIConfiguration
	if err := viper.UnmarshalKey("ai", &config); err != nil {
		config = AIConfiguration{
			Providers: []AIProviderConfig{},
		}
	}

	// 检查是否已存在同名提供者
	for i, p := range config.Providers {
		if p.Name == provider.Name {
			// 更新现有提供者
			config.Providers[i] = provider
			viper.Set("ai", config)
			if err := viper.WriteConfig(); err != nil {
				return fmt.Errorf("failed to write config: %w", err)
			}
			klog.Infof("Updated AI provider: %s", provider.Name)
			return nil
		}
	}

	// 添加新提供者
	config.Providers = append(config.Providers, provider)

	// 如果这是第一个提供者，设置为默认
	if len(config.Providers) == 1 {
		config.DefaultProvider = provider.Name
	}

	viper.Set("ai", config)
	if err := viper.WriteConfig(); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	klog.Infof("Added AI provider: %s", provider.Name)
	return nil
}

// RemoveProvider 删除 AI 提供者
func (m *AIConfigManager) RemoveProvider(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	var config AIConfiguration
	if err := viper.UnmarshalKey("ai", &config); err != nil {
		return fmt.Errorf("failed to read config: %w", err)
	}

	var newProviders []AIProviderConfig
	found := false
	for _, p := range config.Providers {
		if p.Name != name {
			newProviders = append(newProviders, p)
		} else {
			found = true
		}
	}

	if !found {
		return fmt.Errorf("provider %s not found", name)
	}

	config.Providers = newProviders

	// 如果删除的是默认提供者，重新设置默认值
	if config.DefaultProvider == name {
		if len(newProviders) > 0 {
			config.DefaultProvider = newProviders[0].Name
		} else {
			config.DefaultProvider = ""
		}
	}

	viper.Set("ai", config)
	if err := viper.WriteConfig(); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	klog.Infof("Removed AI provider: %s", name)
	return nil
}

// ListProviders 列出所有 AI 提供者
func (m *AIConfigManager) ListProviders() (AIConfiguration, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var config AIConfiguration
	if err := viper.UnmarshalKey("ai", &config); err != nil {
		return AIConfiguration{
			Providers: []AIProviderConfig{},
		}, nil
	}

	return config, nil
}

// SetDefaultProvider 设置默认 AI 提供者
func (m *AIConfigManager) SetDefaultProvider(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	var config AIConfiguration
	if err := viper.UnmarshalKey("ai", &config); err != nil {
		return fmt.Errorf("failed to read config: %w", err)
	}

	// 检查提供者是否存在
	found := false
	for _, p := range config.Providers {
		if p.Name == name {
			found = true
			break
		}
	}

	if !found {
		return fmt.Errorf("provider %s not found", name)
	}

	config.DefaultProvider = name
	viper.Set("ai", config)
	if err := viper.WriteConfig(); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	klog.Infof("Set default AI provider: %s", name)
	return nil
}

// GetProvider 获取指定的 AI 提供者配置
func (m *AIConfigManager) GetProvider(name string) (*AIProviderConfig, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var config AIConfiguration
	if err := viper.UnmarshalKey("ai", &config); err != nil {
		return nil, fmt.Errorf("failed to read config: %w", err)
	}

	for _, p := range config.Providers {
		if p.Name == name {
			return &p, nil
		}
	}

	return nil, fmt.Errorf("provider %s not found", name)
}

// GetAvailableBackends 获取可用的 AI 后端列表
func (m *AIConfigManager) GetAvailableBackends() []string {
	return ai.Backends
}

// ConvertToK8sGPTProvider 将配置转换为 k8sgpt 的 AIProvider
func (p *AIProviderConfig) ConvertToK8sGPTProvider() ai.AIProvider {
	return ai.AIProvider{
		Name:           p.Name,
		Model:          p.Model,
		Password:       p.Password,
		BaseURL:        p.BaseURL,
		ProxyEndpoint:  p.ProxyEndpoint,
		ProxyPort:      p.ProxyPort,
		EndpointName:   p.EndpointName,
		Engine:         p.Engine,
		Temperature:    p.Temperature,
		ProviderRegion: p.ProviderRegion,
		ProviderId:     p.ProviderId,
		CompartmentId:  p.CompartmentId,
		TopP:           p.TopP,
		TopK:           p.TopK,
		MaxTokens:      p.MaxTokens,
		OrganizationId: p.OrganizationId,
	}
}

// ValidateProvider 验证 AI 提供者配置
func (p *AIProviderConfig) ValidateProvider() error {
	if p.Name == "" {
		return fmt.Errorf("provider name is required")
	}
	if p.Model == "" {
		return fmt.Errorf("provider model is required")
	}

	// 检查是否需要密码
	if ai.NeedPassword(p.Name) && p.Password == "" {
		return fmt.Errorf("password is required for provider %s", p.Name)
	}

	return nil
}

// ToJSON 将配置转换为 JSON
func (c *AIConfiguration) ToJSON() ([]byte, error) {
	return json.MarshalIndent(c, "", "  ")
}

// Explain 使用 AI 后端生成解释
func (m *AIConfigManager) Explain(ctx context.Context, backendName, prompt, language string) (string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// 获取提供者配置
	provider, err := m.GetProvider(backendName)
	if err != nil {
		return "", fmt.Errorf("failed to get provider: %w", err)
	}

	// 转换为 k8sgpt 的 AIProvider
	aiProvider := provider.ConvertToK8sGPTProvider()

	// 创建 AI 客户端
	aiClient := ai.NewClient(backendName)

	// 配置客户端
	if err := aiClient.Configure(&aiProvider); err != nil {
		return "", fmt.Errorf("failed to configure AI client: %w", err)
	}

	// 确保在完成后关闭客户端
	defer aiClient.Close()

	// 调用 AI 生成解释
	response, err := aiClient.GetCompletion(ctx, prompt)
	if err != nil {
		return "", fmt.Errorf("failed to get AI response: %w", err)
	}

	return response, nil
}
