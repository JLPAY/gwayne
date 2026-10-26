package models

import (
	"time"
)

const (
	TableNamePodDiagnosisReport = "pod_diagnosis_report"

	// PodDiagnosisStatusRunning 排查进行中（后台异步执行）
	PodDiagnosisStatusRunning = "running"
	// PodDiagnosisStatusSuccess 排查成功，content 为报告正文
	PodDiagnosisStatusSuccess = "success"
	// PodDiagnosisStatusFailed 排查失败，error 为失败原因
	PodDiagnosisStatusFailed = "failed"
)

// PodDiagnosisReport Pod 异常 AI 排查报告。
// 点击「AI 排查」后立即创建 running 记录并由后台 goroutine 执行，
// 完成后回填 content/error，前端可在 Pod 列表随时查看历史报告。
type PodDiagnosisReport struct {
	ID         int64      `gorm:"primary_key;auto_increment" json:"id,omitempty"`
	Cluster    string     `gorm:"index;size:128" json:"cluster,omitempty"`
	Namespace  string     `gorm:"index;size:128" json:"namespace,omitempty"`
	Pod        string     `gorm:"index;size:255" json:"pod,omitempty"`
	Status     string     `gorm:"index;size:16" json:"status,omitempty"`  // running | success | failed
	Model      string     `gorm:"size:255" json:"model,omitempty"`        // 使用的 LLM 模型（配置名/模型名）
	ModelName  string     `gorm:"size:128" json:"modelName,omitempty"`    // LLM 模型配置名称
	Content    string     `gorm:"type:longtext" json:"content,omitempty"` // 报告正文（Markdown）
	Error      string     `gorm:"type:text" json:"error,omitempty"`       // 失败原因
	DurationMs int64      `json:"durationMs,omitempty"`                   // 排查耗时（毫秒）
	CollectMs  int64      `json:"collectMs,omitempty"`                    // 现场采集耗时（毫秒）
	LLMMs      int64      `json:"llmMs,omitempty"`                        // LLM 生成耗时（毫秒）
	CreateTime *time.Time `gorm:"autoCreateTime" json:"createTime,omitempty"`
	UpdateTime *time.Time `gorm:"autoUpdateTime" json:"updateTime,omitempty"`
	User       string     `gorm:"size:128" json:"user,omitempty"` // 发起人
	Deleted    bool       `gorm:"default:false" json:"deleted,omitempty"`
}

// TableName 返回表名
func (PodDiagnosisReport) TableName() string {
	return TableNamePodDiagnosisReport
}

// AddPodDiagnosisReport 新建排查报告（初始 status=running）
func AddPodDiagnosisReport(r *PodDiagnosisReport) (int64, error) {
	err := DB.Create(r).Error
	if err != nil {
		return 0, err
	}
	return r.ID, nil
}

// UpdatePodDiagnosisReport 回填报告结果（status/content/error/durationMs）
func UpdatePodDiagnosisReport(r *PodDiagnosisReport) error {
	return DB.Model(&PodDiagnosisReport{}).Where("id = ?", r.ID).Updates(map[string]interface{}{
		"status":      r.Status,
		"content":     r.Content,
		"error":       r.Error,
		"duration_ms": r.DurationMs,
		"collect_ms":  r.CollectMs,
		"llm_ms":      r.LLMMs,
		"model":       r.Model,
	}).Error
}

// GetPodDiagnosisReportByID 根据 ID 获取报告
func GetPodDiagnosisReportByID(id int64) (*PodDiagnosisReport, error) {
	var r PodDiagnosisReport
	err := DB.Where("id = ? AND deleted = ?", id, false).First(&r).Error
	if err != nil {
		return nil, err
	}
	return &r, nil
}

// PodDiagnosisReportFilter 报告查询过滤条件（均可选）
type PodDiagnosisReportFilter struct {
	Cluster   string
	Namespace string
	Pod       string
	Status    string
	Limit     int
}

// GetPodDiagnosisReports 按条件查询报告，按创建时间倒序
func GetPodDiagnosisReports(f PodDiagnosisReportFilter) ([]PodDiagnosisReport, error) {
	q := DB.Where("deleted = ?", false)
	if f.Cluster != "" {
		q = q.Where("cluster = ?", f.Cluster)
	}
	if f.Namespace != "" {
		q = q.Where("namespace = ?", f.Namespace)
	}
	if f.Pod != "" {
		q = q.Where("pod = ?", f.Pod)
	}
	if f.Status != "" {
		q = q.Where("status = ?", f.Status)
	}
	if f.Limit <= 0 || f.Limit > 200 {
		f.Limit = 50
	}
	var rs []PodDiagnosisReport
	err := q.Order("create_time DESC").Limit(f.Limit).Find(&rs).Error
	return rs, err
}

// HasRunningPodDiagnosis 是否存在进行中的排查任务（用于前端轮询判断）
func HasRunningPodDiagnosis() (bool, error) {
	var count int64
	err := DB.Model(&PodDiagnosisReport{}).
		Where("deleted = ? AND status = ?", false, PodDiagnosisStatusRunning).
		Count(&count).Error
	return count > 0, err
}

// DeletePodDiagnosisReport 删除报告（软删除）
func DeletePodDiagnosisReport(id int64) error {
	return DB.Model(&PodDiagnosisReport{}).Where("id = ?", id).Update("deleted", true).Error
}

// MarkStaleRunningReportsFailed 将遗留的 running 报告标记为失败。
// 排查任务在内存 goroutine 中执行，后端重启会中断任务、记录停留在 running，
// 服务启动时调用本函数清理，避免前端无限轮询「排查中」。
func MarkStaleRunningReportsFailed() (int64, error) {
	q := DB.Model(&PodDiagnosisReport{}).
		Where("deleted = ? AND status = ?", false, PodDiagnosisStatusRunning).
		Updates(map[string]interface{}{
			"status": PodDiagnosisStatusFailed,
			"error":  "服务重启导致排查任务中断，请重新发起",
		})
	return q.RowsAffected, q.Error
}
