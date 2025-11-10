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
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog/v2"
)

const (
	// Kubernetes API 客户端的速率限制配置
	defaultQPS          = 50               // 每秒最多 50 个请求
	defaultBurst        = 100              // 突发请求最多 100 个
	defaultResyncPeriod = 30 * time.Second //资源的同步周期，默认30秒
)

var (
	ErrNotExist    = errors.New("集群不存在。 ")
	ErrMaintaining = errors.New("集群正在维护，请稍后再试。 ")
)

var (
	clusterManagerSets = &sync.Map{} //并发场景下对集群管理器的访问更加安全
)

type ClusterManager struct {
	Cluster       *models.Cluster
	Client        *kubernetes.Clientset
	Config        *rest.Config
	CacheFactory  *CacheFactory
	KubeClient    ResourceHandler
	DynamicClient *dynamic.DynamicClient
	CrdClient     *apiextensionsclientset.Clientset
}

func BuildApiserverClient() {
	// 获取所有正常集群
	clusters, err := models.GetAllNormalClusters()
	if err != nil {
		klog.Errorf("failed to get all normal clusters: %v", err)
		return
	}

	// 删除那些在 clusterManagerSets 中但不再存在于数据库中，或者集群管理器已被删除（Deleted 为 true）的集群
	cleanUpDeletedClusters(clusters)

	var wg sync.WaitGroup

	// 遍历所有集群
	for _, cluster := range clusters {
		if !isClusterUnchanged(cluster) {
			// 集群没有发生变化，则跳过
			klog.V(3).Infof("k8s集群 %s 集群配置没有发生变化。", cluster.Name)
			continue
		}

		wg.Add(1)

		// 为每个集群创建一个 goroutine
		go func(cluster models.Cluster) {
			defer wg.Done()

			// 使用 LoadOrStore 避免重复构建 client
			_, exists := clusterManagerSets.LoadOrStore(cluster.Name, &ClusterManager{
				Cluster: &cluster,
			})

			// 如果集群已经存在，不需要重新构建
			if exists {
				klog.V(2).Infof("k8s集群 %s 已经存在，不需要重新构建", cluster.Name)
				return
			}

			// 构建 Client 和其他资源
			clientSet, config, err := buildClient(cluster.Master, cluster.KubeConfig)
			if err != nil {
				klog.Errorf("failed to build client for cluster %s: %v", cluster.Name, err)
				// 初始化失败，删除部分初始化的 ClusterManager
				clusterManagerSets.Delete(cluster.Name)
				return
			}

			dynamicClient, err := dynamic.NewForConfig(config)
			if err != nil {
				klog.Errorf("failed to create dynamic client for cluster %s: %v", cluster.Name, err)
				// 初始化失败，删除部分初始化的 ClusterManager
				clusterManagerSets.Delete(cluster.Name)
				return
			}

			crdClient, err := apiextensionsclientset.NewForConfig(config)
			if err != nil {
				klog.Errorf("failed to create crdClient for cluster %s: %v", cluster.Name, err)
				// 初始化失败，删除部分初始化的 ClusterManager
				clusterManagerSets.Delete(cluster.Name)
				return
			}

			cacheFactory, err := buildCacheController(clientSet, cluster.Name)
			if err != nil {
				klog.Errorf("failed to build cache controller for cluster %s: %v", cluster.Name, err)
				// 初始化失败，删除部分初始化的 ClusterManager
				clusterManagerSets.Delete(cluster.Name)
				return
			}

			// 更新 clusterManagerSets 中的管理器
			clusterManagerSets.Store(cluster.Name, &ClusterManager{
				Cluster:       &cluster,
				Client:        clientSet,
				Config:        config,
				CacheFactory:  cacheFactory,
				KubeClient:    NewResourceHandler(clientSet, dynamicClient, cacheFactory),
				DynamicClient: dynamicClient,
				CrdClient:     crdClient,
			})

		}(cluster)
	}

	// 等待所有 goroutine 完成
	wg.Wait()

	klog.V(3).Info("Finished resyncing clusters.")
}

