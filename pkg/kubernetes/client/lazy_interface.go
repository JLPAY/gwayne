package client

import (
	"github.com/JLPAY/gwayne/models"
	"k8s.io/client-go/kubernetes"
)

// LazyClient 懒加载客户端接口
type LazyClient struct {
	manager *LazyClientManager
}

// NewLazyClient 创建新的懒加载客户端
func NewLazyClient() *LazyClient {
	return &LazyClient{
		manager: GetLazyManager(),
	}
}

// LazyClient 获取集群客户端（懒加载）
func (lc *LazyClient) Client(cluster string) (*kubernetes.Clientset, error) {
	lazyManager, err := lc.manager.GetCluster(cluster)
	if err != nil {
		return nil, err
	}
	return lazyManager.GetClient()
}

// LazyKubeClient 获取资源处理器（懒加载）
func (lc *LazyClient) KubeClient(cluster string) (ResourceHandler, error) {
	lazyManager, err := lc.manager.GetCluster(cluster)
	if err != nil {
		return nil, err
	}
	return lazyManager.GetKubeClient()
}

// LazyManager 获取集群管理器（懒加载）
func (lc *LazyClient) Manager(cluster string) (*LazyClusterManager, error) {
	return lc.manager.GetCluster(cluster)
}

// LazyCluster 获取集群信息（懒加载）
func (lc *LazyClient) Cluster(cluster string) (*models.Cluster, error) {
	lazyManager, err := lc.manager.GetCluster(cluster)
	if err != nil {
		return nil, err
	}
	return lazyManager.cluster, nil
}

// 全局懒加载客户端实例
var lazyClient *LazyClient

// GetLazyClient 获取全局懒加载客户端
func GetLazyClient() *LazyClient {
	if lazyClient == nil {
		lazyClient = NewLazyClient()
	}
	return lazyClient
}

// 向后兼容的函数，使用懒加载模式
func LazyClientFunc(cluster string) (*kubernetes.Clientset, error) {
	return GetLazyClient().Client(cluster)
}

func LazyKubeClientFunc(cluster string) (ResourceHandler, error) {
	return GetLazyClient().KubeClient(cluster)
}

func LazyManagerFunc(cluster string) (*LazyClusterManager, error) {
	return GetLazyClient().Manager(cluster)
}

func LazyClusterFunc(cluster string) (*models.Cluster, error) {
	return GetLazyClient().Cluster(cluster)
}

// 统计信息
func GetLazyStats() map[string]interface{} {
	stats := make(map[string]interface{})

	if lazyManager != nil {
		count := 0
		initializedCount := 0

		lazyManager.managers.Range(func(key, value interface{}) bool {
			count++
			lazyClusterManager := value.(*LazyClusterManager)
			if lazyClusterManager.initialized {
				initializedCount++
			}
			return true
		})

		stats["total_clusters"] = count
		stats["initialized_clusters"] = initializedCount
		stats["idle_clusters"] = count - initializedCount
	}

	return stats
}
