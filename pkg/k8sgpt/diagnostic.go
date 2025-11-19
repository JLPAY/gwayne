package k8sgpt

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/JLPAY/gwayne/pkg/kubernetes/client"
	"github.com/k8sgpt-ai/k8sgpt/pkg/analysis"
	k8sgptk8s "github.com/k8sgpt-ai/k8sgpt/pkg/kubernetes"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"
	k8sclient "k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime/pkg/client"
)

// DiagnosticService 诊断服务
type DiagnosticService struct {
	aiManager *AIConfigManager
}

// NewDiagnosticService 创建诊断服务实例
func NewDiagnosticService() *DiagnosticService {
	return &DiagnosticService{
		aiManager: GetAIConfigManager(),
	}
}

// DiagnosticRequest 诊断请求
type DiagnosticRequest struct {
	Cluster      string   `json:"cluster"`
	Namespace    string   `json:"namespace,omitempty"`
	ResourceType string   `json:"resourceType,omitempty"` // Pod, Node, Event 等
	ResourceName string   `json:"resourceName,omitempty"`
	Filters      []string `json:"filters,omitempty"`  // 分析器过滤器
	Explain      bool     `json:"explain"`            // 是否使用 AI 生成解释
	Backend      string   `json:"backend,omitempty"`  // AI 后端名称
	Language     string   `json:"language,omitempty"` // 语言，默认中文
}

// DiagnosticResult 诊断结果
type DiagnosticResult struct {
	Status   string         `json:"status"`             // OK, ProblemDetected
	Problems int            `json:"problems"`           // 问题数量
	Results  []ResultDetail `json:"results"`            // 诊断结果详情
	Provider string         `json:"provider,omitempty"` // 使用的 AI 提供者
	Errors   []string       `json:"errors,omitempty"`   // 错误信息
}

// ResultDetail 结果详情
type ResultDetail struct {
	Kind         string   `json:"kind"`                   // 资源类型
	Name         string   `json:"name"`                   // 资源名称
	Errors       []string `json:"errors"`                 // 错误列表
	Details      string   `json:"details"`                // AI 生成的详细说明
	ParentObject string   `json:"parentObject,omitempty"` // 父对象
}

