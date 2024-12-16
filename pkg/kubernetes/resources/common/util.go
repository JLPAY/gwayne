package common

import corev1 "k8s.io/api/core/v1"

func ContainersRequestResourceList(containers []corev1.Container) *ResourceList {
	var cpuUsage, memoryUsage int64
	for _, container := range containers {
		// unit m
		cpuUsage += container.Resources.Requests.Cpu().MilliValue()
		// unit Byte
		memoryUsage += container.Resources.Requests.Memory().Value()
	}
	return &ResourceList{
		Cpu:    cpuUsage,
		Memory: memoryUsage,
	}
}
