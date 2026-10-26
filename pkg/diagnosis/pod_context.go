package diagnosis

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/JLPAY/gwayne/pkg/kubernetes/client"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	k8sscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/klog/v2"
)

const (
	maxLogTailLines = 100  // 每个容器最多采集的日志行数
	maxLogBytes     = 8000 // 每段日志的最大字节数
	maxEvents       = 15   // 最多采集的事件条数
	maxEventMsgLen  = 400  // 单条事件消息最大长度

	logFetchTimeout    = 10 * time.Second // 单次日志采集超时
	collectPhaseBudget = 25 * time.Second // 整个现场采集阶段的时间预算
)

// PodContext 保存一次 Pod 异常排查所需的全部现场数据
type PodContext struct {
	Cluster   string
	Namespace string
	PodName   string
	Pod       *corev1.Pod
	Events    []corev1.Event
	Logs      []containerLog
}

type containerLog struct {
	name    string
	isPrev  bool
	content string
	err     string
}

// CollectPodContext 采集指定 Pod 的现场信息：Pod 详情、关联事件、各容器日志（含上一次崩溃）。
// 整个采集阶段限时 collectPhaseBudget，超时后未完成的采集以已得数据继续诊断，不让用户干等。
func CollectPodContext(ctx context.Context, cluster, namespace, podName string) (*PodContext, error) {
	ctx, cancel := context.WithTimeout(ctx, collectPhaseBudget)
	defer cancel()

	manager, err := client.Manager(cluster)
	if err != nil || manager == nil || manager.Client == nil {
		return nil, fmt.Errorf("获取集群 %s 客户端失败: %w", cluster, err)
	}
	cli := manager.Client

	pod, err := cli.CoreV1().Pods(namespace).Get(ctx, podName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("获取 Pod %s/%s 失败: %w", namespace, podName, err)
	}

	events, err := listPodEvents(ctx, cli, namespace, pod)
	if err != nil {
		klog.Warningf("采集 Pod %s/%s 事件失败(继续诊断): %v", namespace, podName, err)
	}

	logs := collectContainerLogs(ctx, cli, namespace, pod)

	return &PodContext{
		Cluster:   cluster,
		Namespace: namespace,
		PodName:   podName,
		Pod:       pod,
		Events:    events,
		Logs:      logs,
	}, nil
}

// listPodEvents 获取与 Pod 关联的事件（Normal + Warning），按时间倒序取最新若干条
func listPodEvents(ctx context.Context, cli *kubernetes.Clientset, namespace string, pod *corev1.Pod) ([]corev1.Event, error) {
	eventList, err := cli.CoreV1().Events(namespace).Search(k8sscheme.Scheme, pod)
	if err != nil {
		return nil, err
	}
	events := eventList.Items
	sort.Slice(events, func(i, j int) bool {
		ti, tj := eventTime(events[i]), eventTime(events[j])
		return ti.After(tj)
	})
	if len(events) > maxEvents {
		events = events[:maxEvents]
	}
	return events, nil
}

func eventTime(e corev1.Event) time.Time {
	if !e.LastTimestamp.Time.IsZero() {
		return e.LastTimestamp.Time
	}
	return e.EventTime.Time
}

// collectContainerLogs 并发采集所有容器（init + 普通）的日志；对发生过重启/终止的容器额外采集上一次日志。
// 串行逐个拉日志在多容器 Pod 上会显著拖慢排查，这里并发请求并保持结果顺序稳定。
func collectContainerLogs(ctx context.Context, cli *kubernetes.Clientset, namespace string, pod *corev1.Pod) []containerLog {
	all := append([]corev1.Container{}, pod.Spec.InitContainers...)
	all = append(all, pod.Spec.Containers...)

	type logTask struct {
		container string
		previous  bool
	}
	var tasks []logTask
	for _, c := range all {
		tasks = append(tasks, logTask{container: c.Name, previous: false})
		if containerTerminatedOrRestarted(pod, c.Name) {
			tasks = append(tasks, logTask{container: c.Name, previous: true})
		}
	}

	tail := int64(maxLogTailLines)
	results := make([]containerLog, len(tasks))
	var wg sync.WaitGroup
	for i, t := range tasks {
		wg.Add(1)
		go func(i int, t logTask) {
			defer wg.Done()
			// 单次日志采集限时，避免个别容器把整体采集拖死
			fctx, cancel := context.WithTimeout(ctx, logFetchTimeout)
			defer cancel()
			results[i] = fetchOneLog(fctx, cli, namespace, pod.Name, t.container, t.previous, tail)
		}(i, t)
	}
	wg.Wait()
	return results
}

