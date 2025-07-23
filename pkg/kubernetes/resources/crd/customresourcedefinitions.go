package crd

import (
	"context"
	"fmt"
	"github.com/JLPAY/gwayne/pkg/kubernetes/resources/dataselector"
	"github.com/JLPAY/gwayne/pkg/pagequery"
	apiextensions "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apiextensionsclientset "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
	"sort"
)

// 获取 CRD 列表
func GetCRDPage(clientset *apiextensionsclientset.Clientset, q *pagequery.QueryParam) (*pagequery.Page, error) {
	// 获取 CRD 列表
	crdList, err := clientset.ApiextensionsV1().CustomResourceDefinitions().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		klog.Errorf("Get CRD List error: %v", err)
		return nil, err
	}

	// 遍历 CRD 列表并获取最优版本
	for i := range crdList.Items {
		crd := &crdList.Items[i]
		// 获取最优版本
		bestVersion := getBestCRDVersion(crd)
		// 将最优版本设置为 APIVersion
		crd.APIVersion = bestVersion.Name
	}

	// 返回分页数据
	return dataselector.DataSelectPage(toCells(crdList.Items), q), nil
}

// 适配不同 CRD 版本
func toCells(deploy []apiextensions.CustomResourceDefinition) []dataselector.DataCell {
	cells := make([]dataselector.DataCell, len(deploy))
	for i := range deploy {
		cells[i] = CRDCell(deploy[i])
	}
	return cells
}

// 获取 CRD 的最优版本
func getBestCRDVersion(crd *apiextensions.CustomResourceDefinition) *apiextensions.CustomResourceDefinitionVersion {
	// 优先选择存储版本
	for _, version := range crd.Spec.Versions {
		if version.Storage {
			return &version
		}
	}

	// 如果没有存储版本，选择第一个提供的版本
	for _, version := range crd.Spec.Versions {
		if version.Served {
			return &version
		}
	}

	// 如果都没有，选择按名称排序的最新版本
	sort.Slice(crd.Spec.Versions, func(i, j int) bool {
		return crd.Spec.Versions[i].Name > crd.Spec.Versions[j].Name
	})

	return &crd.Spec.Versions[0]
}

func GetBestCRDVersionByGroupKind(clientset *apiextensionsclientset.Clientset, group, kind string) (*apiextensions.CustomResourceDefinitionVersion, error) {
	// 创建 CRD 客户端
	crdClient := clientset.ApiextensionsV1().CustomResourceDefinitions()

	crdName := fmt.Sprintf("%s.%s", kind, group)

	// 获取指定的 CRD
	crd, err := crdClient.Get(context.TODO(), crdName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get CRD %s: %v", crdName, err)
	}

	// 优先选择存储版本
	for _, version := range crd.Spec.Versions {
		if version.Storage {
			return &version, nil
		}
	}

	// 如果没有存储版本，选择第一个提供的版本
	for _, version := range crd.Spec.Versions {
		if version.Served {
			return &version, nil
		}
	}

	// 如果都没有，选择按名称排序的最新版本
	sort.Slice(crd.Spec.Versions, func(i, j int) bool {
		return crd.Spec.Versions[i].Name > crd.Spec.Versions[j].Name
	})

	return &crd.Spec.Versions[0], nil
}
