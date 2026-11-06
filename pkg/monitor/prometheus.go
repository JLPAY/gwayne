package monitor

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// Sample 单条时序样本：metric 为标签集合，FloatValue 为采样值（Prometheus 返回值以字符串编码）。
type Sample struct {
	Metric     map[string]string `json:"metric"`
	Value      []interface{}     `json:"value"`
	FloatValue float64           `json:"-"`
}

// UnmarshalJSON 解析 Prometheus 的 [timestamp, "value"] 形式，将字符串值转成 float。
func (s *Sample) UnmarshalJSON(data []byte) error {
	var aux struct {
		Metric map[string]string `json:"metric"`
		Value  []interface{}     `json:"value"`
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	s.Metric = aux.Metric
	if len(aux.Value) == 2 {
		if str, ok := aux.Value[1].(string); ok {
			if f, err := strconv.ParseFloat(str, 64); err == nil {
				s.FloatValue = f
			}
		}
	}
	return nil
}

// queryResponse Prometheus 即时查询的标准响应结构。
type queryResponse struct {
	Status string `json:"status"`
	Data   struct {
		ResultType string   `json:"resultType"`
		Result     []Sample `json:"result"`
	} `json:"data"`
	Error     string `json:"error"`
	ErrorType string `json:"errorType"`
}

// InstantQuery 执行 Prometheus 即时查询（GET /api/v1/query?query=...），返回样本切片。
// baseURL 为到 /api/v1 的上一级地址；token 为空表示匿名访问。
func InstantQuery(baseURL, token, query string) ([]Sample, error) {
	if baseURL == "" {
		return nil, fmt.Errorf("Prometheus 地址未配置（请在巡检页面配置 Prometheus 数据源）")
	}
	u, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("Prometheus 地址非法: %v", err)
	}
	u.Path = "/api/v1/query"
	q := u.Query()
	q.Set("query", query)
	u.RawQuery = q.Encode()

	req, err := http.NewRequest(http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("查询 Prometheus 失败: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Prometheus 返回 HTTP %d: %s", resp.StatusCode, truncate(string(body), 300))
	}

	var qr queryResponse
	if err := json.Unmarshal(body, &qr); err != nil {
		return nil, fmt.Errorf("解析 Prometheus 响应失败: %v", err)
	}
	if qr.Status != "success" {
		return nil, fmt.Errorf("Prometheus 查询错误(%s): %s", qr.ErrorType, qr.Error)
	}
	return qr.Data.Result, nil
}

// truncate 截断过长文本，便于错误信息展示。
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