func containerTerminatedOrRestarted(pod *corev1.Pod, name string) bool {
	for _, cs := range pod.Status.ContainerStatuses {
		if cs.Name == name && (cs.RestartCount > 0 || cs.State.Terminated != nil) {
			return true
		}
	}
	for _, cs := range pod.Status.InitContainerStatuses {
		if cs.Name == name && (cs.RestartCount > 0 || cs.State.Terminated != nil) {
			return true
		}
	}
	return false
}

func fetchOneLog(ctx context.Context, cli *kubernetes.Clientset, namespace, podName, container string, previous bool, tail int64) containerLog {
	opt := &corev1.PodLogOptions{Container: container, TailLines: &tail, Previous: previous}
	req := cli.CoreV1().Pods(namespace).GetLogs(podName, opt)
	stream, err := req.Stream(ctx)
	if err != nil {
		return containerLog{name: container, isPrev: previous, err: err.Error()}
	}
	defer stream.Close()

	buf := new(bytes.Buffer)
	if _, err := io.Copy(buf, stream); err != nil {
		return containerLog{name: container, isPrev: previous, err: err.Error()}
	}
	data := buf.Bytes()
	if len(data) > maxLogBytes {
		data = data[:maxLogBytes]
	}
	return containerLog{name: container, isPrev: previous, content: string(data)}
}

// SystemPrompt 给 AI Agent 的诊断系统提示词
func SystemPrompt() string {
	return `你是一名资深的 Kubernetes SRE 工程师。下面提供的是一个异常 Pod 的现场快照，包含 Pod 状态条件、容器状态、关联事件（Events）以及容器日志（Logs）。
请基于这些真实数据输出一份精炼的排查报告，严格按照以下结构，报告整体控制在 30 行以内，不要输出冗长废话：

# 一句话总结
<用一句话说清：哪个容器、什么异常现象、当前状态>

# 根本原因判断
- 直接原因：<从容器状态（Waiting/Terminated、ExitCode、RestartCount）、Pod 条件、Events 中得出的最直接原因，附上证据>
- 排除项：<明确排除不可能的原因（如 Init 容器已 Completed / PodScheduled=True / 镜像已拉起），一句话带过，附证据>
- 若日志证据不足以锁定根因，必须如实说明"日志证据不足，不能确定具体异常点"，并列出还需要采集什么信息，不要臆测。

# 需要重点排查的候选原因
<按可能性从高到低列出 2-5 条，每条一句话并指出对应的日志/事件依据；没有依据的不要列>

# 排查与解决步骤
<编号列表，按优先级排序：先止血（如回滚、扩资源、关探针）再根治（如修配置、修依赖）；每条具体可操作，如涉及命令/配置修改给出简短示例>

# 附加发现
<与本次 Crash 无直接关系但快照中值得注意的问题（如长期报错的事件、资源隐患），没有就省略本节>

要求：结论先行、证据导向、每个判断都要指出来自快照的依据；使用中文 Markdown 排版；无法确定的事情明确说无法确定。`
}

