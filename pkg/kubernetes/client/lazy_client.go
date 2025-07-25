package client

import (
	"errors"
	"sync"
	"time"

	"github.com/JLPAY/gwayne/models"
	apiextensionsclientset "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"
)

// LazyClusterManager 懒加载集群管理器
type LazyClusterManager struct {
	cluster       *models.Cluster
	client        *kubernetes.Clientset
	config        *rest.Config
	cacheFactory  *CacheFactory
	kubeClient    ResourceHandler
	dynamicClient *dynamic.DynamicClient
	crdClient     *apiextensionsclientset.Clientset

	// 懒加载相关字段
	initialized bool
	mu          sync.RWMutex
	lastUsed    time.Time
	accessCount int64
}

// LazyClientManager 懒加载客户端管理器
type LazyClientManager struct {
	managers *sync.Map
	// 配置选项
	maxIdleTime    time.Duration // 最大空闲时间
	maxAccessCount int64         // 最大访问次数（用于LRU）
	cleanupTicker  *time.Ticker
	stopChan       chan struct{}
}

var (
	lazyManager *LazyClientManager
	once        sync.Once
)

// NewLazyClientManager 创建懒加载客户端管理器
func NewLazyClientManager() *LazyClientManager {
	once.Do(func() {
		lazyManager = &LazyClientManager{
			managers:       &sync.Map{},
			maxIdleTime:    30 * time.Minute, // 30分钟空闲后清理
			maxAccessCount: 1000,             // 访问1000次后清理
			stopChan:       make(chan struct{}),
		}

		// 启动清理协程
		go lazyManager.startCleanup()
	})
	return lazyManager
}

// GetLazyManager 获取懒加载管理器
func GetLazyManager() *LazyClientManager {
	if lazyManager == nil {
		return NewLazyClientManager()
	}
	return lazyManager
}

// GetCluster 获取集群（懒加载）
func (lcm *LazyClientManager) GetCluster(clusterName string) (*LazyClusterManager, error) {
	// 首先检查是否已存在
	if manager, exists := lcm.managers.Load(clusterName); exists {
		lazyManager := manager.(*LazyClusterManager)
		lazyManager.updateAccess()
		return lazyManager, nil
	}

	// 从数据库获取集群信息
	cluster, err := models.GetClusterByName(clusterName)
	if err != nil {
		return nil, err
	}

	// 创建新的懒加载管理器
	lazyManager := &LazyClusterManager{
		cluster:     cluster,
		initialized: false,
		lastUsed:    time.Now(),
		accessCount: 1,
	}

	// 存储到缓存中
	lcm.managers.Store(clusterName, lazyManager)

	return lazyManager, nil
}

// GetClient 获取K8s客户端（懒加载初始化）
func (lcm *LazyClusterManager) GetClient() (*kubernetes.Clientset, error) {
	lcm.mu.RLock()
	if lcm.initialized && lcm.client != nil {
		lcm.updateAccess()
		client := lcm.client
		lcm.mu.RUnlock()
		return client, nil
	}
	lcm.mu.RUnlock()

	// 需要初始化
	lcm.mu.Lock()
	defer lcm.mu.Unlock()

	// 双重检查
	if lcm.initialized && lcm.client != nil {
		lcm.updateAccess()
		return lcm.client, nil
	}

	// 初始化客户端
	if err := lcm.initialize(); err != nil {
		return nil, err
	}

	lcm.updateAccess()
	return lcm.client, nil
}

// GetKubeClient 获取资源处理器（懒加载初始化）
func (lcm *LazyClusterManager) GetKubeClient() (ResourceHandler, error) {
	lcm.mu.RLock()
	if lcm.initialized && lcm.kubeClient != nil {
		lcm.updateAccess()
		client := lcm.kubeClient
		lcm.mu.RUnlock()
		return client, nil
	}
	lcm.mu.RUnlock()

	// 需要初始化
	lcm.mu.Lock()
	defer lcm.mu.Unlock()

	// 双重检查
	if lcm.initialized && lcm.kubeClient != nil {
		lcm.updateAccess()
		return lcm.kubeClient, nil
	}

	// 初始化客户端
	if err := lcm.initialize(); err != nil {
		return nil, err
	}

	lcm.updateAccess()
	return lcm.kubeClient, nil
}

