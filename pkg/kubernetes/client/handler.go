package client

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/JLPAY/gwayne/pkg/kubernetes/client/api"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
)

// 定义了资源操作的标准方法，包括 Create、Update、Get、List 和 Delete
// 支持处理命名空间资源和全局资源。
type ResourceHandler interface {
	Create(kind string, namespace string, object *runtime.Unknown) (*runtime.Unknown, error)
	Update(kind string, namespace string, name string, object *runtime.Unknown) (*runtime.Unknown, error)
	Get(kind string, namespace string, name string) (runtime.Object, error)
	List(kind string, namespace string, labelSelector string) ([]runtime.Object, error)
	Delete(kind string, namespace string, name string, options *metav1.DeleteOptions) error
	GVRK(resourceName string) (api.ResourceMap, error)
}

type resourceHandler struct {
	client        *kubernetes.Clientset
	dynamicClient *dynamic.DynamicClient
	cacheFactory  *CacheFactory
	// 缓存资源映射，减少对discoveryClient.ServerPreferredResources()的调用
	resourceCache    map[string]api.ResourceMap
	cacheInitialized bool
	cacheLock        sync.RWMutex
}

func NewResourceHandler(kubeClient *kubernetes.Clientset, dynamicClient *dynamic.DynamicClient, cacheFactory *CacheFactory) ResourceHandler {
	return &resourceHandler{
		client:        kubeClient,
		dynamicClient: dynamicClient,
		cacheFactory:  cacheFactory,
	}
}

// runtime.Object 资源对象的抽象，包括Pod/Deployment/Service等各类资源
func (h *resourceHandler) Get(kind string, namespace string, name string) (runtime.Object, error) {
	if kind == "" {
		klog.Errorf("kind is empty, cannot retrieve resource")
		return nil, fmt.Errorf("name cannot be empty")
	}

	if name == "" {
		klog.Errorf("Name is empty, cannot retrieve resource")
		return nil, fmt.Errorf("name cannot be empty")
	}

	resource, err := h.getResource(kind)
	if err != nil {
		klog.Errorf("getResource(kind) err: %v", err)
		return nil, err
	}

	if resource.Namespaced && namespace == "" {
		klog.Errorf("Namespace is required for namespaced resources")
		return nil, fmt.Errorf("namespace cannot be empty for namespaced resources")
	}

	klog.Infof("getResource(kind): %v", resource)

	informer, err := h.cacheFactory.sharedInformerFactory.ForResource(resource.GroupVersionResourceKind.GroupVersionResource)
	if err != nil {
		klog.Errorf("sharedInformerFactory.ForResource error: %v", err)
		return nil, err
	}

	lister := informer.Lister()
	var obj runtime.Object
	if resource.Namespaced {
		obj, err = lister.ByNamespace(namespace).Get(name)
	} else {
		obj, err = lister.Get(name)
	}
	if err != nil {
		klog.Errorf("lister.Get(name) err: %v", err)
		return nil, err
	}

	obj.GetObjectKind().SetGroupVersionKind(schema.GroupVersionKind{
		Group:   resource.GroupVersionResourceKind.Group,
		Version: resource.GroupVersionResourceKind.Version,
		Kind:    resource.GroupVersionResourceKind.Kind,
	})
	return obj, nil
}

