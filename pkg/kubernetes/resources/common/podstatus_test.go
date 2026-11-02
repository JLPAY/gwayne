package common

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TestEffectivePodStatus 锁定「有效状态」的优先级，保证服务端排序口径与列表展示一致：
// Terminating > 非 Running Phase > status.reason > Ready condition reason > Running/Unknown
func TestEffectivePodStatus(t *testing.T) {
	now := metav1.Now()
	mk := func(mut func(*corev1.Pod)) *corev1.Pod {
		p := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "p"}}
		p.Status.Phase = corev1.PodRunning
		p.Status.Conditions = []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue}}
		if mut != nil {
			mut(p)
		}
		return p
	}

	cases := []struct {
		name string
		pod  *corev1.Pod
		want string
	}{
		{"nil", nil, ""},
		{"running-ready", mk(nil), "Running"},
		{"terminating", mk(func(p *corev1.Pod) { p.DeletionTimestamp = &now }), "Terminating"},
		{"pending", mk(func(p *corev1.Pod) { p.Status.Phase = corev1.PodPending }), "Pending"},
		{"succeeded", mk(func(p *corev1.Pod) { p.Status.Phase = corev1.PodSucceeded }), "Succeeded"},
		{"failed", mk(func(p *corev1.Pod) { p.Status.Phase = corev1.PodFailed }), "Failed"},
		{"crashloop", mk(func(p *corev1.Pod) { p.Status.Reason = "CrashLoopBackOff" }), "CrashLoopBackOff"},
		{"containers-not-ready", mk(func(p *corev1.Pod) {
			p.Status.Conditions[0] = corev1.PodCondition{Type: corev1.PodReady, Status: corev1.ConditionFalse, Reason: "ContainersNotReady"}
		}), "ContainersNotReady"},
		{"not-ready-no-reason", mk(func(p *corev1.Pod) { p.Status.Conditions = nil }), "Unknown"},
		{"reason-wins-over-ready-reason", mk(func(p *corev1.Pod) {
			p.Status.Reason = "Evicted"
			p.Status.Conditions[0].Reason = "ContainersNotReady"
		}), "Evicted"},
		{"terminating-wins-over-reason", mk(func(p *corev1.Pod) {
			p.DeletionTimestamp = &now
			p.Status.Reason = "CrashLoopBackOff"
		}), "Terminating"},
	}

	for _, c := range cases {
		if got := EffectivePodStatus(c.pod); got != c.want {
			t.Errorf("%s: EffectivePodStatus() = %q, want %q", c.name, got, c.want)
		}
	}
}
