package inspect

import (
	"fmt"
	"strings"
	"time"

	k8snode "github.com/JLPAY/gwayne/pkg/kubernetes/resources/node"
)

// notifyMaxAbnormalItems 报告中最多列出的异常条目数（超出部分提示看控制台）
const notifyMaxAbnormalItems = 30

// checkLabels 后端侧巡检项中文名（企业微信消息不走前端 i18n）
var checkLabels = map[k8snode.InspectItemType]string{
	k8snode.ItemReady:         "节点就绪",
	k8snode.ItemMemory:        "内存",
	k8snode.ItemDisk:          "磁盘",
	k8snode.ItemCPU:           "CPU",
	k8snode.ItemLoad:          "系统负载",
	k8snode.ItemPressure:      "压力状态",
	k8snode.ItemFD:            "文件描述符",
	k8snode.ItemPod:           "Pod 数量",
	k8snode.ItemCertAPIServer: "API Server 证书",
}

// checkLabel 取巡检项中文名
func checkLabel(ch k8snode.CheckResult) string {
	if l, ok := checkLabels[ch.Type]; ok {
		return l
	}
	return string(ch.Type)
}

// BuildInspectReportMarkdown 生成企业微信机器人 markdown 巡检报告：
// 按集群给汇总，列出异常节点（含异常项与取值），异常过多时截断并提示看控制台。
func BuildInspectReportMarkdown(results []k8snode.NodeInspectionResult, runTime time.Time) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## 节点巡检报告\n> 时间：%s\n", runTime.Format("2006-01-02 15:04:05"))

	abnormal := 0
	var lines []string
	appendAbnormal := func(format string, args ...interface{}) {
		if abnormal >= notifyMaxAbnormalItems {
			return
		}
		abnormal++
		lines = append(lines, "> "+fmt.Sprintf(format, args...))
	}

	for _, r := range results {
		if r.Error != "" {
			fmt.Fprintf(&b, "\n**%s**：<font color=\"warning\">巡检失败：%s</font>\n", r.Cluster, r.Error)
			continue
		}
		fmt.Fprintf(&b, "\n**%s**：共 %d 节点，<font color=\"warning\">告警 %d</font>，<font color=\"warning\">严重 %d</font>\n",
			r.Cluster, r.Total, r.Warning, r.Critical)

		// 异常节点（含异常巡检项与取值）
		for _, it := range r.Items {
			if it.Status == k8snode.StatusOK {
				continue
			}
			bad := ""
			for _, ch := range it.Checks {
				if ch.Status == k8snode.StatusOK {
					continue
				}
				if bad != "" {
					bad += "、"
				}
				bad += fmt.Sprintf("%s %s", checkLabel(ch), ch.Value)
			}
			if it.Status == k8snode.StatusCritical {
				appendAbnormal("<font color=\"warning\">**%s**</font>：%s", it.Name, bad)
			} else {
				appendAbnormal("%s：%s", it.Name, bad)
			}
		}

		// 集群级巡检项异常（如 API Server 证书）
		for _, cc := range r.ClusterChecks {
			if cc.Status == k8snode.StatusOK {
				continue
			}
			appendAbnormal("<font color=\"warning\">[集群] %s</font>：%s", checkLabel(cc), cc.Value)
		}
	}

	if len(lines) > 0 {
		b.WriteString("\n**异常明细**\n")
		b.WriteString(strings.Join(lines, "\n"))
		b.WriteString("\n")
		if abnormal >= notifyMaxAbnormalItems {
			b.WriteString(fmt.Sprintf("\n<font color=\"comment\">异常条目过多，仅展示前 %d 条</font>\n", notifyMaxAbnormalItems))
		}
	} else {
		b.WriteString("\n<font color=\"info\">全部节点正常</font>\n")
	}
	b.WriteString("\n<font color=\"comment\">详情见 gwayne 控制台 - 巡检</font>")
	return b.String()
}
