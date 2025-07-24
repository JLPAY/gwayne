package proxy

import (
	"testing"

	"github.com/JLPAY/gwayne/pkg/kubernetes/resources/dataselector"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestEventCell_GetProperty(t *testing.T) {
	// 创建测试用的Event
	now := metav1.Now()
	event := corev1.Event{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-event",
			Namespace: "default",
			Labels: map[string]string{
				"app": "test",
			},
			Annotations: map[string]string{
				"description": "test event",
			},
		},
		InvolvedObject: corev1.ObjectReference{
			Kind:      "Pod",
			Name:      "test-pod",
			Namespace: "default",
			UID:       "test-uid",
		},
		Reason:         "TestReason",
		Message:        "Test message",
		Type:           "Normal",
		Count:          5,
		FirstTimestamp: now,
		LastTimestamp:  now,
		Source: corev1.EventSource{
			Component: "test-component",
			Host:      "test-host",
		},
	}

	cell := EventCell(event)

	tests := []struct {
		name     string
		property dataselector.PropertyName
		expected interface{}
	}{
		{
			name:     "Name property",
			property: dataselector.NameProperty,
			expected: "test-event",
		},
		{
			name:     "Namespace property",
			property: dataselector.NamespaceProperty,
			expected: "default",
		},
		{
			name:     "Reason property",
			property: dataselector.ReasonProperty,
			expected: "TestReason",
		},
		{
			name:     "Message property",
			property: dataselector.MessageProperty,
			expected: "Test message",
		},
		{
			name:     "Type property",
			property: dataselector.TypeProperty,
			expected: "Normal",
		},
		{
			name:     "Count property",
			property: dataselector.CountProperty,
			expected: 5,
		},
		{
			name:     "SourceComponent property",
			property: dataselector.SourceComponentProperty,
			expected: "test-component",
		},
		{
			name:     "SourceHost property",
			property: dataselector.SourceHostProperty,
			expected: "test-host",
		},
		{
			name:     "InvolvedObjectKind property",
			property: dataselector.InvolvedObjectKindProperty,
			expected: "Pod",
		},
		{
			name:     "InvolvedObjectName property",
			property: dataselector.InvolvedObjectNameProperty,
			expected: "test-pod",
		},
		{
			name:     "InvolvedObjectNamespace property",
			property: dataselector.InvolvedObjectNamespaceProperty,
			expected: "default",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := cell.GetProperty(tt.property)
			if result == nil {
				t.Errorf("expected %v, got nil", tt.expected)
				return
			}

			// 根据期望的类型进行类型断言
			switch expected := tt.expected.(type) {
			case string:
				if str, ok := result.(dataselector.StdComparableString); ok {
					if string(str) != expected {
						t.Errorf("expected %v, got %v", expected, string(str))
					}
				} else {
					t.Errorf("expected StdComparableString, got %T", result)
				}
			case int:
				if num, ok := result.(dataselector.StdComparableInt); ok {
					if int(num) != expected {
						t.Errorf("expected %v, got %v", expected, int(num))
					}
				} else {
					t.Errorf("expected StdComparableInt, got %T", result)
				}
			default:
				t.Errorf("unsupported expected type: %T", expected)
			}
		})
	}
}

func TestEventCell_GetLabels(t *testing.T) {
	event := corev1.Event{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{
				"app":     "test",
				"version": "v1",
			},
		},
	}

	cell := EventCell(event)
	labels := cell.GetLabels()

	if len(labels) != 2 {
		t.Errorf("expected 2 labels, got %d", len(labels))
	}

	if labels["app"] != "test" {
		t.Errorf("expected app=test, got app=%s", labels["app"])
	}

	if labels["version"] != "v1" {
		t.Errorf("expected version=v1, got version=%s", labels["version"])
	}
}

