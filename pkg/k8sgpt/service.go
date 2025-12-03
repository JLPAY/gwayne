package k8sgpt

import (
	"context"
	"fmt"
	"strings"

	"github.com/JLPAY/gwayne/models"
	"github.com/JLPAY/gwayne/pkg/kubernetes/client"
	"github.com/k8sgpt-ai/k8sgpt/pkg/ai"
	"github.com/k8sgpt-ai/k8sgpt/pkg/analyzer"
	"github.com/k8sgpt-ai/k8sgpt/pkg/common"
	"github.com/k8sgpt-ai/k8sgpt/pkg/kubernetes"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime/pkg/client"
)

// K8sGPTService K8sGPT服务包装器
type K8sGPTService struct {
	aiProvider ai.IAI
}

// NewK8sGPTService 创建新的K8sGPT服务实例
func NewK8sGPTService(backendID int64) (*K8sGPTService, error) {
	// 获取AI后端配置
	backend, err := models.GetAIBackendByID(backendID)
	if err != nil {
		// 如果指定ID不存在，尝试获取默认后端
		backend, err = models.GetDefaultAIBackend()
		if err != nil {
			return nil, fmt.Errorf("无法找到AI后端配置: %w", err)
		}
	}

	if !backend.Enabled {
		return nil, fmt.Errorf("AI后端 %s 未启用", backend.Name)
	}

	// 创建AI客户端
	aiClient := ai.NewClient(backend.Provider)
	if aiClient == nil {
		return nil, fmt.Errorf("不支持的AI提供商: %s", backend.Provider)
	}

	// 配置AI客户端
	aiConfig := &ai.AIProvider{
		Name:        backend.Name,
		Model:       backend.Model,
		Password:    backend.APIKey,
		BaseURL:     backend.BaseURL,
		Temperature: float32(backend.Temperature),
	}

	if err := aiClient.Configure(aiConfig); err != nil {
		return nil, fmt.Errorf("配置AI客户端失败: %w", err)
	}

	return &K8sGPTService{
		aiProvider: aiClient,
	}, nil
}

