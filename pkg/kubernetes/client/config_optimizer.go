package client

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"sync"
	"time"

	"github.com/JLPAY/gwayne/models"
	"k8s.io/klog/v2"
)

// ConfigOptimizer 配置优化器
type ConfigOptimizer struct {
	configCache map[string]*ClusterConfig
	mu          sync.RWMutex
}

// ClusterConfig 集群配置信息
type ClusterConfig struct {
	Cluster     *models.Cluster
	ConfigHash  string
	LastUpdated time.Time
	AccessCount int64
	LastUsed    time.Time
}

// NewConfigOptimizer 创建配置优化器
func NewConfigOptimizer() *ConfigOptimizer {
	return &ConfigOptimizer{
		configCache: make(map[string]*ClusterConfig),
	}
}

// GetClusterConfig 获取集群配置（带缓存）
func (co *ConfigOptimizer) GetClusterConfig(clusterName string) (*models.Cluster, error) {
	co.mu.RLock()
	if config, exists := co.configCache[clusterName]; exists {
		config.AccessCount++
		config.LastUsed = time.Now()
		cluster := config.Cluster
		co.mu.RUnlock()
		return cluster, nil
	}
	co.mu.RUnlock()

	// 从数据库获取
	cluster, err := models.GetClusterByName(clusterName)
	if err != nil {
		return nil, err
	}

	// 缓存配置
	co.mu.Lock()
	defer co.mu.Unlock()

	configHash := co.calculateConfigHash(cluster)
	co.configCache[clusterName] = &ClusterConfig{
		Cluster:     cluster,
		ConfigHash:  configHash,
		LastUpdated: time.Now(),
		AccessCount: 1,
		LastUsed:    time.Now(),
	}

	return cluster, nil
}

// IsConfigChanged 检查配置是否发生变化
func (co *ConfigOptimizer) IsConfigChanged(clusterName string) (bool, error) {
	// 从数据库获取最新配置
	cluster, err := models.GetClusterByName(clusterName)
	if err != nil {
		return false, err
	}

	co.mu.RLock()
	defer co.mu.RUnlock()

	if config, exists := co.configCache[clusterName]; exists {
		newHash := co.calculateConfigHash(cluster)
		return config.ConfigHash != newHash, nil
	}

	// 如果缓存中不存在，认为配置发生了变化
	return true, nil
}

// UpdateConfig 更新配置缓存
func (co *ConfigOptimizer) UpdateConfig(clusterName string) error {
	cluster, err := models.GetClusterByName(clusterName)
	if err != nil {
		return err
	}

	co.mu.Lock()
	defer co.mu.Unlock()

	configHash := co.calculateConfigHash(cluster)
	co.configCache[clusterName] = &ClusterConfig{
		Cluster:     cluster,
		ConfigHash:  configHash,
		LastUpdated: time.Now(),
		AccessCount: 1,
		LastUsed:    time.Now(),
	}

	return nil
}

// RemoveConfig 移除配置缓存
func (co *ConfigOptimizer) RemoveConfig(clusterName string) {
	co.mu.Lock()
	defer co.mu.Unlock()
	delete(co.configCache, clusterName)
}

// calculateConfigHash 计算配置哈希
func (co *ConfigOptimizer) calculateConfigHash(cluster *models.Cluster) string {
	// 只对关键配置字段计算哈希
	configData := map[string]interface{}{
		"master":     cluster.Master,
		"kubeConfig": cluster.KubeConfig,
		"status":     cluster.Status,
	}

	jsonData, _ := json.Marshal(configData)
	hash := md5.Sum(jsonData)
	return hex.EncodeToString(hash[:])
}

// CleanupOldConfigs 清理旧的配置缓存
func (co *ConfigOptimizer) CleanupOldConfigs(maxAge time.Duration) {
	co.mu.Lock()
	defer co.mu.Unlock()

	now := time.Now()
	for clusterName, config := range co.configCache {
		if now.Sub(config.LastUsed) > maxAge {
			delete(co.configCache, clusterName)
			klog.V(2).Infof("Cleaned up old config cache for cluster: %s", clusterName)
		}
	}
}

// GetStats 获取配置缓存统计
func (co *ConfigOptimizer) GetStats() map[string]interface{} {
	co.mu.RLock()
	defer co.mu.RUnlock()

	stats := make(map[string]interface{})
	stats["cached_clusters"] = len(co.configCache)
	
	totalAccess := int64(0)
	for _, config := range co.configCache {
		totalAccess += config.AccessCount
	}
	stats["total_access"] = totalAccess
	
	return stats
}

// 全局配置优化器实例
var globalConfigOptimizer *ConfigOptimizer

// GetConfigOptimizer 获取全局配置优化器
func GetConfigOptimizer() *ConfigOptimizer {
	if globalConfigOptimizer == nil {
		globalConfigOptimizer = NewConfigOptimizer()
	}
	return globalConfigOptimizer
} 