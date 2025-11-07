package pod

import (
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
		return dataselector.StdComparableString(cell.Status.Phase)
	case "podIP":
		return dataselector.StdComparableString(cell.Status.PodIP)
	default:
		// if name is not supported then just return a constant dummy value, sort will have no effect.
		return nil
	}
}