// BuildSnapshot 将采集到的现场信息序列化为供 LLM 分析的文本
func BuildSnapshot(pc *PodContext) string {
	var b strings.Builder
	pod := pc.Pod

	b.WriteString("# Pod 现场快照\n\n")
	fmt.Fprintf(&b, "- 集群: %s\n", pc.Cluster)
	fmt.Fprintf(&b, "- 命名空间: %s\n", pc.Namespace)
	fmt.Fprintf(&b, "- Pod 名称: %s\n", pc.PodName)
	fmt.Fprintf(&b, "- 节点: %s\n", pod.Spec.NodeName)
	fmt.Fprintf(&b, "- PodIP: %s\n", pod.Status.PodIP)
	fmt.Fprintf(&b, "- QoS: %s\n", pod.Status.QOSClass)
	fmt.Fprintf(&b, "- 重启策略: %s\n", pod.Spec.RestartPolicy)
	fmt.Fprintf(&b, "- 创建时间: %s\n", pod.CreationTimestamp.Format(time.RFC3339))
	if len(pod.OwnerReferences) > 0 {
		owners := make([]string, 0, len(pod.OwnerReferences))
		for _, o := range pod.OwnerReferences {
			owners = append(owners, fmt.Sprintf("%s/%s", o.Kind, o.Name))
		}
		fmt.Fprintf(&b, "- 属主: %s\n", strings.Join(owners, ", "))
	}
	if len(pod.Spec.Containers) > 0 {
		imgs := make([]string, 0, len(pod.Spec.Containers))
		for _, c := range pod.Spec.Containers {
			imgs = append(imgs, c.Image)
		}
		fmt.Fprintf(&b, "- 容器镜像: %s\n", strings.Join(imgs, ", "))
	}

	b.WriteString("\n## Pod 状态条件\n")
	if len(pod.Status.Conditions) == 0 {
		b.WriteString("- (无)\n")
	} else {
		for _, c := range pod.Status.Conditions {
			fmt.Fprintf(&b, "- %s: %s (Reason=%s, Message=%s)\n", c.Type, c.Status, c.Reason, c.Message)
		}
	}

	b.WriteString("\n## 容器状态\n")
	writeContainerStatuses(&b, pod.Status.InitContainerStatuses, "Init 容器")
	writeContainerStatuses(&b, pod.Status.ContainerStatuses, "容器")

	b.WriteString("\n## 关联事件\n")
	if len(pc.Events) == 0 {
		b.WriteString("- (无)\n")
	} else {
		for _, e := range pc.Events {
			msg := e.Message
			if len(msg) > maxEventMsgLen {
				msg = msg[:maxEventMsgLen] + "...(truncated)"
			}
			fmt.Fprintf(&b, "- [%s] %s (count=%d, source=%s, time=%s): %s\n",
				e.Type, e.Reason, e.Count, e.Source.Component, eventTime(e).Format(time.RFC3339), msg)
		}
	}

	b.WriteString("\n## 容器日志\n")
	hasLog := false
	for _, l := range pc.Logs {
		tag := ""
		if l.isPrev {
			tag = " (上一次崩溃)"
		}
		if l.err != "" {
			fmt.Fprintf(&b, "### %s%s 日志获取失败: %s\n", l.name, tag, l.err)
			hasLog = true
			continue
		}
		if strings.TrimSpace(l.content) == "" {
			continue
		}
		hasLog = true
		fmt.Fprintf(&b, "### %s%s 日志(尾部)\n```\n%s\n```\n", l.name, tag, l.content)
	}
	if !hasLog {
		b.WriteString("- (无)\n")
	}

	return b.String()
}

func writeContainerStatuses(b *strings.Builder, statuses []corev1.ContainerStatus, title string) {
	if len(statuses) == 0 {
		return
	}
	for _, cs := range statuses {
		fmt.Fprintf(b, "### %s %s\n", title, cs.Name)
		fmt.Fprintf(b, "- 镜像: %s\n", cs.Image)
		fmt.Fprintf(b, "- Ready: %v, RestartCount: %d\n", cs.Ready, cs.RestartCount)
		if cs.State.Waiting != nil {
			fmt.Fprintf(b, "- 状态: Waiting, Reason=%s, Message=%s\n", cs.State.Waiting.Reason, cs.State.Waiting.Message)
		}
		if cs.State.Running != nil {
			fmt.Fprintf(b, "- 状态: Running, StartedAt=%s\n", cs.State.Running.StartedAt.Format(time.RFC3339))
		}
		if cs.State.Terminated != nil {
			t := cs.State.Terminated
			fmt.Fprintf(b, "- 状态: Terminated, Reason=%s, ExitCode=%d, Signal=%d, Message=%s\n", t.Reason, t.ExitCode, t.Signal, t.Message)
		}
		if cs.LastTerminationState.Terminated != nil {
			t := cs.LastTerminationState.Terminated
			fmt.Fprintf(b, "- 上次终止: Reason=%s, ExitCode=%d, Message=%s\n", t.Reason, t.ExitCode, t.Message)
		}
	}
}
