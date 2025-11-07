package proxy

import (
	"github.com/JLPAY/gwayne/pkg/kubernetes/resources/dataselector"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EventCell 专门用于处理Event资源
type EventCell corev1.Event

// GetProperty 实现DataCell接口，提供Event特有的属性访问
func (cell EventCell) GetProperty(name dataselector.PropertyName) dataselector.ComparableValue {
	switch name {
	case dataselector.NameProperty:
		return dataselector.StdComparableString(cell.Name)
	case dataselector.CreationTimestampProperty:
		return dataselector.StdComparableTime(cell.CreationTimestamp.Time)
	case dataselector.NamespaceProperty:
		return dataselector.StdComparableString(cell.Namespace)
	case dataselector.ReferenceUIDProperty:
		return dataselector.StdComparableString(string(cell.InvolvedObject.UID))
	// Event特有属性
	case dataselector.ReasonProperty:
		return dataselector.StdComparableString(cell.Reason)
	case dataselector.TypeProperty:
		return dataselector.StdComparableString(cell.Type)
	case dataselector.MessageProperty:
		return dataselector.StdComparableString(cell.Message)
	case dataselector.CountProperty:
		return dataselector.StdComparableInt(int(cell.Count))
	case dataselector.FirstTimestampProperty:
		// 检查FirstTimestamp是否为零值
		if !cell.FirstTimestamp.IsZero() {
			return dataselector.StdComparableTime(cell.FirstTimestamp.Time)
		}
		return nil
	case dataselector.LastTimestampProperty:
		// 检查LastTimestamp是否为零值
		if !cell.LastTimestamp.IsZero() {
			return dataselector.StdComparableTime(cell.LastTimestamp.Time)
		}
		return nil
	case dataselector.SourceComponentProperty:
		return dataselector.StdComparableString(cell.Source.Component)
	case dataselector.SourceHostProperty:
		return dataselector.StdComparableString(cell.Source.Host)
	case dataselector.InvolvedObjectKindProperty:
		return dataselector.StdComparableString(cell.InvolvedObject.Kind)
	case dataselector.InvolvedObjectNameProperty:
		return dataselector.StdComparableString(cell.InvolvedObject.Name)
	case dataselector.InvolvedObjectNamespaceProperty:
		return dataselector.StdComparableString(cell.InvolvedObject.Namespace)
	default:
		return baseProperty(name, cell.ObjectMeta)
	}
}

// GetLabels 获取Event的标签
func (cell EventCell) GetLabels() map[string]string {
	return cell.Labels
}

// GetAnnotations 获取Event的注解
func (cell EventCell) GetAnnotations() map[string]string {
	return cell.Annotations
}

// GetReason 获取事件原因
func (cell EventCell) GetReason() string {
	return cell.Reason
}

// GetMessage 获取事件消息
func (cell EventCell) GetMessage() string {
	return cell.Message
}

// GetType 获取事件类型
func (cell EventCell) GetType() string {
	return cell.Type
}

// GetCount 获取事件计数
func (cell EventCell) GetCount() int32 {
	return cell.Count
}

// GetSource 获取事件来源
func (cell EventCell) GetSource() corev1.EventSource {
	return cell.Source
}

// GetInvolvedObject 获取相关对象
func (cell EventCell) GetInvolvedObject() corev1.ObjectReference {
	return cell.InvolvedObject
}

// GetFirstTimestamp 获取首次发生时间
func (cell EventCell) GetFirstTimestamp() metav1.Time {
	return cell.FirstTimestamp
}

// GetLastTimestamp 获取最后发生时间
func (cell EventCell) GetLastTimestamp() metav1.Time {
	return cell.LastTimestamp
}
