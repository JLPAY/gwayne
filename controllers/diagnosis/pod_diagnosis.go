package diagnosis

import (
	"context"
	"net/http"
	"strconv"
	"time"

	pdiag "github.com/JLPAY/gwayne/pkg/diagnosis"
	pkgllm "github.com/JLPAY/gwayne/pkg/llm"

	"github.com/JLPAY/gwayne/models"
	"github.com/gin-gonic/gin"
	"k8s.io/klog/v2"
)

func fail(c *gin.Context, status int, msg string) {
	c.JSON(status, gin.H{"code": status, "message": "error", "msg": msg, "error": msg})
}

func ok(c *gin.Context, data interface{}) {
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "success", "data": data})
}

type podDiagnoseRequest struct {
	Cluster   string `json:"cluster"`
	Namespace string `json:"namespace"`
	Pod       string `json:"pod"`
	ModelID   int64  `json:"modelId"`
}

// DiagnosisAvailable 判断是否存在可用的 LLM 模型，供前端决定是否展示「Pod AI 排查」入口。
// GET /api/v1/diagnosis/available
func DiagnosisAvailable(c *gin.Context) {
	available := pkgllm.IsModelConfigured()
	name := ""
	if available {
		if m, err := pkgllm.ResolveModel(0); err == nil {
			name = m.Name
		}
	}
	ok(c, gin.H{"available": available, "defaultModel": name})
}

// DiagnosePodAsync 提交 Pod 异常 AI 排查任务：立即创建 running 报告记录并返回，
// 后台 goroutine 采集现场并调用 LLM，完成后回填报告内容。前端不阻塞、可继续其它操作。
// POST /api/v1/diagnosis/pod
func DiagnosePodAsync(c *gin.Context) {
	var req podDiagnoseRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "请求参数错误: "+err.Error())
		return
	}
	if req.Cluster == "" || req.Namespace == "" || req.Pod == "" {
		fail(c, http.StatusBadRequest, "cluster、namespace、pod 均为必填")
		return
	}

	model, err := pkgllm.ResolveModel(req.ModelID)
	if err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}

	// 创建 running 状态的报告记录
	report := &models.PodDiagnosisReport{
		Cluster:   req.Cluster,
		Namespace: req.Namespace,
		Pod:       req.Pod,
		Status:    models.PodDiagnosisStatusRunning,
		Model:     model.Model,
		ModelName: model.Name,
	}
	if u, exists := c.Get("User"); exists {
		if user, ok := u.(*models.User); ok {
			report.User = user.Name
		}
	}
	id, err := models.AddPodDiagnosisReport(report)
	if err != nil {
		klog.Errorf("创建 Pod 排查报告记录失败: %v", err)
		fail(c, http.StatusInternalServerError, "创建排查任务失败: "+err.Error())
		return
	}

	// 后台执行排查，不占用请求上下文（请求立即返回）
	go runPodDiagnosis(id, req, model)

	ok(c, gin.H{
		"id":        id,
		"status":    models.PodDiagnosisStatusRunning,
		"pod":       req.Pod,
		"modelName": model.Name,
	})
}