// Diagnose 执行诊断
func (s *DiagnosticService) Diagnose(ctx context.Context, req DiagnosticRequest) (*DiagnosticResult, error) {
	// Event 特殊处理：如果 resourceType 是 Event 且提供了 resourceName，直接获取 Event 信息并发送给 AI
	if strings.ToLower(req.ResourceType) == "event" && req.ResourceName != "" && req.Namespace != "" {
		return s.diagnoseEventDirectly(ctx, req)
	}

	// 获取集群管理器
	manager, err := client.Manager(req.Cluster)
	if err != nil {
		return nil, fmt.Errorf("failed to get cluster manager: %w", err)
	}

	// 构建 k8sgpt 的 Kubernetes 客户端
	k8sGPTClient, err := buildK8sGPTClient(manager)
	if err != nil {
		return nil, fmt.Errorf("failed to build k8sgpt client: %w", err)
	}

	// 确定使用的 AI 后端
	backend := req.Backend
	if backend == "" {
		config, err := s.aiManager.ListProviders()
		if err != nil {
			return nil, fmt.Errorf("failed to get AI config: %w", err)
		}
		if config.DefaultProvider != "" {
			backend = config.DefaultProvider
		} else if len(config.Providers) > 0 {
			backend = config.Providers[0].Name
		} else {
			// 如果没有配置 AI，只进行基础分析
			backend = ""
		}
	}

	// 确定语言
	language := req.Language
	if language == "" {
		language = "中文"
	}

	// 确定过滤器
	filters := req.Filters
	if len(filters) == 0 && req.ResourceType != "" {
		// 根据资源类型设置过滤器
		switch strings.ToLower(req.ResourceType) {
		case "pod":
			filters = []string{"Pod"}
		case "node":
			filters = []string{"Node"}
		case "event":
			// Event 分析器在 K8sGPT 中不存在，不设置过滤器，让 K8sGPT 运行所有核心分析器
			// 通过命名空间限制范围，这样可以诊断该命名空间中的其他资源（如 Pod、Service 等）
			// 这些资源的问题可能与 Event 相关
			filters = []string{}
		case "deployment":
			filters = []string{"Deployment"}
		case "statefulset":
			filters = []string{"StatefulSet"}
		case "cronjob":
			filters = []string{"CronJob"}
		case "ingress":
			// 确保使用正确的 Ingress 分析器名称（首字母大写）
			filters = []string{"Ingress"}
		default:
			// 保持原始资源类型名称（首字母大写）
			resourceType := req.ResourceType
			if len(resourceType) > 0 {
				resourceType = strings.ToUpper(resourceType[:1]) + strings.ToLower(resourceType[1:])
			}
			filters = []string{resourceType}
		}
	}

	// 对于 Ingress，确保 filters 中包含 "Ingress"（首字母大写）
	// 这是 k8sgpt 的 IngressAnalyzer 在 coreAnalyzerMap 中注册的名称
	if strings.ToLower(req.ResourceType) == "ingress" {
		// 检查 filters 中是否已经包含 "Ingress"
		hasIngress := false
		for _, f := range filters {
			if f == "Ingress" {
				hasIngress = true
				break
			}
		}
		// 如果没有，确保添加 "Ingress"
		if !hasIngress {
			filters = []string{"Ingress"}
		}
		klog.V(4).Infof("Ingress diagnosis: using filters %v", filters)
	}

	// 保存原始环境变量
	originalKubeconfig := os.Getenv("KUBECONFIG")
	originalKubernetesMaster := os.Getenv("KUBERNETES_MASTER")

	// 创建临时 kubeconfig 文件，让 k8sgpt 能够使用 wayne 的集群配置
	var tempKubeconfigFile string
	if manager.Cluster != nil && manager.Cluster.KubeConfig != "" {
		// 创建临时目录
		tempDir, err := os.MkdirTemp("", "k8sgpt-kubeconfig-")
		if err != nil {
			return nil, fmt.Errorf("failed to create temp directory: %w", err)
		}

		// 创建临时 kubeconfig 文件
		tempKubeconfigFile = filepath.Join(tempDir, "kubeconfig")
		err = os.WriteFile(tempKubeconfigFile, []byte(manager.Cluster.KubeConfig), 0600)
		if err != nil {
			os.RemoveAll(tempDir)
			return nil, fmt.Errorf("failed to write temp kubeconfig file: %w", err)
		}

		// 设置 KUBECONFIG 环境变量
		os.Setenv("KUBECONFIG", tempKubeconfigFile)
		klog.V(4).Infof("Set KUBECONFIG to temp file: %s", tempKubeconfigFile)
	} else if manager.Config.Host != "" {
		// 如果没有 kubeconfig，至少设置 KUBERNETES_MASTER
		os.Setenv("KUBERNETES_MASTER", manager.Config.Host)
		klog.V(4).Infof("Set KUBERNETES_MASTER to: %s", manager.Config.Host)
	}

	defer func() {
		// 恢复原始环境变量
		if originalKubeconfig != "" {
			os.Setenv("KUBECONFIG", originalKubeconfig)
		} else {
			os.Unsetenv("KUBECONFIG")
		}
		if originalKubernetesMaster != "" {
			os.Setenv("KUBERNETES_MASTER", originalKubernetesMaster)
		} else {
			os.Unsetenv("KUBERNETES_MASTER")
		}

		// 清理临时文件
		if tempKubeconfigFile != "" {
			tempDir := filepath.Dir(tempKubeconfigFile)
			os.RemoveAll(tempDir)
			klog.V(4).Infof("Cleaned up temp kubeconfig directory: %s", tempDir)
		}
	}()

	// 创建分析实例
	explain := req.Explain && backend != ""
	analysisInstance, err := analysis.NewAnalysis(
		backend,
		language,
		filters,
		req.Namespace,
		"",    // labelSelector
		false, // noCache
		explain,
		10,         // maxConcurrency
		false,      // withDoc
		false,      // interactiveMode
		[]string{}, // customHeaders
		false,      // withStats
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create analysis: %w", err)
	}
	defer analysisInstance.Close()

	// 替换 Kubernetes 客户端为使用 wayne manager 的客户端
	analysisInstance.Client = k8sGPTClient

	// 对于 Ingress，先检查集群是否支持 NetworkingV1 API
	if strings.ToLower(req.ResourceType) == "ingress" {
		if err := checkIngressAPISupport(ctx, k8sGPTClient); err != nil {
			klog.Warningf("Ingress API check failed: %v", err)
			// 返回友好的错误信息
			return &DiagnosticResult{
				Status:   "Error",
				Problems: 0,
				Results:  []ResultDetail{},
				Provider: backend,
				Errors: []string{
					fmt.Sprintf("Ingress 资源诊断失败：%s\n"+
						"可能的原因：\n"+
						"1. 集群 Kubernetes 版本过低（需要 >= 1.19）\n"+
						"2. 集群未启用 networking.k8s.io/v1 API\n"+
						"3. Ingress 控制器未安装或未正确配置\n"+
						"请检查集群配置和 Kubernetes 版本。", err.Error()),
				},
			}, nil
		}
	}

	// 对于 CronJob，先检查集群是否支持 BatchV1 API
	if strings.ToLower(req.ResourceType) == "cronjob" {
		if err := checkCronJobAPISupport(ctx, k8sGPTClient); err != nil {
			klog.Warningf("CronJob API check failed: %v", err)
			// 返回友好的错误信息
			return &DiagnosticResult{
				Status:   "Error",
				Problems: 0,
				Results:  []ResultDetail{},
				Provider: backend,
				Errors: []string{
					fmt.Sprintf("CronJob 资源诊断失败：%s\n"+
						"可能的原因：\n"+
						"1. 集群 Kubernetes 版本过低（CronJob 需要 >= 1.21 才能使用 batch/v1 API）\n"+
						"2. 集群未启用 batch/v1 API（旧版本可能只支持 batch/v1beta1）\n"+
						"3. CronJob 控制器未安装或未正确配置\n"+
						"请检查集群配置和 Kubernetes 版本。", err.Error()),
				},
			}, nil
		}
	}

	// 运行分析
	analysisInstance.RunAnalysis()

	// 检查是否有分析错误，特别是资源类型不支持的错误
	if len(analysisInstance.Errors) > 0 {
		klog.V(4).Infof("Analysis errors: %v", analysisInstance.Errors)
		// 检查是否是资源类型不支持的错误
		for _, errMsg := range analysisInstance.Errors {
			lowerErrMsg := strings.ToLower(errMsg)
			if strings.Contains(lowerErrMsg, "could not find the requested resource") ||
				strings.Contains(lowerErrMsg, "the server could not find the requested resource") ||
				strings.Contains(lowerErrMsg, "no matches for kind") ||
				strings.Contains(lowerErrMsg, "not found") {
				klog.Warningf("Resource type may not be supported by the cluster: %s, error: %s", req.ResourceType, errMsg)
				// 对于 Ingress，可能是 API 版本问题，返回更详细的错误信息
				if strings.ToLower(req.ResourceType) == "ingress" {
					return &DiagnosticResult{
						Status:   "Error",
						Problems: 0,
						Results:  []ResultDetail{},
						Provider: backend,
						Errors:   []string{fmt.Sprintf("Ingress 资源诊断失败：集群可能不支持 networking.k8s.io/v1 API 版本，或 Ingress 控制器未安装。错误: %s", errMsg)},
					}, nil
				}
				// 对于 CronJob，可能是 API 版本问题，返回更详细的错误信息
				if strings.ToLower(req.ResourceType) == "cronjob" {
					return &DiagnosticResult{
						Status:   "Error",
						Problems: 0,
						Results:  []ResultDetail{},
						Provider: backend,
						Errors:   []string{fmt.Sprintf("CronJob 资源诊断失败：集群可能不支持 batch/v1 API 版本（需要 Kubernetes >= 1.21），或 CronJob 控制器未安装。错误: %s", errMsg)},
					}, nil
				}
				// 返回一个友好的错误信息
				return &DiagnosticResult{
					Status:   "Error",
					Problems: 0,
					Results:  []ResultDetail{},
					Provider: backend,
					Errors:   []string{fmt.Sprintf("集群可能不支持 %s 资源类型，或 API 版本不兼容。错误: %s", req.ResourceType, errMsg)},
				}, nil
			}
		}
	}

	// 如果需要 AI 解释，获取 AI 结果
	if explain {
		if err := analysisInstance.GetAIResults("json", false); err != nil {
			klog.Warningf("Failed to get AI results: %v", err)
			// 继续返回基础分析结果
		}
	}

	// 转换结果
	result := &DiagnosticResult{
		Status:   "OK",
		Problems: 0,
		Results:  []ResultDetail{},
		Provider: backend,
		Errors:   analysisInstance.Errors,
	}

	if len(analysisInstance.Results) > 0 {
		result.Status = "ProblemDetected"
		result.Problems = len(analysisInstance.Results)
	}

	for _, r := range analysisInstance.Results {
		errors := make([]string, 0, len(r.Error))
		for _, e := range r.Error {
			errors = append(errors, e.Text)
		}

		result.Results = append(result.Results, ResultDetail{
			Kind:         r.Kind,
			Name:         r.Name,
			Errors:       errors,
			Details:      r.Details,
			ParentObject: r.ParentObject,
		})
	}

	return result, nil
}

