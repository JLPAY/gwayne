package pod

import (
	"github.com/JLPAY/gwayne/pkg/kubernetes/resources/common"
	"github.com/JLPAY/gwayne/pkg/kubernetes/resources/dataselector"
	corev1 "k8s.io/api/core/v1"
)

type ObjectCell corev1.Pod

// ObjectCell 实现 dataselector 接口中的 GetProperty
func (cell ObjectCell) GetProperty(name dataselector.PropertyName) dataselector.ComparableValue {
	switch name {
	case dataselector.NameProperty:
		return dataselector.StdComparableString(cell.ObjectMeta.Name)
	case dataselector.CreationTimestampProperty:
		return dataselector.StdComparableTime(cell.ObjectMeta.CreationTimestamp.Time)
	case dataselector.NamespaceProperty:
		return dataselector.StdComparableString(cell.ObjectMeta.Namespace)
	case dataselector.StatusProperty:
		// 按界面展示的「有效状态」排序，与状态列文案保持一致
		return dataselector.StdComparableString(common.EffectivePodStatus((*corev1.Pod)(&cell)))
	case "podIP":
		return dataselector.StdComparableString(cell.Status.PodIP)
	default:
		// if name is not supported then just return a constant dummy value, sort will have no effect.
		return nil
	}
}