// Analyze 执行Kubernetes集群诊断分析
func (s *K8sGPTService) Analyze(ctx context.Context, clusterName string, namespace string, filters []string) ([]common.Result, error) {
	// 获取集群管理器
	manager, err := client.Manager(clusterName)
	if err != nil {
		return nil, fmt.Errorf("获取集群管理器失败: %w", err)
	}

	// 获取服务器版本
	serverVersion, err := manager.Client.ServerVersion()
	if err != nil {
		return nil, fmt.Errorf("获取服务器版本失败: %w", err)
	}

	// 创建controller-runtime客户端
	ctrlClient, err := ctrl.New(manager.Config, ctrl.Options{})
	if err != nil {
		// 如果创建失败，可以继续，某些分析器可能不需要
		klog.Warningf("创建controller-runtime客户端失败: %v", err)
	}

	// 创建k8sgpt的kubernetes客户端包装器
	k8sgptClient := &kubernetes.Client{
		Client:        manager.Client,
		Config:        manager.Config,
		DynamicClient: manager.DynamicClient,
		ServerVersion: serverVersion,
		CtrlClient:    ctrlClient,
	}

	// 创建分析器配置
	analyzerConfig := common.Analyzer{
		Client:    k8sgptClient,
		Context:   ctx,
		Namespace: namespace,
		AIClient:  s.aiProvider,
	}

	// 获取所有分析器
	coreAnalyzerMap, additionalAnalyzerMap := analyzer.GetAnalyzerMap()
	allAnalyzers := make(map[string]common.IAnalyzer)
	for k, v := range coreAnalyzerMap {
		allAnalyzers[k] = v
	}
	for k, v := range additionalAnalyzerMap {
		allAnalyzers[k] = v
	}

	var results []common.Result

	// 如果没有指定过滤器，使用所有分析器
	if len(filters) == 0 {
		filters = make([]string, 0, len(allAnalyzers))
		for k := range allAnalyzers {
			filters = append(filters, k)
		}
	}

	// 执行分析
	for _, filter := range filters {
		analyzerInstance, exists := allAnalyzers[filter]
		if !exists {
			klog.Warningf("分析器 %s 不存在，跳过", filter)
			continue
		}

		analyzerResults, err := analyzerInstance.Analyze(analyzerConfig)
		if err != nil {
			klog.Errorf("分析器 %s 执行失败: %v", filter, err)
			continue
		}

		results = append(results, analyzerResults...)
	}

	// 为每个结果生成AI解释（Details字段）
	// 只有当结果有错误时才需要生成Details
	for i := range results {
		if len(results[i].Error) > 0 {
			// 收集所有错误文本
			var texts []string
			for _, failure := range results[i].Error {
				if failure.Text != "" {
					texts = append(texts, failure.Text)
				}
			}

			// 如果有错误文本，调用AI生成详细说明
			if len(texts) > 0 && s.aiProvider != nil {
				// 检查是否是 NoOp AI 客户端（测试用，不实际调用AI）
				if s.aiProvider.GetName() == "noopai" {
					// NoOp 客户端不实际调用AI，直接使用原始错误文本
					results[i].Details = strings.Join(texts, "; ")
				} else {
					// 使用默认提示模板
					promptTemplate := "请用中文详细解释以下Kubernetes资源问题：\n%s"
					
					// 构建提示
					inputKey := fmt.Sprintf("%s/%s: %s", results[i].Kind, results[i].Name, strings.Join(texts, "; "))
					prompt := fmt.Sprintf(promptTemplate, inputKey)
					
					// 调用AI生成解释
					response, err := s.aiProvider.GetCompletion(ctx, prompt)
					if err != nil {
						klog.Warningf("为结果 %s/%s 生成AI解释失败: %v", results[i].Kind, results[i].Name, err)
						// 如果AI调用失败，至少显示错误文本
						results[i].Details = strings.Join(texts, "; ")
					} else {
						// 清理响应：移除可能的提示词前缀
						cleanedResponse := cleanAIResponse(response, prompt)
						results[i].Details = cleanedResponse
					}
				}
			} else if len(texts) > 0 {
				// 如果没有AI提供者，至少显示错误文本
				results[i].Details = strings.Join(texts, "; ")
			}
		}
	}

	return results, nil
}

// cleanAIResponse 清理AI响应，移除提示词前缀和多余内容
func cleanAIResponse(response, prompt string) string {
	cleaned := strings.TrimSpace(response)
	
	// 移除常见的提示词前缀
	prefixes := []string{
		"I am a noop response to the prompt ",
		"请用中文详细解释以下Kubernetes资源问题：",
		"请用中文详细解释以下Kubernetes资源问题:",
		"以下是对Kubernetes资源问题的详细解释：",
		"以下是对Kubernetes资源问题的详细解释:",
	}
	
	for _, prefix := range prefixes {
		if strings.HasPrefix(cleaned, prefix) {
			cleaned = strings.TrimPrefix(cleaned, prefix)
			cleaned = strings.TrimSpace(cleaned)
		}
	}
	
	// 如果响应包含完整的提示词，尝试提取实际内容
	if strings.Contains(cleaned, prompt) {
		// 移除提示词部分
		cleaned = strings.ReplaceAll(cleaned, prompt, "")
		cleaned = strings.TrimSpace(cleaned)
	}
	
	// 如果清理后为空，返回原始响应
	if cleaned == "" {
		return response
	}
	
	return cleaned
}

// ListAnalyzers 列出所有可用的分析器
func ListAnalyzers() ([]string, []string, []string) {
	return analyzer.ListFilters()
}

// Close 关闭服务并清理资源
func (s *K8sGPTService) Close() {
	if s.aiProvider != nil {
		s.aiProvider.Close()
	}
}