func (h *resourceHandler) Create(kind string, namespace string, object *runtime.Unknown) (*runtime.Unknown, error) {
	// 参数检查
	if kind == "" || object == nil {
		return nil, fmt.Errorf("invalid input: kind or object cannot be empty")
	}

	// 获取资源的定义信息，包括资源类型、版本、API组等
	resource, err := h.getResource(kind)
	if err != nil {
		return nil, fmt.Errorf("failed to get resource definition for kind %s: %v", kind, err)
	}
	klog.Infof("Resource definition: %v", resource)

	// 获取对应的 RESTClient，根据资源的 API 组和版本
	kubeClient := h.getClientByGroupVersion(resource.GroupVersionResourceKind.GroupVersionResource)
	if kubeClient == nil {
		return nil, fmt.Errorf("failed to get RESTClient for resource kind %s, group: %s, version: %s",
			kind, resource.GroupVersionResourceKind.GroupVersionResource.Group, resource.GroupVersionResourceKind.GroupVersionResource.Version)
	}

	// 创建 HTTP 请求，设置资源类型、Content-Type 和请求体
	req := kubeClient.Post().
		Resource(kind).
		SetHeader("Content-Type", "application/json").
		Body([]byte(object.Raw))

	// 如果资源是命名空间级别的，设置 namespace
	if resource.Namespaced {
		req.Namespace(namespace)
	}

	var result runtime.Unknown
	err = req.Do(context.Background()).Into(&result) // 使用 context.Background() 代替 context.TODO()，如果没有特定的上下文需求
	if err != nil {
		return nil, fmt.Errorf("failed to create resource %s in namespace %s: %v", kind, namespace, err)
	}
	return &result, nil
}

// 使用 dynamicClient 更新resource
func (h *resourceHandler) Update(kind string, namespace string, name string, object *runtime.Unknown) (*runtime.Unknown, error) {
	resource, err := h.getResource(kind)
	if err != nil {
		return nil, err
	}

	// 构建 GroupVersionResource (GVR)
	gvr := schema.GroupVersionResource{
		Group:    resource.GroupVersionResourceKind.GroupVersionResource.Group,
		Version:  resource.GroupVersionResourceKind.GroupVersionResource.Version,
		Resource: kind, // 通常是资源的小写复数形式，例如 "pods"
	}

	var resourceInterface dynamic.ResourceInterface
	if resource.Namespaced {
		resourceInterface = h.dynamicClient.Resource(gvr).Namespace(namespace)
	} else {
		resourceInterface = h.dynamicClient.Resource(gvr)
	}

	// 将 object 转化成 unstructured.Unstructured
	unstructuredObj := &unstructured.Unstructured{}
	err = unstructuredObj.UnmarshalJSON(object.Raw)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal object: %v", err)
	}

	// 获取当前资源的状态
	currentObj, err := resourceInterface.Get(context.Background(), name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get current resource %s/%s/%s: %v", namespace, kind, name, err)
	}

	// 设置 metadata.resourceVersion
	unstructuredObj.SetResourceVersion(currentObj.GetResourceVersion())

	// 使用 dynamicClient 更新资源
	updatedObj, err := resourceInterface.Update(context.Background(), unstructuredObj, metav1.UpdateOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to update resource %s/%s/%s: %v", namespace, kind, name, err)
	}

	// 将更新的对象转化为 runtime.Unknown
	updatedData, err := json.Marshal(updatedObj.Object)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal updated object: %v", err)
	}

	return &runtime.Unknown{Raw: updatedData}, nil
}

func (h *resourceHandler) Delete(kind string, namespace string, name string, options *metav1.DeleteOptions) error {
	resource, err := h.getResource(kind)
	if err != nil {
		return err
	}

	// 获取对应的 RESTClient，根据资源的 API 组和版本
	kubeClient := h.getClientByGroupVersion(resource.GroupVersionResourceKind.GroupVersionResource)
	if kubeClient == nil {
		return fmt.Errorf("failed to get RESTClient for resource kind %s, group: %s, version: %s",
			kind, resource.GroupVersionResourceKind.GroupVersionResource.Group, resource.GroupVersionResourceKind.GroupVersionResource.Version)
	}
	req := kubeClient.Delete().
		Resource(kind).
		Name(name).
		Body(options)
	if resource.Namespaced {
		req.Namespace(namespace)
	}

	return req.Do(context.TODO()).Error()
}