func TestEventCell_GetAnnotations(t *testing.T) {
	event := corev1.Event{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				"description": "test event",
				"priority":    "high",
			},
		},
	}

	cell := EventCell(event)
	annotations := cell.GetAnnotations()

	if len(annotations) != 2 {
		t.Errorf("expected 2 annotations, got %d", len(annotations))
	}

	if annotations["description"] != "test event" {
		t.Errorf("expected description=test event, got description=%s", annotations["description"])
	}

	if annotations["priority"] != "high" {
		t.Errorf("expected priority=high, got priority=%s", annotations["priority"])
	}
}

func TestEventCell_GetReason(t *testing.T) {
	event := corev1.Event{
		Reason: "TestReason",
	}

	cell := EventCell(event)
	reason := cell.GetReason()

	if reason != "TestReason" {
		t.Errorf("expected TestReason, got %s", reason)
	}
}

func TestEventCell_GetMessage(t *testing.T) {
	event := corev1.Event{
		Message: "Test message",
	}

	cell := EventCell(event)
	message := cell.GetMessage()

	if message != "Test message" {
		t.Errorf("expected Test message, got %s", message)
	}
}

func TestEventCell_GetType(t *testing.T) {
	event := corev1.Event{
		Type: "Warning",
	}

	cell := EventCell(event)
	eventType := cell.GetType()

	if eventType != "Warning" {
		t.Errorf("expected Warning, got %s", eventType)
	}
}

func TestEventCell_GetCount(t *testing.T) {
	event := corev1.Event{
		Count: 10,
	}

	cell := EventCell(event)
	count := cell.GetCount()

	if count != 10 {
		t.Errorf("expected 10, got %d", count)
	}
}

func TestEventCell_GetSource(t *testing.T) {
	event := corev1.Event{
		Source: corev1.EventSource{
			Component: "test-component",
			Host:      "test-host",
		},
	}

	cell := EventCell(event)
	source := cell.GetSource()

	if source.Component != "test-component" {
		t.Errorf("expected test-component, got %s", source.Component)
	}

	if source.Host != "test-host" {
		t.Errorf("expected test-host, got %s", source.Host)
	}
}

func TestEventCell_GetInvolvedObject(t *testing.T) {
	event := corev1.Event{
		InvolvedObject: corev1.ObjectReference{
			Kind:      "Pod",
			Name:      "test-pod",
			Namespace: "default",
			UID:       "test-uid",
		},
	}

	cell := EventCell(event)
	involvedObject := cell.GetInvolvedObject()

	if involvedObject.Kind != "Pod" {
		t.Errorf("expected Pod, got %s", involvedObject.Kind)
	}

	if involvedObject.Name != "test-pod" {
		t.Errorf("expected test-pod, got %s", involvedObject.Name)
	}

	if involvedObject.Namespace != "default" {
		t.Errorf("expected default, got %s", involvedObject.Namespace)
	}

	if string(involvedObject.UID) != "test-uid" {
		t.Errorf("expected test-uid, got %s", involvedObject.UID)
	}
}

func TestEventCell_GetFirstTimestamp(t *testing.T) {
	now := metav1.Now()
	event := corev1.Event{
		FirstTimestamp: now,
	}

	cell := EventCell(event)
	firstTimestamp := cell.GetFirstTimestamp()

	if firstTimestamp.IsZero() {
		t.Error("expected non-zero FirstTimestamp")
		return
	}

	if !firstTimestamp.Time.Equal(now.Time) {
		t.Errorf("expected %v, got %v", now.Time, firstTimestamp.Time)
	}
}

func TestEventCell_GetLastTimestamp(t *testing.T) {
	now := metav1.Now()
	event := corev1.Event{
		LastTimestamp: now,
	}

	cell := EventCell(event)
	lastTimestamp := cell.GetLastTimestamp()

	if lastTimestamp.IsZero() {
		t.Error("expected non-zero LastTimestamp")
		return
	}

	if !lastTimestamp.Time.Equal(now.Time) {
		t.Errorf("expected %v, got %v", now.Time, lastTimestamp.Time)
	}
}