// DiagnoseNode 诊断节点
func (s *DiagnosticService) DiagnoseNode(ctx context.Context, cluster, nodeName string, explain bool) (*DiagnosticResult, error) {
	return s.Diagnose(ctx, DiagnosticRequest{
		Cluster:      cluster,
		ResourceType: "Node",
		ResourceName: nodeName,
		Filters:      []string{"Node"},
		Explain:      explain,
		Language:     "中文",
	})
}

// DiagnosePod 诊断 Pod
func (s *DiagnosticService) DiagnosePod(ctx context.Context, cluster, namespace, podName string, explain bool) (*DiagnosticResult, error) {
	return s.Diagnose(ctx, DiagnosticRequest{
		Cluster:      cluster,
		Namespace:    namespace,
		ResourceType: "Pod",
		ResourceName: podName,
		Filters:      []string{"Pod"},
		Explain:      explain,
		Language:     "中文",
	})
}

// diagnoseEventDirectly 直接诊断 Event，获取 Event 信息并发送给 AI 生成解释
func (s *DiagnosticService) diagnoseEventDirectly(ctx context.Context, req DiagnosticRequest) (*DiagnosticResult, error) {
	// 获取集群管理器
	manager, err := client.Manager(req.Cluster)
	if err != nil {
		return nil, fmt.Errorf("failed to get cluster manager: %w", err)
	}

	// 获取 Event 信息
	event, err := manager.Client.CoreV1().Events(req.Namespace).Get(ctx, req.ResourceName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get event: %w", err)
	}

	// 构建错误信息列表（从 Event 中提取）
	errors := []string{}
	if event.Reason != "" {
		errors = append(errors, fmt.Sprintf("原因: %s", event.Reason))
	}
	if event.Message != "" {
		errors = append(errors, fmt.Sprintf("消息: %s", event.Message))
	}
	if event.Type != "" && event.Type != "Normal" {
		errors = append(errors, fmt.Sprintf("类型: %s", event.Type))
	}
	if len(errors) == 0 {
		errors = append(errors, "这是一个 Kubernetes 事件，请分析其含义和可能的问题")
	}

	// 构建结果
	result := &DiagnosticResult{
		Status:   "ProblemDetected",
		Problems: 1,
		Results: []ResultDetail{
			{
				Kind:   "Event",
				Name:   fmt.Sprintf("%s/%s", req.Namespace, req.ResourceName),
				Errors: errors,
			},
		},
	}

	// 如果需要 AI 解释，生成分析说明
	if req.Explain {
		// 确定使用的 AI 后端
		backend := req.Backend
		if backend == "" {
			config, err := s.aiManager.ListProviders()
			if err == nil {
				if config.DefaultProvider != "" {
					backend = config.DefaultProvider
				} else if len(config.Providers) > 0 {
					backend = config.Providers[0].Name
				}
			}
		}

		if backend != "" {
			// 构建详细的提示词
			language := req.Language
			if language == "" {
				language = "中文"
			}

			prompt := fmt.Sprintf("请用%s详细分析以下 Kubernetes Event 事件，并提供原因和解决办法：\n\n", language)
			prompt += fmt.Sprintf("事件名称: %s\n", req.ResourceName)
			prompt += fmt.Sprintf("命名空间: %s\n", req.Namespace)
			prompt += fmt.Sprintf("事件类型: %s\n", event.Type)
			prompt += fmt.Sprintf("原因: %s\n", event.Reason)
			prompt += fmt.Sprintf("消息: %s\n", event.Message)
			if event.InvolvedObject.Kind != "" {
				prompt += fmt.Sprintf("相关对象: %s/%s (%s)\n", event.InvolvedObject.Kind, event.InvolvedObject.Name, event.InvolvedObject.Namespace)
			}
			if event.Source.Component != "" {
				prompt += fmt.Sprintf("来源组件: %s\n", event.Source.Component)
			}
			if !event.FirstTimestamp.IsZero() {
				prompt += fmt.Sprintf("首次发生时间: %s\n", event.FirstTimestamp.Format("2006-01-02 15:04:05"))
			}
			if !event.LastTimestamp.IsZero() {
				prompt += fmt.Sprintf("最后发生时间: %s\n", event.LastTimestamp.Format("2006-01-02 15:04:05"))
			}
			if event.Count > 0 {
				prompt += fmt.Sprintf("发生次数: %d\n", event.Count)
			}
			prompt += "\n请提供：\n1. 事件的详细解释\n2. 可能的原因分析\n3. 处理办法和步骤\n4. 预防措施（如果适用）"

			// 调用 AI 生成解释
			explanation, err := s.aiManager.Explain(ctx, backend, prompt, language)
			if err != nil {
				klog.Warningf("Failed to get AI explanation for event: %v", err)
				// 即使 AI 解释失败，也返回基础结果
			} else {
				// 将 AI 解释添加到结果中
				result.Results[0].Details = explanation
				result.Provider = backend
			}
		}
	}

	return result, nil
}