// initialize 初始化集群连接
func (lcm *LazyClusterManager) initialize() error {
	if lcm.initialized {
		return nil
	}

	// 检查集群状态
	if lcm.cluster.Status == models.ClusterStatusMaintaining {
		return errors.New("集群正在维护中")
	}

	// 构建客户端
	clientSet, config, err := buildClient(lcm.cluster.Master, lcm.cluster.KubeConfig)
	if err != nil {
		return err
	}

	// 构建动态客户端
	dynamicClient, err := dynamic.NewForConfig(config)
	if err != nil {
		return err
	}

	// 构建CRD客户端
	crdClient, err := apiextensionsclientset.NewForConfig(config)
	if err != nil {
		return err
	}

	// 构建缓存工厂（可选，根据需要决定是否启用）
	var cacheFactory *CacheFactory
	if shouldUseCache(lcm.cluster.Name) {
		cacheFactory, err = buildCacheController(clientSet, lcm.cluster.Name)
		if err != nil {
			klog.Warningf("Failed to build cache for cluster %s: %v", lcm.cluster.Name, err)
			// 不返回错误，继续使用无缓存模式
		}
	}

	// 构建资源处理器
	kubeClient := NewResourceHandler(clientSet, dynamicClient, cacheFactory)

	// 设置字段
	lcm.client = clientSet
	lcm.config = config
	lcm.dynamicClient = dynamicClient
	lcm.crdClient = crdClient
	lcm.cacheFactory = cacheFactory
	lcm.kubeClient = kubeClient
	lcm.initialized = true

	klog.Infof("Lazy initialized cluster %s", lcm.cluster.Name)
	return nil
}

// shouldUseCache 判断是否应该使用缓存
func shouldUseCache(clusterName string) bool {
	// 可以根据集群名称、配置或其他条件决定是否启用缓存
	// 例如：只有频繁访问的集群才启用缓存
	return true // 暂时全部启用，后续可以根据访问模式优化
}

// updateAccess 更新访问统计
func (lcm *LazyClusterManager) updateAccess() {
	lcm.lastUsed = time.Now()
	lcm.accessCount++
}

// Close 关闭连接
func (lcm *LazyClusterManager) Close() {
	lcm.mu.Lock()
	defer lcm.mu.Unlock()

	if lcm.cacheFactory != nil {
		lcm.cacheFactory.Close()
	}

	lcm.initialized = false
	lcm.client = nil
	lcm.config = nil
	lcm.dynamicClient = nil
	lcm.crdClient = nil
	lcm.cacheFactory = nil
	lcm.kubeClient = nil
}

// startCleanup 启动清理协程
func (lcm *LazyClientManager) startCleanup() {
	lcm.cleanupTicker = time.NewTicker(5 * time.Minute) // 每5分钟检查一次
	defer lcm.cleanupTicker.Stop()

	for {
		select {
		case <-lcm.cleanupTicker.C:
			lcm.cleanup()
		case <-lcm.stopChan:
			return
		}
	}
}

// cleanup 清理长时间未使用的连接
func (lcm *LazyClientManager) cleanup() {
	now := time.Now()
	var toDelete []string

	lcm.managers.Range(func(key, value interface{}) bool {
		clusterName := key.(string)
		manager := value.(*LazyClusterManager)

		manager.mu.RLock()
		idleTime := now.Sub(manager.lastUsed)
		accessCount := manager.accessCount
		manager.mu.RUnlock()

		// 如果空闲时间超过阈值或访问次数过多，标记为删除
		if idleTime > lcm.maxIdleTime || accessCount > lcm.maxAccessCount {
			toDelete = append(toDelete, clusterName)
		}

		return true
	})

	// 删除标记的连接
	for _, clusterName := range toDelete {
		if manager, exists := lcm.managers.Load(clusterName); exists {
			lazyManager := manager.(*LazyClusterManager)
			lazyManager.Close()
			lcm.managers.Delete(clusterName)
			klog.Infof("Cleaned up idle cluster connection: %s", clusterName)
		}
	}
}

// Stop 停止管理器
func (lcm *LazyClientManager) Stop() {
	close(lcm.stopChan)
	if lcm.cleanupTicker != nil {
		lcm.cleanupTicker.Stop()
	}
}
 