// kind: 资源类型的名称（如 Pod、Deployment 等）。
// namespace: 资源的命名空间，如果是非命名空间资源则可以忽略。
// labelSelector: 用于过滤资源的标签选择器。
func (h *resourceHandler) List(kind string, namespace string, labelSelector string) ([]runtime.Object, error) {
	// 获取指定 kind 的资源对象信息
	resource, err := h.getResource(kind)
	if err != nil {
		return nil, err
	}

	// 获取资源的Informer，用来访问资源的缓存数据
	informer, err := h.cacheFactory.sharedInformerFactory.ForResource(resource.GroupVersionResourceKind.GroupVersionResource)
	if err != nil {
		return nil, err
	}

	// 将标签选择器字符串解析成 labels.Selector 对象
	selectors, err := labels.Parse(labelSelector)
	if err != nil {
		klog.Errorf("Build label selector error.", err)
		return nil, err
	}

	lister := informer.Lister()
	var objs []runtime.Object
	// 如果资源是命名空间级别的，按命名空间过滤
	if resource.Namespaced {
		objs, err = lister.ByNamespace(namespace).List(selectors)
	} else {
		// 非命名空间资源，直接列出
		objs, err = lister.List(selectors)
	}
	if err != nil {
		return nil, err
	}

	// 为每个对象设置正确的 GroupVersionKind
	for _, obj := range objs {
		obj.GetObjectKind().SetGroupVersionKind(schema.GroupVersionKind{
			Group:   resource.GroupVersionResourceKind.Group,
			Version: resource.GroupVersionResourceKind.Version,
			Kind:    resource.GroupVersionResourceKind.Kind,
		})
	}

	// 返回获取到的资源对象列表
	return objs, nil
}

func (h *resourceHandler) GVRK(kind string) (api.ResourceMap, error) {
	// 获取指定 kind 的资源对象信息
	resource, err := h.getResource(kind)
	if err != nil {
		return api.ResourceMap{}, err
	}
	return resource, nil
}

// 获取资源的GVRK，带缓存支持
func (h *resourceHandler) getResource(kind string) (api.ResourceMap, error) {
	// 检查缓存是否已初始化
	h.cacheLock.RLock()
	if !h.cacheInitialized {
		h.cacheLock.RUnlock()
		// 初始化缓存
		klog.V(2).Info("更新k8s集群的resourceMap缓存")
		h.cacheLock.Lock()
		if !h.cacheInitialized { // Double check after acquiring write lock
			resourceMap, err := api.GetResourceMap(h.client)
			if err != nil {
				h.cacheLock.Unlock()
				klog.Errorf("Failed to initialize resource cache, error: %v", err)
				return api.ResourceMap{}, err
			}
			h.resourceCache = resourceMap
			h.cacheInitialized = true
		}
		h.cacheLock.Unlock()
		h.cacheLock.RLock() // Re-acquire read lock for reading
	}
	defer h.cacheLock.RUnlock()

	// 从缓存中获取资源
	resource, ok := h.resourceCache[kind]
	if !ok {
		klog.Errorf("getResource unsupported resource kind: %s", kind)
		return api.ResourceMap{}, fmt.Errorf("unsupported resource kind: %s", kind)
	}

	return resource, nil
}

func (h *resourceHandler) updateServiceResourceVersion(namespace, name string, object *runtime.Unknown) error {
	// 获取当前 Service 对象
	currentService, err := h.client.CoreV1().Services(namespace).Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get service: %v", err)
	}

	// 解码传入的对象
	var updatedService v1.Service
	err = json.Unmarshal(object.Raw, &updatedService)
	if err != nil {
		return fmt.Errorf("failed to unmarshal service: %v", err)
	}

	// 更新资源版本
	updatedService.ResourceVersion = currentService.ResourceVersion

	// 编码更新后的对象
	updatedRaw, err := json.Marshal(updatedService)
	if err != nil {
		return fmt.Errorf("failed to marshal updated service: %v", err)
	}

	// 更新原始对象
	object.Raw = updatedRaw
	return nil
}