// DiagnoseEvent 诊断事件
func (s *DiagnosticService) DiagnoseEvent(ctx context.Context, cluster, namespace string, explain bool) (*DiagnosticResult, error) {
	// Event 分析器在 K8sGPT 中不存在，不传递过滤器
	// 让 K8sGPT 运行所有核心分析器，通过命名空间限制范围
	return s.Diagnose(ctx, DiagnosticRequest{
		Cluster:      cluster,
		Namespace:    namespace,
		ResourceType: "Event",
		Filters:      []string{}, // 不设置过滤器，运行所有核心分析器
		Explain:      explain,
		Language:     "中文",
	})
}

// ExplainResult AI 解释诊断结果
// ExplainRequest AI 解释请求
type ExplainRequest struct {
	Cluster  string   `json:"cluster"`
	Kind     string   `json:"kind"`               // 资源类型
	Name     string   `json:"name"`               // 资源名称
	Errors   []string `json:"errors"`             // 错误列表
	Backend  string   `json:"backend,omitempty"`  // AI 后端名称
	Language string   `json:"language,omitempty"` // 语言，默认中文
}

// ExplainResult AI 解释结果
type ExplainResult struct {
	Explanation string `json:"explanation"`        // AI 生成的解释
	Provider    string `json:"provider,omitempty"` // 使用的 AI 提供者
}

