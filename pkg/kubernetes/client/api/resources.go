package api

import (
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
)

type ResourceName = string
type KindName = string

// 资源名称常量
const (
	ResourceNameConfigMap               ResourceName = "configmaps"
	ResourceNameDaemonSet               ResourceName = "daemonsets"
	ResourceNameDeployment              ResourceName = "deployments"
	ResourceNameEvent                   ResourceName = "events"
	ResourceNameHorizontalPodAutoscaler ResourceName = "horizontalpodautoscalers"
	ResourceNameIngress                 ResourceName = "ingresses"
	ResourceNameJob                     ResourceName = "jobs"
	ResourceNameCronJob                 ResourceName = "cronjobs"
	ResourceNameNamespace               ResourceName = "namespaces"
	ResourceNameNode                    ResourceName = "nodes"
	ResourceNamePersistentVolumeClaim   ResourceName = "persistentvolumeclaims"
	ResourceNamePersistentVolume        ResourceName = "persistentvolumes"
	ResourceNamePod                     ResourceName = "pods"
	ResourceNameReplicaSet              ResourceName = "replicasets"
	ResourceNameSecret                  ResourceName = "secrets"
	ResourceNameService                 ResourceName = "services"
	ResourceNameStatefulSet             ResourceName = "statefulsets"
	ResourceNameEndpoint                ResourceName = "endpoints"
	ResourceNameStorageClass            ResourceName = "storageclasses"
	ResourceNameRole                    ResourceName = "roles"
	ResourceNameRoleBinding             ResourceName = "rolebindings"
	ResourceNameClusterRole             ResourceName = "clusterroles"
	ResourceNameClusterRoleBinding      ResourceName = "clusterrolebindings"
	ResourceNameServiceAccount          ResourceName = "serviceaccounts"
)

// 资源种类常量
const (
	KindNameConfigMap               KindName = "ConfigMap"
	KindNameDaemonSet               KindName = "DaemonSet"
	KindNameDeployment              KindName = "Deployment"
	KindNameEvent                   KindName = "Event"
	KindNameHorizontalPodAutoscaler KindName = "HorizontalPodAutoscaler"
	KindNameIngress                 KindName = "Ingress"
	KindNameJob                     KindName = "Job"
	KindNameCronJob                 KindName = "CronJob"
	KindNameNamespace               KindName = "Namespace"
	KindNameNode                    KindName = "Node"
	KindNamePersistentVolumeClaim   KindName = "PersistentVolumeClaim"
	KindNamePersistentVolume        KindName = "PersistentVolume"
	KindNamePod                     KindName = "Pod"
	KindNameReplicaSet              KindName = "ReplicaSet"
	KindNameSecret                  KindName = "Secret"
	KindNameService                 KindName = "Service"
	KindNameStatefulSet             KindName = "StatefulSet"
	KindNameEndpoint                KindName = "Endpoints"
	KindNameStorageClass            KindName = "StorageClass"
	KindNameRole                    KindName = "Role"
	KindNameRoleBinding             KindName = "RoleBinding"
	KindNameClusterRole             KindName = "ClusterRole"
	KindNameClusterRoleBinding      KindName = "ClusterRoleBinding"
	KindNameServiceAccount          KindName = "ServiceAccount"
)

// ResourceMap 包含资源的 GVRK 信息和命名空间标记
type ResourceMap struct {
	GroupVersionResourceKind GroupVersionResourceKind
	Namespaced               bool
}

// GroupVersionResourceKind 包含了资源的 GVR 和 Kind 信息
type GroupVersionResourceKind struct {
	schema.GroupVersionResource
	Kind string
}

// 资源配置结构体
type ResourceConfig struct {
	Name         string // 资源名称，如 "pods", "services"
	Kind         string // 资源类型，如 "Pod", "Service"
	CacheEnabled bool   // 是否启用缓存
}

/*
   只缓存这些资源类型
   Pods (corev1.PodLister)
   Events (corev1.EventLister)
   Deployments (appsv1.DeploymentLister)
   Nodes (corev1.NodeLister)
   Endpoints (corev1.EndpointsLister)
   HorizontalPodAutoscalers (HPA) (autoscalingv1.HorizontalPodAutoscalerLister)
*/

