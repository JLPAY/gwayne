package common

import corev1 "k8s.io/api/core/v1"

// EffectivePodStatus 计算 Pod 的「有效状态」文案，与服务端返回、前端展示的状态保持一致：
//   - 正在删除 -> Terminating
//   - 非 Running 阶段 -> 直接返回 Phase（Pending/Succeeded/Failed/Unknown）
//   - Running 阶段 -> 优先取 status.reason，其次取 Ready condition 的 reason，
//     两者都为空且 Ready 为 True 时返回 Running，否则 Unknown
//
// 该函数用于 Pod 列表「状态」列的服务端排序，避免直接按 Phase 排序导致
// Terminating（Phase 仍为 Running）与 CrashLoopBackOff（Phase 为 Running）被归到 Running 分组。
func EffectivePodStatus(pod *corev1.Pod) string {
	if pod == nil {
		return ""
	}
	if pod.DeletionTimestamp != nil {
		return "Terminating"
	}
	if pod.Status.Phase != corev1.PodRunning {
		return string(pod.Status.Phase)
	}

	ready := false
	notReadyReason := ""
	for i := range pod.Status.Conditions {
		cond := pod.Status.Conditions[i]
		if cond.Type == corev1.PodReady {
			ready = cond.Status == corev1.ConditionTrue
			notReadyReason = cond.Reason
		}
	}

	if pod.Status.Reason != "" {
		return pod.Status.Reason
	}
	if notReadyReason != "" {
		return notReadyReason
	}
	if ready {
		return "Running"
	}
	return "Unknown"
}
