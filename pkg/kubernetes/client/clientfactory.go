package client

import (
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"
)

// getClientByGroupVersion 返回与传入的 GroupVersionResource 匹配的 RESTClient
func (h *resourceHandler) getClientByGroupVersion(groupVersion schema.GroupVersionResource) rest.Interface {
	// 根据 groupVersion 动态选择相应的 API 组客户端
	switch groupVersion.Group {
	case "": // core API (如 Pod, Service 等)
		return h.client.CoreV1().RESTClient()

	case "apps":
		return h.client.AppsV1().RESTClient()

	case "batch":
		return h.client.BatchV1().RESTClient()

	case "extensions":
		return h.client.ExtensionsV1beta1().RESTClient()

	case "autoscaling":
		return h.client.AutoscalingV1().RESTClient()

	case "networking.k8s.io":
		return h.client.NetworkingV1().RESTClient()

	case "rbac.authorization.k8s.io":
		return h.client.RbacV1().RESTClient()

	case "storage.k8s.io":
		return h.client.StorageV1().RESTClient()

	default:
		// 如果是其他未知的组，记录日志并返回 nil
		klog.Warningf("Unknown API group: %s", groupVersion.Group)
		return nil
	}
}
