package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/JLPAY/gwayne/models"
	"k8s.io/klog/v2"
)

const (
	llmHTTPTimeout       = 150 * time.Second
	llmStreamHTTPTimeout = 300 * time.Second
)

// ModelConfig 调用 LLM 模型 所需的配置
type ModelConfig struct {
	Provider    string // deepseek, openai, ollama, azure 等 OpenAI 兼容后端
	APIKey      string
	BaseURL     string
	Model       string
	Temperature float64
	// MaxTokens 限制单次生成的最大 token 数（0 表示不限制）。
	// 生成时长与输出长度近似成正比，排查类场景建议设置上限以缩短等待。
	MaxTokens int
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatRequest struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	Temperature *float64      `json:"temperature,omitempty"`
	MaxTokens   *int          `json:"max_tokens,omitempty"`
	Stream      bool          `json:"stream,omitempty"`
}

type chatResponse struct {
	Choices []struct {
		FinishReason string `json:"finish_reason"`
		Message      struct {
			Content          string `json:"content"`
			ReasoningContent string `json:"reasoning_content"` // 思考类模型（如 deepseek-reasoner）的推理内容
		} `json:"message"`
	} `json:"choices"`

	// 部分网关把错误包在顶层 error 字段里
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error,omitempty"`

	// 部分网关把错误包在 choices[0].message.content 的 JSON 字符串里，或使用其它字段名，
	// 保留原始响应便于诊断（仅在内容为空时打日志）
	Raw string `json:"-"`
}

// defaultBaseURL 返回常见提供商的默认 API 地址
func defaultBaseURL(provider string) string {
	switch provider {
	case "openai":
		return "https://api.openai.com/v1"
	case "deepseek":
		return "https://api.deepseek.com"
	case "moonshot":
		return "https://api.moonshot.cn/v1"
	case "zhipu", "glm":
		return "https://open.bigmodel.cn/api/paas/v4"
	case "ollama":
		return "http://127.0.0.1:11434/v1"
	default:
		return ""
	}
}

// buildChatURL 根据 BaseURL / Provider 推导 chat completions 完整 URL
func buildChatURL(cfg ModelConfig) (string, error) {
	provider := strings.ToLower(cfg.Provider)

	if provider == "azureopenai" || provider == "azure" {
		base := strings.TrimRight(cfg.BaseURL, "/")
		if base == "" {
			return "", fmt.Errorf("azure openai 必须配置 BaseURL")
		}
		// Azure 网关格式: {base}/openai/deployments/{model}/chat/completions
		if strings.Contains(base, "/openai/deployments/") {
			return base + "/chat/completions?api-version=2024-06-01", nil
		}
		return fmt.Sprintf("%s/openai/deployments/%s/chat/completions?api-version=2024-06-01", base, cfg.Model), nil
	}

	base := strings.TrimRight(cfg.BaseURL, "/")
	if base == "" {
		base = defaultBaseURL(provider)
	}
	if base == "" {
		return "", fmt.Errorf("LLM 模型 %s 未配置 BaseURL，且没有默认地址", cfg.Provider)
	}
	if strings.HasSuffix(base, "/chat/completions") {
		return base, nil
	}
	if strings.HasSuffix(base, "/v1") || strings.HasSuffix(base, "/v4") || strings.HasSuffix(base, "/v3") {
		return base + "/chat/completions", nil
	}
	return base + "/v1/chat/completions", nil
}

// ResolveModel 解析要使用的 LLM 模型：
// 优先使用指定的 modelID（不存在时忽略），其次默认模型，最后回退到第一个启用的模型。
func ResolveModel(modelID int64) (*models.LLMModel, error) {
	if modelID > 0 {
		m, err := models.GetLLMModelByID(modelID)
		if err == nil {
			return m, nil
		}
		klog.Warningf("指定的 LLM 模型 %d 不存在，回退到默认模型: %v", modelID, err)
	}

	m, err := models.GetDefaultLLMModel()
	if err == nil {
		return m, nil
	}

	ms, lerr := models.GetAllEnabledLLMModels()
	if lerr != nil || len(ms) == 0 {
		return nil, fmt.Errorf("未找到可用的 LLM 模型，请先在系统管理中添加并启用 LLM 模型: %w", err)
	}
	return &ms[0], nil
}

