package notify

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
	"unicode/utf8"
)

// wecomContentMaxBytes 企业微信 markdown 消息正文上限（UTF-8 字节数）
const wecomContentMaxBytes = 4096

// wecomTimeout webhook 请求超时
const wecomTimeout = 10 * time.Second

// wecomResp 企业微信 webhook 响应：{"errcode":0,"errmsg":"ok"}
type wecomResp struct {
	ErrCode int    `json:"errcode"`
	ErrMsg  string `json:"errmsg"`
}

// SendWeComMarkdown 推送 markdown 消息到企业微信群机器人。
// webhookURL 形如 https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=xxx。
// content 超过 4096 字节（UTF-8）时自动截断并追加省略号。
func SendWeComMarkdown(webhookURL, content string) error {
	if webhookURL == "" {
		return fmt.Errorf("webhook 地址为空")
	}
	payload, err := json.Marshal(map[string]interface{}{
		"msgtype":  "markdown",
		"markdown": map[string]string{"content": truncateUTF8(content, wecomContentMaxBytes)},
	})
	if err != nil {
		return fmt.Errorf("序列化企业微信消息失败: %v", err)
	}
	cli := &http.Client{Timeout: wecomTimeout}
	resp, err := cli.Post(webhookURL, "application/json", bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("请求企业微信 webhook 失败: %v", err)
	}
	defer resp.Body.Close()
	var r wecomResp
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return fmt.Errorf("解析企业微信响应失败(http %d): %v", resp.StatusCode, err)
	}
	if r.ErrCode != 0 {
		return fmt.Errorf("企业微信返回错误 errcode=%d errmsg=%s", r.ErrCode, r.ErrMsg)
	}
	return nil
}

// truncateUTF8 按字节数截断字符串且不切断多字节字符，截断时追加 "...";
// 中文字符 3 字节，emoji 4 字节，需按 rune 边界回退。
func truncateUTF8(s string, max int) string {
	if len(s) <= max {
		return s
	}
	limit := max - 3
	if limit <= 0 {
		return ""
	}
	b := []byte(s)[:limit]
	for len(b) > 0 {
		r, size := utf8.DecodeLastRune(b)
		if r == utf8.RuneError && size <= 1 {
			// 尾部是被切断的多字节字符，去掉后重试
			b = b[:len(b)-1]
			continue
		}
		break
	}
	return string(b) + "..."
}
