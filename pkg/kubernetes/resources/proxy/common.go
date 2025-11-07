package proxy

import (
	"github.com/JLPAY/gwayne/pkg/kubernetes/resources/common"
	"github.com/JLPAY/gwayne/pkg/kubernetes/resources/dataselector"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type ObjectCell common.Object

func (cell ObjectCell) GetProperty(name dataselector.PropertyName) dataselector.ComparableValue {
	return baseProperty(name, cell.ObjectMeta)
}

func baseProperty(name dataselector.PropertyName, meta metav1.ObjectMeta) dataselector.ComparableValue {
	switch name {
	case dataselector.NameProperty:
		return dataselector.StdComparableString(meta.Name)
	case dataselector.CreationTimestampProperty:
		return dataselector.StdComparableTime(meta.CreationTimestamp.Time)
	case dataselector.NamespaceProperty:
		return dataselector.StdComparableString(meta.Namespace)
	case dataselector.ReferenceUIDProperty:
		refs := meta.OwnerReferences
		for i := range refs {
			if refs[i].Controller != nil && *refs[i].Controller {
				return dataselector.StdComparableString(refs[i].UID)
			}
		}
		return nil
	default:
		return nil
	}
}

type PodCell corev1.Pod

func (cell PodCell) GetProperty(name dataselector.PropertyName) dataselector.ComparableValue {
	switch name {
	case dataselector.PodIPProperty:
		return dataselector.StdComparableString(cell.Status.PodIP)
	case dataselector.NodeNameProperty:
		return dataselector.StdComparableString(cell.Spec.NodeName)
	case dataselector.StatusPhaseProperty:
		// Return pod status phase (Running, Pending, Succeeded, Failed, Unknown, etc.)
		return dataselector.StdComparableString(string(cell.Status.Phase))
	default:
		return baseProperty(name, cell.ObjectMeta)
	}
}