// Explain AI 解释诊断结果
func (s *DiagnosticService) Explain(ctx context.Context, req ExplainRequest) (*ExplainResult, error) {
	// 确定使用的 AI 后端
	backend := req.Backend
	if backend == "" {
		config, err := s.aiManager.ListProviders()
		if err != nil {
			return nil, fmt.Errorf("failed to get AI config: %w", err)
		}
		if config.DefaultProvider != "" {
			backend = config.DefaultProvider
		} else if len(config.Providers) > 0 {
			backend = config.Providers[0].Name
		} else {
			return nil, fmt.Errorf("no AI provider configured")
		}
	}

	// 确定语言
	language := req.Language
	if language == "" {
		language = "中文"
	}

	// 构建提示词
	prompt := fmt.Sprintf("请用%s详细解释以下 Kubernetes 资源的问题，并提供处理办法：\n\n", language)
	prompt += fmt.Sprintf("资源类型: %s\n", req.Kind)
	prompt += fmt.Sprintf("资源名称: %s\n", req.Name)
	prompt += "错误信息:\n"
	for i, err := range req.Errors {
		prompt += fmt.Sprintf("%d. %s\n", i+1, err)
	}
	prompt += "\n请提供：\n1. 问题的详细解释\n2. 可能的原因\n3. 处理办法和步骤"

	// 调用 AI 后端生成解释
	explanation, err := s.aiManager.Explain(ctx, backend, prompt, language)
	if err != nil {
		// 检查是否是模型相关的错误
		errMsg := err.Error()
		if strings.Contains(errMsg, "does not exist") || strings.Contains(errMsg, "not have access") {
			// 获取提供者配置以显示模型名称
			provider, providerErr := s.aiManager.GetProvider(backend)
			if providerErr == nil {
				return nil, fmt.Errorf("AI 模型 '%s' 不存在或您没有访问权限。请检查 AI 引擎配置中的模型名称是否正确", provider.Model)
			}
			return nil, fmt.Errorf("AI 模型不存在或您没有访问权限。请检查 AI 引擎配置")
		}
		if strings.Contains(errMsg, "status code: 404") {
			return nil, fmt.Errorf("AI 服务返回 404 错误，可能是模型名称配置错误或 API 端点不正确")
		}
		if strings.Contains(errMsg, "status code: 401") || strings.Contains(errMsg, "unauthorized") {
			return nil, fmt.Errorf("AI 服务认证失败，请检查 API Key 是否正确")
		}
		return nil, fmt.Errorf("获取 AI 解释失败: %w", err)
	}

	return &ExplainResult{
		Explanation: explanation,
		Provider:    backend,
	}, nil
}