// IsModelConfigured 判断是否存在可用的 LLM 模型（默认模型或任一启用中的模型），
// 用于前端判断是否需要展示「Pod AI 排查」等 AI 功能入口。
func IsModelConfigured() bool {
	if _, err := models.GetDefaultLLMModel(); err == nil {
		return true
	}
	ms, err := models.GetAllEnabledLLMModels()
	return err == nil && len(ms) > 0
}

// ConfigFromModel 将模型记录转换为调用配置
func ConfigFromModel(m *models.LLMModel) ModelConfig {
	return ModelConfig{
		Provider:    m.Provider,
		APIKey:      m.APIKey,
		BaseURL:     m.BaseURL,
		Model:       m.Model,
		Temperature: m.Temperature,
	}
}

// ChatCompletion 调用 OpenAI 兼容的 chat completions 接口，返回模型回答文本
func ChatCompletion(ctx context.Context, cfg ModelConfig, systemPrompt, userPrompt string) (string, error) {
	if cfg.Model == "" {
		return "", fmt.Errorf("LLM 模型 未配置模型名称")
	}

	url, err := buildChatURL(cfg)
	if err != nil {
		return "", err
	}

	reqBody := chatRequest{
		Model: cfg.Model,
		Messages: []chatMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		},
	}
	if cfg.Temperature > 0 {
		reqBody.Temperature = &cfg.Temperature
	}
	if cfg.MaxTokens > 0 {
		mt := cfg.MaxTokens
		reqBody.MaxTokens = &mt
	}

	payload, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("序列化请求失败: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, llmHTTPTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("构造 AI 请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	provider := strings.ToLower(cfg.Provider)
	if provider == "azureopenai" || provider == "azure" {
		req.Header.Set("api-key", cfg.APIKey)
	} else if cfg.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+cfg.APIKey)
	}

	resp, err := (&http.Client{}).Do(req)
	if err != nil {
		return "", fmt.Errorf("调用 AI 接口失败(%s): %w", url, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return "", fmt.Errorf("读取 AI 响应失败: %w", err)
	}

	var chatResp chatResponse
	if err := json.Unmarshal(body, &chatResp); err != nil {
		return "", fmt.Errorf("解析 AI 响应失败(HTTP %d): %s", resp.StatusCode, excerpt(string(body), 512))
	}
	if chatResp.Error != nil && chatResp.Error.Message != "" {
		return "", fmt.Errorf("AI 接口返回错误: %s", chatResp.Error.Message)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("AI 接口返回 HTTP %d: %s", resp.StatusCode, excerpt(string(body), 512))
	}
	if len(chatResp.Choices) == 0 {
		klog.Errorf("LLM 响应无 choices, model=%s, url=%s, 原始响应: %s", cfg.Model, url, excerpt(string(body), 1024))
		return "", fmt.Errorf("AI 接口未返回内容(响应无 choices): %s", excerpt(string(body), 256))
	}

	msg := chatResp.Choices[0].Message
	// 思考类模型 content 可能为空，回退用推理内容
	content := msg.Content
	if content == "" {
		content = msg.ReasoningContent
	}
	if content == "" {
		klog.Errorf("LLM 响应内容为空, model=%s, finish_reason=%s, 原始响应: %s",
			cfg.Model, chatResp.Choices[0].FinishReason, excerpt(string(body), 1024))
		if chatResp.Choices[0].FinishReason == "length" {
			return "", fmt.Errorf("AI 输出被 max_tokens 截断且正文为空（推理类模型会把推理 token 也计入额度），请增大 max_tokens 或更换非推理模型")
		}
		return "", fmt.Errorf("AI 接口未返回内容(finish_reason=%s): %s",
			chatResp.Choices[0].FinishReason, excerpt(string(body), 256))
	}

	klog.V(4).Infof("LLM 模型 调用成功, model=%s, url=%s", cfg.Model, url)
	return content, nil
}

