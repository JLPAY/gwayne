package llmmodel

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/JLPAY/gwayne/models"
	pkgllm "github.com/JLPAY/gwayne/pkg/llm"
	"github.com/gin-gonic/gin"
	"k8s.io/klog/v2"
)

// fail 统一错误响应
func fail(c *gin.Context, status int, msg string) {
	c.JSON(status, gin.H{
		"code":    status,
		"message": "error",
		"msg":     msg,
		"error":   msg,
		"data": gin.H{
			"message": msg,
		},
	})
}

// ok 统一成功响应
func ok(c *gin.Context, data interface{}) {
	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "success",
		"data":    data,
	})
}

// ListLLMModels 获取所有 LLM 模型 列表
func ListLLMModels(c *gin.Context) {
	list, err := models.GetAllLLMModels()
	if err != nil {
		klog.Errorf("获取 LLM 模型 列表失败: %v", err)
		fail(c, http.StatusInternalServerError, "获取 LLM 模型 列表失败: "+err.Error())
		return
	}
	// 列表接口对 API 密钥做掩码处理，避免明文泄露给前端
	for i := range list {
		list[i].APIKeySet = list[i].APIKey != ""
		list[i].APIKey = list[i].MaskAPIKey()
	}
	ok(c, list)
}

// GetLLMModel 获取单个 LLM 模型 详情
func GetLLMModel(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		fail(c, http.StatusBadRequest, "无效的 ID 参数")
		return
	}

	model, err := models.GetLLMModelByID(id)
	if err != nil {
		klog.Errorf("获取 LLM 模型 %d 失败: %v", id, err)
		fail(c, http.StatusNotFound, "LLM 模型 不存在")
		return
	}
	// 详情接口同样对 API 密钥做掩码处理
	model.APIKeySet = model.APIKey != ""
	model.APIKey = model.MaskAPIKey()
	ok(c, model)
}

// validateLLMModel 校验创建/更新请求的必填字段
func validateLLMModel(model *models.LLMModel) string {
	if model.Name == "" {
		return "名称不能为空"
	}
	if model.Provider == "" {
		return "提供商不能为空"
	}
	if model.Model == "" {
		return "模型名称不能为空"
	}
	return ""
}

// CreateLLMModel 创建 LLM 模型
func CreateLLMModel(c *gin.Context) {
	var model models.LLMModel
	if err := c.ShouldBindJSON(&model); err != nil {
		fail(c, http.StatusBadRequest, "请求参数错误: "+err.Error())
		return
	}

	if msg := validateLLMModel(&model); msg != "" {
		fail(c, http.StatusBadRequest, msg)
		return
	}

	// 检查名称是否已存在
	if existing, _ := models.GetLLMModelByName(model.Name); existing != nil {
		fail(c, http.StatusConflict, "名称已存在")
		return
	}

	model.ID = 0
	model.Deleted = false
	if u, exists := c.Get("User"); exists {
		if user, ok := u.(*models.User); ok {
			model.User = user.Name
		}
	}

	id, err := models.AddLLMModel(&model)
	if err != nil {
		klog.Errorf("创建 LLM 模型 失败: %v", err)
		fail(c, http.StatusInternalServerError, "创建 LLM 模型 失败: "+err.Error())
		return
	}

	created, _ := models.GetLLMModelByID(id)
	ok(c, created)
}

// UpdateLLMModel 更新 LLM 模型
func UpdateLLMModel(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		fail(c, http.StatusBadRequest, "无效的 ID 参数")
		return
	}

	var req models.LLMModel
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "请求参数错误: "+err.Error())
		return
	}

	existing, err := models.GetLLMModelByID(id)
	if err != nil {
		fail(c, http.StatusNotFound, "LLM 模型 不存在")
		return
	}

	if req.Name != "" && req.Name != existing.Name {
		if other, _ := models.GetLLMModelByName(req.Name); other != nil && other.ID != existing.ID {
			fail(c, http.StatusConflict, "名称已存在")
			return
		}
	}

	// 只更新允许修改的字段
	existing.Name = req.Name
	existing.Provider = req.Provider
	// 仅在填写了非空且非掩码占位的新密钥时才覆盖，否则保留原密钥
	if req.APIKey != "" && !models.IsMaskedSecret(req.APIKey) {
		existing.APIKey = req.APIKey
	}
	existing.BaseURL = req.BaseURL
	existing.Model = req.Model
	existing.Temperature = req.Temperature
	existing.Enabled = req.Enabled
	existing.IsDefault = req.IsDefault
	existing.Description = req.Description
	existing.Config = req.Config

	if msg := validateLLMModel(existing); msg != "" {
		fail(c, http.StatusBadRequest, msg)
		return
	}

	if err := models.UpdateLLMModel(existing); err != nil {
		klog.Errorf("更新 LLM 模型 %d 失败: %v", id, err)
		fail(c, http.StatusInternalServerError, "更新 LLM 模型 失败: "+err.Error())
		return
	}

	updated, _ := models.GetLLMModelByID(id)
	ok(c, updated)
}