// 预定义资源配置
// 预定义的资源配置
var predefinedResources = map[string]ResourceConfig{
	"configmaps":               {"configmaps", "ConfigMap", true},
	"daemonsets":               {"daemonsets", "DaemonSet", true},
	"deployments":              {"deployments", "Deployment", true},
	"events":                   {"events", "Event", true},
	"horizontalpodautoscalers": {"horizontalpodautoscalers", "HorizontalPodAutoscaler", true},
	"ingresses":                {"ingresses", "Ingress", true},
	"jobs":                     {"jobs", "Job", true},
	"cronjobs":                 {"cronjobs", "CronJob", true},
	"namespaces":               {"namespaces", "Namespace", true},
	"nodes":                    {"nodes", "Node", true},
	"persistentvolumeclaims":   {"persistentvolumeclaims", "PersistentVolumeClaim", true},
	"persistentvolumes":        {"persistentvolumes", "PersistentVolume", true},
	"pods":                     {"pods", "Pod", true},
	"replicasets":              {"replicasets", "ReplicaSet", true},
	"secrets":                  {"secrets", "Secret", true},
	"services":                 {"services", "Service", true},
	"statefulsets":             {"statefulsets", "StatefulSet", true},
	"endpoints":                {"endpoints", "Endpoints", true},
	"storageclasses":           {"storageclasses", "StorageClass", true},
	"roles":                    {"roles", "Role", true},
	"rolebindings":             {"rolebindings", "RoleBinding", true},
	"clusterroles":             {"clusterroles", "ClusterRole", true},
	"clusterrolebindings":      {"clusterrolebindings", "ClusterRoleBinding", true},
	"serviceaccounts":          {"serviceaccounts", "ServiceAccount", true},
}

// 获取 Kubernetes 集群的资源映射
func GetResourceMap(client *kubernetes.Clientset) (map[string]ResourceMap, error) {
	discoveryClient := client.Discovery()

	// 获取服务器支持的所有资源
	apiResourceLists, err := discoveryClient.ServerPreferredResources()
	if err != nil {
		// 忽略部分资源组的解析错误
		// 确保即使某些资源在当前集群版本中不可用或解析失败，也不会中断整个流程
		if discovery.IsGroupDiscoveryFailedError(err) {
			klog.Infof("Warning: discovering resources 获取出错，跳过处理!: %v\n", err)
		} else {
			klog.Errorf("获取资源出错，%v", err)
			return nil, err
		}
	}

	resourceMap := make(map[string]ResourceMap)

	// 遍历所有资源组
	for _, apiResourceList := range apiResourceLists {
		// schema.ParseGroupVersion 动态解析 API 资源的 Group 和 Version, 适配不同版本的变化
		groupVersion, err := schema.ParseGroupVersion(apiResourceList.GroupVersion)
		if err != nil {
			continue
		}

		// 严格过滤 CRD 和扩展资源
		// 原生资源的 Group 通常是 "" 或属于 Kubernetes 官方的 API 组
		if groupVersion.Group != "" && !isKubernetesNativeGroup(groupVersion.Group) {
			continue
		}

		// 遍历每个 API 资源
		for _, apiResource := range apiResourceList.APIResources {
			// 检查资源是否在预定义列表中
			if config, exists := predefinedResources[apiResource.Name]; exists && config.CacheEnabled {
				// 创建 GVR 并填充到映射中
				gvr := schema.GroupVersionResource{
					Group:    groupVersion.Group,
					Version:  groupVersion.Version,
					Resource: apiResource.Name,
				}

				// 添加到 resourceMap
				resourceMap[apiResource.Name] = ResourceMap{
					GroupVersionResourceKind: GroupVersionResourceKind{
						GroupVersionResource: gvr,
						Kind:                 apiResource.Kind,
					},
					Namespaced: apiResource.Namespaced,
				}
			}

		}
	}

	return resourceMap, nil
}

// 判断是否为 Kubernetes 原生的 API 组
func isKubernetesNativeGroup(group string) bool {
	nativeGroups := map[string]bool{
		"":                          true, // 核心组
		"apps":                      true,
		"batch":                     true,
		"extensions":                true,
		"policy":                    true,
		"autoscaling":               true,
		"networking.k8s.io":         true,
		"rbac.authorization.k8s.io": true,
		"storage.k8s.io":            true,
	}

	return nativeGroups[group]
}