// runPodDiagnosis 后台执行：采集 Pod 现场 -> 调用 LLM -> 回填报告
func runPodDiagnosis(reportID int64, req podDiagnoseRequest, model *models.LLMModel) {
	totalStart := time.Now()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// 采集现场（内部已并发拉日志并限时）
	collectStart := time.Now()
	pc, err := pdiag.CollectPodContext(ctx, req.Cluster, req.Namespace, req.Pod)
	collectMs := time.Since(collectStart).Milliseconds()
	if err != nil {
		klog.Errorf("[排查报告 %d] 采集 Pod %s/%s 现场失败(耗时 %dms): %v", reportID, req.Namespace, req.Pod, collectMs, err)
		updateReportFailed(reportID, "采集 Pod 现场失败: "+err.Error(), time.Since(totalStart).Milliseconds(), collectMs, 0)
		return
	}

	snapshot := pdiag.BuildSnapshot(pc)
	klog.V(2).Infof("[排查报告 %d] Pod %s/%s 现场采集完成(耗时 %dms), 快照长度=%d, 事件=%d, 日志段=%d",
		reportID, req.Namespace, req.Pod, collectMs, len(snapshot), len(pc.Events), len(pc.Logs))

	// 调用 LLM 生成报告。
	// 注意：推理类模型（如 deepseek-flash/reasoner）的 reasoning tokens 也计入 max_tokens，
	// 上限太小会被推理消耗殆尽导致 content 为空（finish_reason=length），这里给足额度。
	cfg := pkgllm.ConfigFromModel(model)
	cfg.MaxTokens = 8192
	llmStart := time.Now()
	content, err := pkgllm.ChatCompletion(ctx, cfg, pdiag.SystemPrompt(), snapshot)
	llmMs := time.Since(llmStart).Milliseconds()
	durationMs := time.Since(totalStart).Milliseconds()
	if err != nil {
		klog.Errorf("[排查报告 %d] AI 排查 Pod %s/%s 失败(采集 %dms, LLM %dms): %v", reportID, req.Namespace, req.Pod, collectMs, llmMs, err)
		updateReportFailed(reportID, "AI 分析失败: "+err.Error(), durationMs, collectMs, llmMs)
		return
	}

	// 回填成功结果
	r, err := models.GetPodDiagnosisReportByID(reportID)
	if err != nil {
		klog.Errorf("[排查报告 %d] 读取报告记录失败: %v", reportID, err)
		return
	}
	r.Status = models.PodDiagnosisStatusSuccess
	r.Content = content
	r.DurationMs = durationMs
	r.CollectMs = collectMs
	r.LLMMs = llmMs
	if err := models.UpdatePodDiagnosisReport(r); err != nil {
		klog.Errorf("[排查报告 %d] 回填报告失败: %v", reportID, err)
		return
	}
	klog.V(2).Infof("[排查报告 %d] Pod %s/%s 排查完成，采集 %dms + LLM %dms = %dms",
		reportID, req.Namespace, req.Pod, collectMs, llmMs, durationMs)
}

// updateReportFailed 将报告标记为失败
func updateReportFailed(reportID int64, errMsg string, durationMs, collectMs, llmMs int64) {
	r, err := models.GetPodDiagnosisReportByID(reportID)
	if err != nil {
		klog.Errorf("[排查报告 %d] 读取报告记录失败: %v", reportID, err)
		return
	}
	r.Status = models.PodDiagnosisStatusFailed
	r.Error = errMsg
	r.DurationMs = durationMs
	r.CollectMs = collectMs
	r.LLMMs = llmMs
	if err := models.UpdatePodDiagnosisReport(r); err != nil {
		klog.Errorf("[排查报告 %d] 回填失败状态出错: %v", reportID, err)
	}
}

// ListPodDiagnosisReports 查询排查报告列表（支持 cluster/namespace/pod/status 过滤）。
// GET /api/v1/diagnosis/reports?cluster=&namespace=&pod=&status=&limit=
func ListPodDiagnosisReports(c *gin.Context) {
	limit, _ := strconv.Atoi(c.Query("limit"))
	reports, err := models.GetPodDiagnosisReports(models.PodDiagnosisReportFilter{
		Cluster:   c.Query("cluster"),
		Namespace: c.Query("namespace"),
		Pod:       c.Query("pod"),
		Status:    c.Query("status"),
		Limit:     limit,
	})
	if err != nil {
		klog.Errorf("查询排查报告列表失败: %v", err)
		fail(c, http.StatusInternalServerError, "查询排查报告失败: "+err.Error())
		return
	}
	// 附带是否有进行中任务，供前端决定是否继续轮询
	running, _ := models.HasRunningPodDiagnosis()
	ok(c, gin.H{"items": reports, "running": running})
}

// GetPodDiagnosisReport 获取单份报告详情（含正文）
// GET /api/v1/diagnosis/reports/:id
func GetPodDiagnosisReport(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		fail(c, http.StatusBadRequest, "无效的 ID 参数")
		return
	}
	report, err := models.GetPodDiagnosisReportByID(id)
	if err != nil {
		fail(c, http.StatusNotFound, "排查报告不存在")
		return
	}
	ok(c, report)
}

// DeletePodDiagnosisReport 删除排查报告
// DELETE /api/v1/diagnosis/reports/:id
func DeletePodDiagnosisReport(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		fail(c, http.StatusBadRequest, "无效的 ID 参数")
		return
	}
	if err := models.DeletePodDiagnosisReport(id); err != nil {
		klog.Errorf("删除排查报告 %d 失败: %v", id, err)
		fail(c, http.StatusInternalServerError, "删除排查报告失败: "+err.Error())
		return
	}
	ok(c, nil)
}
