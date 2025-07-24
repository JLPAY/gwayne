package client

import (
	"sync"

	"github.com/JLPAY/gwayne/pkg/kubernetes/client/api"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	appsv1 "k8s.io/client-go/listers/apps/v1"
	autoscalingv1 "k8s.io/client-go/listers/autoscaling/v1"
	corev1 "k8s.io/client-go/listers/core/v1"
	"k8s.io/klog/v2"
)

var sharedInformerFactoryCache = sync.Map{} // 用于缓存工厂实例

type CacheFactory struct {
	stopChan              chan struct{}
	sharedInformerFactory informers.SharedInformerFactory
}

func (c ClusterManager) Close() {
	// 清理 informer 和 stop 通道
	close(c.CacheFactory.stopChan)
	c.CacheFactory.sharedInformerFactory.Shutdown()
}

func buildCacheController(client *kubernetes.Clientset, clusterName string) (*CacheFactory, error) {
	stop := make(chan struct{})

	// 使用单例的 SharedInformerFactory
	sharedInformerFactory := getSharedInformerFactory(client, clusterName)

	// 确保 Informer 已经启动
	ensureInformerStarted(sharedInformerFactory, stop)

	ResourceMaps, err := api.GetResourceMap(client)
	if err != nil {
		return nil, err
	}

	klog.V(2).Infof("start cache controller for cluster %s , has %d ResourceKind", clusterName, len(ResourceMaps))

	// Register all Informers without running them
	for _, gvrk := range ResourceMaps {
		klog.V(2).Infof("创建sharedInformerFactory.ForResource,cluster: %s Resource Name:%s Kind: %s value: %v", clusterName, gvrk.GroupVersionResourceKind.GroupVersionResource, gvrk.GroupVersionResourceKind.Kind, gvrk)
		genericInformer, err := sharedInformerFactory.ForResource(gvrk.GroupVersionResourceKind.GroupVersionResource)
		if err != nil {
			return nil, err
		}

		go genericInformer.Informer().Run(stop)
	}

	return &CacheFactory{
		stopChan:              stop,
		sharedInformerFactory: sharedInformerFactory,
	}, nil
}

func getSharedInformerFactory(client *kubernetes.Clientset, clusterName string) informers.SharedInformerFactory {
	// 检查是否已经存在该集群的工厂实例
	if factory, ok := sharedInformerFactoryCache.Load(clusterName); ok {
		return factory.(informers.SharedInformerFactory)
	}

	// 如果不存在，则初始化一个新的 SharedInformerFactory
	newFactory := informers.NewSharedInformerFactory(client, defaultResyncPeriod)
	sharedInformerFactoryCache.Store(clusterName, newFactory)
	return newFactory
}

func ensureInformerStarted(factory informers.SharedInformerFactory, stopCh chan struct{}) {
	// 如果已经启动，则不会重复启动
	if factory != nil {
		factory.Start(stopCh)
	}
}

func (c *CacheFactory) PodLister() corev1.PodLister {
	return c.sharedInformerFactory.Core().V1().Pods().Lister()
}

func (c *CacheFactory) EventLister() corev1.EventLister {
	return c.sharedInformerFactory.Core().V1().Events().Lister()
}

func (c *CacheFactory) DeploymentLister() appsv1.DeploymentLister {
	return c.sharedInformerFactory.Apps().V1().Deployments().Lister()
}

func (c *CacheFactory) NodeLister() corev1.NodeLister {
	return c.sharedInformerFactory.Core().V1().Nodes().Lister()
}

func (c *CacheFactory) EndpointLister() corev1.EndpointsLister {
	return c.sharedInformerFactory.Core().V1().Endpoints().Lister()
}

func (c *CacheFactory) HPALister() autoscalingv1.HorizontalPodAutoscalerLister {
	return c.sharedInformerFactory.Autoscaling().V1().HorizontalPodAutoscalers().Lister()
}

// Close 关闭缓存工厂
func (c *CacheFactory) Close() {
	// 清理 informer 和 stop 通道
	if c.stopChan != nil {
		close(c.stopChan)
	}
	if c.sharedInformerFactory != nil {
		c.sharedInformerFactory.Shutdown()
	}
}
