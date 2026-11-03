package node

import (
	"net/http"
	"strconv"
	"time"

	"github.com/JLPAY/gwayne/models"
	"github.com/gin-gonic/gin"
	"k8s.io/klog/v2"
)

// History 查询巡检历史明细
// GET /api/v1/kubernetes/nodes/inspect/history?cluster=&node=&checkType=&from=&to=&limit=
// from/to 支持 RFC3339 或 "2006-01-02"（按本地时区）。
func History(c *gin.Context) {
	cluster := c.Query("cluster")
	nodeName := c.Query("node")
	checkType := c.Query("checkType")
	from := parseTimeParam(c.Query("from"))
	to := parseTimeParam(c.Query("to"))
	limit, _ := strconv.Atoi(c.Query("limit"))

	rows, err := models.QueryNodeInspectHistory(cluster, nodeName, checkType, from, to, limit)
	if err != nil {
		klog.Errorf("查询巡检历史失败: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": rows})
}

// HistoryRuns 查询历次巡检运行汇总
// GET /api/v1/kubernetes/nodes/inspect/history/runs?cluster=&limit=
func HistoryRuns(c *gin.Context) {
	cluster := c.Query("cluster")
	limit, _ := strconv.Atoi(c.Query("limit"))

	rows, err := models.QueryNodeInspectRuns(cluster, limit)
	if err != nil {
		klog.Errorf("查询巡检运行汇总失败: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": rows})
}

// parseTimeParam 解析时间参数：RFC3339 优先，其次 YYYY-MM-DD（按本地时区）。
func parseTimeParam(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t
	}
	if t, err := time.ParseInLocation("2006-01-02", s, time.Local); err == nil {
		return t
	}
	return time.Time{}
}