// checkCronJobAPISupport 检查集群是否支持 CronJob 的 BatchV1 API
func checkCronJobAPISupport(ctx context.Context, k8sGPTClient *k8sgptk8s.Client) error {
	// 尝试通过 ServerResourcesForGroupVersion 检查 API 是否可用
	discoveryClient := k8sGPTClient.GetClient().Discovery()

	// 检查 batch/v1 API 组是否可用
	apiResourceList, err := discoveryClient.ServerResourcesForGroupVersion("batch/v1")
	if err != nil {
		return fmt.Errorf("集群不支持 batch/v1 API: %w", err)
	}

	// 检查是否有 CronJob 资源
	hasCronJob := false
	for _, resource := range apiResourceList.APIResources {
		if resource.Kind == "CronJob" {
			hasCronJob = true
			break
		}
	}

	if !hasCronJob {
		return fmt.Errorf("集群的 batch/v1 API 中未找到 CronJob 资源类型")
	}

	// 尝试实际调用 API（使用空命名空间列表，这不会真正创建资源，只是检查 API 是否可用）
	_, err = k8sGPTClient.GetClient().BatchV1().CronJobs("").List(ctx, metav1.ListOptions{Limit: 1})
	if err != nil {
		// 如果错误是 "the server could not find the requested resource"，说明 API 不可用
		errMsg := err.Error()
		if strings.Contains(errMsg, "could not find the requested resource") ||
			strings.Contains(errMsg, "the server could not find the requested resource") {
			return fmt.Errorf("集群不支持 CronJob 的 BatchV1 API，错误: %w", err)
		}
		// 其他错误（如权限问题、命名空间不存在等）不算 API 不支持
		// 这些错误在后续的分析中可能会被正确处理
		klog.V(4).Infof("CronJob API test call returned error (may be expected): %v", err)
	}

	return nil
}

