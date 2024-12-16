package pod

import (
	"fmt"
	"github.com/JLPAY/gwayne/pkg/kubernetes/client"
	"github.com/JLPAY/gwayne/pkg/kubernetes/client/api"
	resourcescommon "github.com/JLPAY/gwayne/pkg/kubernetes/resources/common"
	"github.com/JLPAY/gwayne/pkg/kubernetes/resources/dataselector"
	"github.com/JLPAY/gwayne/pkg/pagequery"
	"github.com/JLPAY/gwayne/pkg/slice"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"sort"
)

func ListKubePod(indexer *client.CacheFactory, namespace string, label map[string]string) ([]*corev1.Pod, error) {
	pods, err := indexer.PodLister().Pods(namespace).List(labels.SelectorFromSet(label))
	if err != nil {
		return nil, err
	}
	return pods, nil
}

func GetPodListPageByType(kubeClient client.ResourceHandler, namespace, resourceName string, resourceType api.ResourceName, q *pagequery.QueryParam) (*pagequery.Page, error) {
	relatePod, err := GetPodListByType(kubeClient, namespace, resourceName, resourceType)
	if err != nil {
		return nil, err
	}
	return pageResult(relatePod, q), nil
}

func GetPodListByType(kubeClient client.ResourceHandler, namespace, resourceName string, resourceType api.ResourceName) ([]*corev1.Pod, error) {
	switch resourceType {
	case api.ResourceNameDeployment:
		return getRelatedPodByTypeAndIntermediateType(kubeClient, namespace, resourceName, resourceType, api.ResourceNameReplicaSet)
	case api.ResourceNameCronJob:
		return getRelatedPodByTypeAndIntermediateType(kubeClient, namespace, resourceName, resourceType, api.ResourceNameJob)
	case api.ResourceNameDaemonSet, api.ResourceNameStatefulSet, api.ResourceNameJob:
		objs, err := kubeClient.List(api.ResourceNamePod, namespace, labels.Everything().String())
		if err != nil {
			return nil, err
		}

		relatePod := make([]*corev1.Pod, 0)
		for _, obj := range objs {
			pod, ok := obj.(*corev1.Pod)
			if !ok {
				return nil, fmt.Errorf("Convert pod obj (%v) error. ", obj)
			}
			for _, ref := range pod.OwnerReferences {
				//groupVersionResourceKind, ok := api.KindToResourceMap[resourceType]
				resourceMap, err := kubeClient.GVRK(resourceType)
				if err != nil {
					continue
				}
				if ref.Kind == resourceMap.GroupVersionResourceKind.Kind && resourceName == ref.Name {
					relatePod = append(relatePod, pod)
				}
			}

		}
		return relatePod, nil
	case api.ResourceNamePod:
		obj, err := kubeClient.Get(api.ResourceNamePod, namespace, resourceName)
		if err != nil {
			return nil, err
		}
		relatePod := []*corev1.Pod{
			obj.(*corev1.Pod),
		}
		return relatePod, nil
	default:
		return nil, fmt.Errorf("Unsupported resourceType %s! ", resourceType)
	}
}

func getRelatedPodByTypeAndIntermediateType(kubeClient client.ResourceHandler, namespace, resourceName string,
	resourceType api.ResourceName, intermediateResourceType api.ResourceName) ([]*corev1.Pod, error) {

	resourceMap, err := kubeClient.GVRK(resourceType)
	if err != nil {
		return nil, err
	}

	objs, err := kubeClient.List(intermediateResourceType, namespace, labels.Everything().String())
	if err != nil {
		return nil, err
	}
	relateObj := make([]string, 0)
	for _, obj := range objs {
		commonObj, err := resourcescommon.ToBaseObject(obj)
		if err != nil {
			return nil, err
		}

		for _, ref := range commonObj.OwnerReferences {
			if ref.Kind == resourceMap.GroupVersionResourceKind.Kind && ref.Name == resourceName {
				relateObj = append(relateObj, commonObj.Name)
			}
		}

	}

	relatePod := make([]*corev1.Pod, 0)
	pods, err := kubeClient.List(api.ResourceNamePod, namespace, labels.Everything().String())
	if err != nil {
		return nil, err
	}
	for _, obj := range pods {
		pod, ok := obj.(*corev1.Pod)
		if !ok {
			return nil, fmt.Errorf("Convert pod obj (%v) error. ", obj)
		}
		for _, ref := range pod.OwnerReferences {
			if ref.Kind == resourceMap.GroupVersionResourceKind.Kind &&
				slice.StrSliceContains(relateObj, ref.Name) {
				relatePod = append(relatePod, pod)
			}
		}

	}

	return relatePod, nil
}

func pageResult(relatePod []*corev1.Pod, q *pagequery.QueryParam) *pagequery.Page {
	commonObjs := make([]dataselector.DataCell, 0)
	for _, pod := range relatePod {
		commonObjs = append(commonObjs, ObjectCell(*pod))
	}

	sort.Slice(commonObjs, func(i, j int) bool {
		return commonObjs[j].GetProperty(dataselector.NameProperty).
			Compare(commonObjs[i].GetProperty(dataselector.NameProperty)) == -1
	})

	return dataselector.DataSelectPage(commonObjs, q)
}