// 检查单个集群是否变化
func isClusterUnchanged(cluster models.Cluster) bool {
	managerInterface, exists := clusterManagerSets.Load(cluster.Name)
	if !exists {
		// 如果找不到管理器，说明集群需要新增
		return true
	}

	manager := managerInterface.(*ClusterManager)
	// 如果 Cluster.Master 和 Cluster.Status 相同，并且 kubeConfig 都没有改变，则认为集群配置没有变化
	return !(manager.Cluster.Master == cluster.Master &&
		manager.Cluster.Status == cluster.Status &&
		manager.Cluster.KubeConfig == cluster.KubeConfig)
}

func buildClient(master string, kubeconfig string) (*kubernetes.Clientset, *rest.Config, error) {
	// 创建客户端配置
	config, err := clientcmd.NewClientConfigFromBytes([]byte(kubeconfig))
	if err != nil {
		klog.Errorf("New Client Config error. %v ", err)
		return nil, nil, err
	}

	clientConfig, err := config.ClientConfig()
	if err != nil {
		klog.Errorf("Error loading client config: %v ", err)
		return nil, nil, err
	}

	// 设置 QPS 和 Burst
	clientConfig.QPS = defaultQPS
	clientConfig.Burst = defaultBurst

	// 创建 Kubernetes clientset
	clientSet, err := kubernetes.NewForConfig(clientConfig)
	if err != nil {
		klog.Errorf("(%s) kubernetes.NewForConfig(%v) error.%v", master, err, config)
		return nil, nil, err
	}

	return clientSet, clientConfig, nil
}

func cleanUpDeletedClusters(clusters []models.Cluster) {
	// 使用一个 map 来记录当前数据库中存在的集群名称
	currentClusterNames := make(map[string]struct{})
	for _, cluster := range clusters {
		// 只考虑那些没有被标记为 deleted 的集群
		if !cluster.Deleted {
			currentClusterNames[cluster.Name] = struct{}{}
		}
	}

	// 清理 clusterManagerSets 中那些集群名称不在当前数据库中的集群
	clusterManagerSets.Range(func(key, value interface{}) bool {
		clusterManager := value.(*ClusterManager)
		clusterName := clusterManager.Cluster.Name

		// 如果 clusterName 不在当前集群列表中，删除该 clusterManager
		if _, found := currentClusterNames[clusterName]; !found {
			managerInterface, _ := clusterManagerSets.Load(key)
			manager := managerInterface.(*ClusterManager)
			manager.Close()
			clusterManagerSets.Delete(key)
			klog.Infof("Cluster %s (Name: %d) has been removed from clusterManagerSets because it no longer exists in the database.\n", clusterName, clusterManager.Cluster.ID)
		}
		return true
	})
}

func Cluster(cluster string) (*models.Cluster, error) {
	manager, err := Manager(cluster)
	if err != nil {
		return nil, err
	}
	return manager.Cluster, nil
}

func Client(cluster string) (*kubernetes.Clientset, error) {
	manager, err := Manager(cluster)
	if err != nil {
		return nil, err
	}
	return manager.Client, nil
}

func Managers() *sync.Map {
	return clusterManagerSets
}

func Manager(cluster string) (*ClusterManager, error) {
	managerInterface, exist := clusterManagerSets.Load(cluster)
	// 如果不存在，则重新获取一次集群信息
	if !exist {
		BuildApiserverClient()
		managerInterface, exist = clusterManagerSets.Load(cluster)
		if !exist {
			return nil, ErrNotExist
		}
	}
	manager := managerInterface.(*ClusterManager)
	if manager.Cluster.Status == models.ClusterStatusMaintaining {
		return nil, ErrMaintaining
	}
	return manager, nil
}

func KubeClient(cluster string) (ResourceHandler, error) {
	manager, err := Manager(cluster)
	if err != nil {
		return nil, err
	}

	return manager.KubeClient, nil
}