// checkIngressAPISupport 检查集群是否支持 Ingress 的 NetworkingV1 API
func checkIngressAPISupport(ctx context.Context, k8sGPTClient *k8sgptk8s.Client) error {
	// 尝试通过 ServerResourcesForGroupVersion 检查 API 是否可用
	discoveryClient := k8sGPTClient.GetClient().Discovery()

	// 检查 networking.k8s.io/v1 API 组是否可用
	apiResourceList, err := discoveryClient.ServerResourcesForGroupVersion("networking.k8s.io/v1")
	if err != nil {
		return fmt.Errorf("集群不支持 networking.k8s.io/v1 API: %w", err)
	}

	// 检查是否有 Ingress 资源
	hasIngress := false
	for _, resource := range apiResourceList.APIResources {
		if resource.Kind == "Ingress" {
			hasIngress = true
			break
		}
	}

	if !hasIngress {
		return fmt.Errorf("集群的 networking.k8s.io/v1 API 中未找到 Ingress 资源类型")
	}

	// 尝试实际调用 API（使用空命名空间列表，这不会真正创建资源，只是检查 API 是否可用）
	_, err = k8sGPTClient.GetClient().NetworkingV1().Ingresses("").List(ctx, metav1.ListOptions{Limit: 1})
	if err != nil {
		// 如果错误是 "the server could not find the requested resource"，说明 API 不可用
		errMsg := err.Error()
		if strings.Contains(errMsg, "could not find the requested resource") ||
			strings.Contains(errMsg, "the server could not find the requested resource") {
			return fmt.Errorf("集群不支持 Ingress 的 NetworkingV1 API，错误: %w", err)
		}
		// 其他错误（如权限问题、命名空间不存在等）不算 API 不支持
		// 这些错误在后续的分析中可能会被正确处理
		klog.V(4).Infof("Ingress API test call returned error (may be expected): %v", err)
	}

	return nil
}

// buildK8sGPTClient 构建 k8sgpt 的 Kubernetes 客户端
func buildK8sGPTClient(manager *client.ClusterManager) (*k8sgptk8s.Client, error) {
	// 从 manager 的 Config 构建 k8sgpt 客户端
	config := manager.Config

	// 创建 Kubernetes clientset
	clientSet, err := k8sclient.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create kubernetes clientset: %w", err)
	}

	// 创建 controller-runtime client
	ctrlClient, err := ctrl.New(config, ctrl.Options{})
	if err != nil {
		return nil, fmt.Errorf("failed to create controller-runtime client: %w", err)
	}

	// 获取服务器版本
	serverVersion, err := clientSet.ServerVersion()
	if err != nil {
		return nil, fmt.Errorf("failed to get server version: %w", err)
	}

	// 创建 dynamic client
	dynamicClient, err := dynamic.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create dynamic client: %w", err)
	}

	// 构建 k8sgpt Client
	return &k8sgptk8s.Client{
		Client:        clientSet,
		CtrlClient:    ctrlClient,
		Config:        config,
		ServerVersion: serverVersion,
		DynamicClient: dynamicClient,
	}, nil
}
