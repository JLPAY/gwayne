package crd

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/JLPAY/gwayne/pkg/kubernetes/resources/dataselector"
	"github.com/JLPAY/gwayne/pkg/pagequery"
	apiextensions "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apiextensionsclientset "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/klog/v2"
)

func GetCustomCRDPage(clientset *apiextensionsclientset.Clientset, dynamicClient dynamic.Interface, group, kind, namespace string, q *pagequery.QueryParam) (*pagequery.Page, error) {
	// 创建 CRD 客户端
	crdClient := clientset.ApiextensionsV1().CustomResourceDefinitions()

	crdName := fmt.Sprintf("%s.%s", kind, group)

	// 获取指定的 CRD
	crd, err := crdClient.Get(context.TODO(), crdName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get CRD %s: %v", crdName, err)
	}

	// 通过 CRD 的 `Spec.Names.Plural` 获取 CRD 定义的资源名称
	resource := crd.Spec.Names.Plural

	// 获取最优版本，而不是直接使用第一个版本
	bestVersion := getBestCRDVersion(crd)
	if bestVersion == nil {
		return nil, fmt.Errorf("no valid version found for CRD %s", crdName)
	}
	version := bestVersion.Name

	// 验证版本是否可用
	if !bestVersion.Served {
		return nil, fmt.Errorf("version %s of CRD %s is not served", version, crdName)
	}

	resourceGVR := schema.GroupVersionResource{
		Group:    group,
		Version:  version,
		Resource: resource,
	}

	// 构建动态客户端
	resourceClient := dynamicClient.Resource(resourceGVR)

	klog.V(2).Infof("CRD Group Version:%s , %s", crd.Spec.Group, version)
	klog.V(2).Info("CRD Resource Name:", resource)

	var crdInstances *unstructured.UnstructuredList

	// 如果传入的 namespace 为空，查询所有命名空间的 CRD 实例
	if namespace == "" {
		// 查询所有命名空间中的 CRD 实例
		crdInstances, err = resourceClient.List(context.TODO(), metav1.ListOptions{})

	} else {
		// 查询特定命名空间中的 CRD 实例
		crdInstances, err = resourceClient.Namespace(namespace).List(context.TODO(), metav1.ListOptions{})
	}
	if err != nil {
		return nil, fmt.Errorf("failed to list instances of CRD %s in namespace %s: %v", crdName, namespace, err)
	}

	return dataselector.DataSelectPage(toCustomCRDCells(crdInstances.Items), q), nil
}

func toCustomCRDCells(items []unstructured.Unstructured) []dataselector.DataCell {
	cells := make([]dataselector.DataCell, len(items))
	for i, item := range items {
		customCRD := CustomCRD{
			TypeMeta: metav1.TypeMeta{
				APIVersion: item.GetAPIVersion(),
				Kind:       item.GetKind(),
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:              item.GetName(),
				Namespace:         item.GetNamespace(),
				Labels:            item.GetLabels(),
				CreationTimestamp: item.GetCreationTimestamp(),
			},
			Spec:   item.Object["spec"],
			Status: item.Object["status"],
		}

		// 将 CustomCRD 转换为 CustomCRDCell 类型
		cells[i] = CustomCRDCell(customCRD)
	}
	return cells
}

// 根据 group, version, kind, namespace, name 获取 CRD 实例
func GetCustomCRDInstanceByName(dynamicClient dynamic.Interface, group, version, kind, namespace, name string) (runtime.Object, error) {
	// 构建 GroupVersionResource
	resourceGVR := schema.GroupVersionResource{
		Group:    group,
		Version:  version,
		Resource: kind,
	}

	// 获取资源客户端
	resourceClient := dynamicClient.Resource(resourceGVR)

	// 获取 CRD 实例
	instance, err := resourceClient.Namespace(namespace).Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get instance %s/%s of CRD %s: %v", namespace, name, kind, err)
	}

	// 返回 CRD 实例数据
	return instance, nil
}

func CreateCustomCRD(clientset *apiextensionsclientset.Clientset, body interface{}) (runtime.Object, error) {
	// 将 body 转换为 CustomResourceDefinition 对象
	crd := &apiextensions.CustomResourceDefinition{}
	if err := json.Unmarshal(body.([]byte), crd); err != nil {
		klog.Errorf("Failed to unmarshal body into CustomResourceDefinition: %v", err)
		return nil, err
	}

	// 使用 ApiextensionsV1 客户端创建 CRD
	crdClient := clientset.ApiextensionsV1().CustomResourceDefinitions()
	createdCRD, err := crdClient.Create(context.TODO(), crd, metav1.CreateOptions{})
	if err != nil {
		klog.Errorf("Failed to create CustomResourceDefinition: %v", err)
		return nil, err
	}

	return createdCRD, nil
}

// 根据 group, version, kind, namespace, name 更新 CRD 实例
func UpdateCustomCRD(dynamicClient dynamic.Interface, group, version, kind, namespace, name string, object *runtime.Unknown) (runtime.Object, error) {
	// 构建 GroupVersionResource
	resourceGVR := schema.GroupVersionResource{
		Group:    group,
		Version:  version,
		Resource: kind,
	}

	// 获取资源客户端
	resourceClient := dynamicClient.Resource(resourceGVR)

	// 将 runtime.Unknown 转换为 unstructured.Unstructured
	unstructuredObj := &unstructured.Unstructured{}
	if err := json.Unmarshal(object.Raw, unstructuredObj); err != nil {
		return nil, fmt.Errorf("failed to unmarshal custom resource object: %v", err)
	}

	// 调用 dynamic client 的 Update 方法来更新 CRD 实例
	updatedInstance, err := resourceClient.Namespace(namespace).Update(context.TODO(), unstructuredObj, metav1.UpdateOptions{})
	if err != nil {
		klog.Errorf("Failed to update CRD instance %s/%s of CRD %s: %v", namespace, name, kind, err)
		return nil, err
	}

	return updatedInstance, nil
}

func DeleteCustomCRD(dynamicClient dynamic.Interface, group, version, kind, namespace, name string) error {
	// 构建 GroupVersionResource
	resourceGVR := schema.GroupVersionResource{
		Group:    group,
		Version:  version,
		Resource: kind,
	}

	// 获取资源客户端
	resourceClient := dynamicClient.Resource(resourceGVR)

	err := resourceClient.Namespace(namespace).Delete(context.TODO(), name, metav1.DeleteOptions{})
	if err != nil {
		klog.Errorf("Failed to delete CRD instance group: %s,version: %s,kind: %s,namespace: %s,name: %s: %v", group, version, kind, namespace, name, err)
		return err
	}
	return nil
}