func excerpt(s string, max int) string {
	s = strings.TrimSpace(s)
	if len(s) <= max {
		return s
	}
	return s[:max] + "...(truncated)"
}

type chatStreamResponse struct {
	Choices []struct {
		Delta struct {
			Content string `json:"content"`
		} `json:"delta"`
		FinishReason *string `json:"finish_reason"`
	} `json:"choices"`

	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error,omitempty"`
}

// buildChatRequest 构造 chat completions HTTP 请求（流式与非流式共用）
func buildChatRequest(ctx context.Context, cfg ModelConfig, systemPrompt, userPrompt string, stream bool) (*http.Request, error) {
	if cfg.Model == "" {
		return nil, fmt.Errorf("LLM 模型 未配置模型名称")
	}

	url, err := buildChatURL(cfg)
	if err != nil {
		return nil, err
	}

	reqBody := chatRequest{
		Model: cfg.Model,
		Messages: []chatMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		},
		Stream: stream,
	}
	if cfg.Temperature > 0 {
		reqBody.Temperature = &cfg.Temperature
	}
	if cfg.MaxTokens > 0 {
		mt := cfg.MaxTokens
		reqBody.MaxTokens = &mt
	}

	payload, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("序列化请求失败: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("构造 AI 请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if stream {
		req.Header.Set("Accept", "text/event-stream")
	}

	provider := strings.ToLower(cfg.Provider)
	if provider == "azureopenai" || provider == "azure" {
		req.Header.Set("api-key", cfg.APIKey)
	} else if cfg.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+cfg.APIKey)
	}
	return req, nil
}

// ChatCompletionStream 以流式（SSE）方式调用 OpenAI 兼容的 chat completions 接口，
// 每收到一段增量文本就回调一次 onDelta，返回完整的模型输出。
// onDelta 为 nil 时行为退化为非流式调用（但走 stream 协议）。
func ChatCompletionStream(ctx context.Context, cfg ModelConfig, systemPrompt, userPrompt string, onDelta func(string) error) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, llmStreamHTTPTimeout)
	defer cancel()

	req, err := buildChatRequest(ctx, cfg, systemPrompt, userPrompt, true)
	if err != nil {
		return "", err
	}

	resp, err := (&http.Client{}).Do(req)
	if err != nil {
		return "", fmt.Errorf("调用 AI 接口失败: %w", err)
	}
	defer resp.Body.Close()

	// 流式模式下错误通常仍以 JSON 返回（非 SSE），先兜底检查
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
		var errResp chatResponse
		if json.Unmarshal(body, &errResp) == nil && errResp.Error != nil && errResp.Error.Message != "" {
			return "", fmt.Errorf("AI 接口返回错误: %s", errResp.Error.Message)
		}
		return "", fmt.Errorf("AI 接口返回 HTTP %d: %s", resp.StatusCode, excerpt(string(body), 512))
	}

	var full strings.Builder
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, ":") {
			continue
		}
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "[DONE]" {
			break
		}
		var chunk chatStreamResponse
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			// 忽略无法解析的心跳/注释行
			continue
		}
		if chunk.Error != nil && chunk.Error.Message != "" {
			return full.String(), fmt.Errorf("AI 接口返回错误: %s", chunk.Error.Message)
		}
		if len(chunk.Choices) == 0 {
			continue
		}
		delta := chunk.Choices[0].Delta.Content
		if delta == "" {
			continue
		}
		full.WriteString(delta)
		if onDelta != nil {
			if err := onDelta(delta); err != nil {
				return full.String(), err
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return full.String(), fmt.Errorf("读取 AI 流式响应失败: %w", err)
	}
	if full.Len() == 0 {
		return "", fmt.Errorf("AI 接口未返回内容")
	}

	klog.V(4).Infof("LLM 模型 流式调用成功, model=%s, 输出长度=%d", cfg.Model, full.Len())
	return full.String(), nil
}
