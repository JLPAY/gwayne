package node

import (
	"context"
	"fmt"
	"github.com/JLPAY/gwayne/pkg/kubernetes/client"
	"github.com/JLPAY/gwayne/pkg/kubernetes/resources/common"
	"github.com/JLPAY/gwayne/pkg/kubernetes/resources/pod"
	corev1 "k8s.io/api/core/v1"
	metaV1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes"
	"sort"
	"strconv"
)

type NodeStatistics struct {
	Total   int            `json:"total,omitempty"`
	Details map[string]int `json:"details,omitempty"`
}

type NodeListResult struct {
	NodeSummary   NodeListSummary `json:"nodeSummary"`
	CpuSummary    ResourceSummary `json:"cpuSummary"`
	MemorySummary ResourceSummary `json:"memorySummary"`
	Nodes         []Node          `json:"nodes"`
}

type NodeListSummary struct {
	// total nodes count
	Total int64
	// ready nodes count
	Ready int64
	// Schedulable nodes count
	Schedulable int64
}

type ResourceSummary struct {
	Total int64
	Used  int64
}

type Node struct {
	Name              string            `json:"name,omitempty"`
	Labels            map[string]string `json:"labels,omitempty"`
	CreationTimestamp metaV1.Time       `json:"creationTimestamp"`

	Spec NodeSpec `json:"spec,omitempty"`

	Status NodeStatus `json:"status,omitempty"`
}

type NodeSpec struct {
	Unschedulable bool `json:"unschedulable"`
	// If specified, the node's taints.
	// +optional
	Taints []corev1.Taint         `json:"taints,omitempty"`
	Ready  corev1.ConditionStatus `json:"ready"`
}

type NodeStatus struct {
	Capacity map[corev1.ResourceName]string `json:"capacity,omitempty"`
	NodeInfo corev1.NodeSystemInfo          `json:"nodeInfo,omitempty"`
}

func GetNodeCounts(indexer *client.CacheFactory) (int, error) {
	nodeList, err := indexer.NodeLister().List(labels.Everything())
	if err != nil {
		return 0, err
	}
	return len(nodeList), nil
}

func ListNode(indexer *client.CacheFactory) (*NodeListResult, error) {
	nodeList, err := indexer.NodeLister().List(labels.Everything())
	if err != nil {
		return nil, err
	}

	nodes := make([]Node, 0)
	ready := 0
	schedulable := 0

	// unit m  1 core = 1000m
	var avaliableCpu int64 = 0
	// unit Byte
	var avaliableMemory int64 = 0

	avaliableNodeMap := make(map[string]*corev1.Node)

	for _, node := range nodeList {
		isReady := false
		isSchedulable := false
		for _, condition := range node.Status.Conditions {
			if condition.Type == corev1.NodeReady && condition.Status == corev1.ConditionTrue {
				ready += 1
				isReady = true
			}

		}
		if !node.Spec.Unschedulable {
			schedulable += 1
			isSchedulable = true
		}

		if isReady && isSchedulable {
			avaliableNodeMap[node.Name] = node

			cpuQuantity := node.Status.Allocatable[corev1.ResourceCPU]
			memoryQuantity := node.Status.Allocatable[corev1.ResourceMemory]
			// unit m
			avaliableCpu += cpuQuantity.MilliValue()
			// unit Byte
			avaliableMemory += memoryQuantity.Value()
		}

		nodes = append(nodes, toNode(node))
	}

	sort.Slice(nodes, func(i, j int) bool {
		return nodes[i].Name < nodes[j].Name
	})

	resourceList, err := podUsedResourcesOnAvaliableNode(indexer, avaliableNodeMap)
	if err != nil {
		return nil, err
	}

	return &NodeListResult{
		NodeSummary: NodeListSummary{
			Total:       int64(len(nodes)),
			Ready:       int64(ready),
			Schedulable: int64(schedulable),
		},
		CpuSummary: ResourceSummary{
			Total: avaliableCpu / 1000,
			Used:  resourceList.Cpu / 1000,
		},
		MemorySummary: ResourceSummary{
			Total: avaliableMemory / (1024 * 1024 * 1024),
			Used:  resourceList.Memory / (1024 * 1024 * 1024),
		},
		Nodes: nodes,
	}, nil
}

func podUsedResourcesOnAvaliableNode(indexer *client.CacheFactory, avaliableNodeMap map[string]*corev1.Node) (*common.ResourceList, error) {
	result := &common.ResourceList{}
	cachePods, err := pod.ListKubePod(indexer, "", nil)
	if err != nil {
		return nil, err
	}

	for _, pod := range cachePods {
		// Exclude Pod on Unavailable Node
		_, ok := avaliableNodeMap[pod.Spec.NodeName]
		if pod.Status.Phase == corev1.PodFailed || pod.Status.Phase == corev1.PodSucceeded || pod.DeletionTimestamp != nil || !ok {
			continue
		}

		resourceList := common.ContainersRequestResourceList(pod.Spec.Containers)

		result.Cpu += resourceList.Cpu
		result.Memory += resourceList.Memory
	}

	return result, nil
}

func toNode(knode *corev1.Node) Node {

	node := Node{
		Name:              knode.Name,
		Labels:            knode.Labels,
		CreationTimestamp: knode.CreationTimestamp,
		Spec: NodeSpec{
			Unschedulable: knode.Spec.Unschedulable,
			Taints:        knode.Spec.Taints,
		},
		Status: NodeStatus{
			NodeInfo: knode.Status.NodeInfo,
		},
	}

	capacity := make(map[corev1.ResourceName]string)

	for resourceName, value := range knode.Status.Capacity {
		if resourceName == corev1.ResourceCPU {
			// cpu unit core
			capacity[resourceName] = strconv.Itoa(int(value.Value()))
		}
		if resourceName == corev1.ResourceMemory {
			// memory unit Gi
			capacity[resourceName] = strconv.Itoa(int(value.Value() / (1024 * 1024 * 1024)))
		}
	}
	node.Status.Capacity = capacity

	for _, condition := range knode.Status.Conditions {
		if condition.Type == corev1.NodeReady {
			node.Spec.Ready = condition.Status
		}
	}

	return node
}

func UpdateNode(cli *kubernetes.Clientset, node *corev1.Node) (*corev1.Node, error) {
	newNode, err := cli.CoreV1().Nodes().Update(context.TODO(), node, metaV1.UpdateOptions{})
	if err != nil {
		return nil, err
	}
	return newNode, nil
}

func DeleteNode(cli *kubernetes.Clientset, name string) error {
	return cli.CoreV1().Nodes().Delete(context.TODO(), name, metaV1.DeleteOptions{})
}

func GetNodeByName(cli *kubernetes.Clientset, name string) (*corev1.Node, error) {
	return cli.CoreV1().Nodes().Get(context.TODO(), name, metaV1.GetOptions{})
}

// 使用 Informer 机制来实现，通过从本地缓存获取node数据
func GetNodeByNameFromInformer(indexer *client.CacheFactory, name string) (*corev1.Node, error) {
	node, err := indexer.NodeLister().Get(name)
	if err != nil {
		return nil, fmt.Errorf("failed to get node %s from informer cache: %v", name, err)
	}

	return node, nil
}