// DeleteLLMModel 删除 LLM 模型（软删除）
func DeleteLLMModel(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		fail(c, http.StatusBadRequest, "无效的 ID 参数")
		return
	}

	if err := models.DeleteLLMModel(id); err != nil {
		klog.Errorf("删除 LLM 模型 %d 失败: %v", id, err)
		fail(c, http.StatusInternalServerError, "删除 LLM 模型 失败: "+err.Error())
		return
	}
	ok(c, nil)
}

// SetDefaultLLMModel 设置默认 LLM 模型
func SetDefaultLLMModel(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		fail(c, http.StatusBadRequest, "无效的 ID 参数")
		return
	}

	if _, err := models.GetLLMModelByID(id); err != nil {
		fail(c, http.StatusNotFound, "LLM 模型 不存在")
		return
	}

	if err := models.SetDefaultLLMModel(id); err != nil {
		klog.Errorf("设置默认 LLM 模型 %d 失败: %v", id, err)
		fail(c, http.StatusInternalServerError, "设置默认 LLM 模型 失败: "+err.Error())
		return
	}
	ok(c, nil)
}

// testModelRequest 保存前连通性测试请求：直接使用表单当前值，无需先保存
type testModelRequest struct {
	ID          int64   `json:"id"`
	Provider    string  `json:"provider"`
	APIKey      string  `json:"apiKey"`
	BaseURL     string  `json:"baseURL"`
	Model       string  `json:"model"`
	Temperature float64 `json:"temperature"`
}

// TestLLMModelConfig 用表单提交的配置做连通性测试（添加/编辑弹窗中调用）。
// 编辑时若 apiKey 为空或为掩码占位，则回退使用库中原密钥，保证不改动密钥也能测试。
// POST /api/v1/llm-models/test
func TestLLMModelConfig(c *gin.Context) {
	var req testModelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "请求参数错误: "+err.Error())
		return
	}

	if req.Provider == "" {
		fail(c, http.StatusBadRequest, "提供商不能为空")
		return
	}
	if req.Model == "" {
		fail(c, http.StatusBadRequest, "模型名称不能为空")
		return
	}

	// 掩码或空密钥时回退到已保存的密钥（仅编辑场景）
	if req.APIKey == "" || models.IsMaskedSecret(req.APIKey) {
		if req.ID > 0 {
			if existing, err := models.GetLLMModelByID(req.ID); err == nil {
				req.APIKey = existing.APIKey
			}
		}
	}

	cfg := pkgllm.ModelConfig{
		Provider:    req.Provider,
		APIKey:      req.APIKey,
		BaseURL:     req.BaseURL,
		Model:       req.Model,
		Temperature: req.Temperature,
	}

	start := time.Now()
	ctx, cancel := context.WithTimeout(c.Request.Context(), 60*time.Second)
	defer cancel()

	reply, err := pkgllm.ChatCompletion(ctx, cfg,
		"你是连通性测试助手，请用一句话简短回答。", "你好，请回复「连接成功」以确认服务可用。")
	durationMs := time.Since(start).Milliseconds()

	if err != nil {
		klog.Errorf("LLM 模型配置连通性测试失败(provider=%s, model=%s): %v", req.Provider, req.Model, err)
		ok(c, gin.H{
			"success":    false,
			"error":      err.Error(),
			"durationMs": durationMs,
			"model":      req.Model,
		})
		return
	}

	ok(c, gin.H{
		"success":    true,
		"reply":      reply,
		"durationMs": durationMs,
		"model":      req.Model,
	})
}

// TestLLMModel 连通性测试：向指定模型发送一条简单消息，验证配置是否可用
func TestLLMModel(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		fail(c, http.StatusBadRequest, "无效的 ID 参数")
		return
	}

	model, err := models.GetLLMModelByID(id)
	if err != nil {
		fail(c, http.StatusNotFound, "LLM 模型 不存在")
		return
	}

	start := time.Now()
	ctx, cancel := context.WithTimeout(c.Request.Context(), 60*time.Second)
	defer cancel()

	reply, err := pkgllm.ChatCompletion(ctx, pkgllm.ConfigFromModel(model),
		"你是连通性测试助手，请用一句话简短回答。", "你好，请回复「连接成功」以确认服务可用。")
	durationMs := time.Since(start).Milliseconds()

	if err != nil {
		klog.Errorf("LLM 模型 %d(%s) 连通性测试失败: %v", model.ID, model.Name, err)
		ok(c, gin.H{
			"success":    false,
			"error":      err.Error(),
			"durationMs": durationMs,
			"model":      model.Model,
		})
		return
	}

	ok(c, gin.H{
		"success":    true,
		"reply":      reply,
		"durationMs": durationMs,
		"model":      model.Model,
	})
}
