// Package services 里放暴露给前端的 Go 服务。
package services

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// AIService 代理对第三方 AI 端点的请求。
//
// 为什么不让前端直接 fetch：
//
//	WebView 里执行的是浏览器语义，跨域请求要先过 CORS 预检。实测
//	opencode.ai/zen/go/v1 的 OPTIONS 直接返回 404、不带任何 Access-Control
//	头，所以前端的请求**根本发不出去**（OpenRouter 带 allow-origin: *，
//	所以在它上面能跑，问题被掩盖了）。
//
//	放到 Go 侧就没有 CORS 这回事，顺带还能加浏览器加不了的头 ——
//	opencode 要求 x-opencode-session，缺了直接 400 MissingSessionID。
type AIService struct {
	client *http.Client
	// sessionID 在一次进程生命周期内固定。opencode 用它做路由，
	// 每次请求换一个也能用，但固定下来更贴近「一个客户端会话」的语义。
	sessionID string
}

func NewAIService() *AIService {
	return &AIService{
		client:    &http.Client{Timeout: 180 * time.Second},
		sessionID: newSessionID(),
	}
}

func newSessionID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "svg-studio-session"
	}
	return hex.EncodeToString(b)
}

// userAgent 自报身份。opencode 的文档明确要求客户端用自己的 UA
// （形如 my-coding-agent/1.0），而不是伪装成浏览器或通用 HTTP 客户端 ——
// 他们的流量是按客户端识别的。
const userAgent = "svg-studio/desktop"

// ErrorInfo 是给前端用的结构化错误。
//
// 单独拆出来是因为前端需要区分「网络问题」「鉴权失败」「限流」「端点本身
// 报错」—— 用一段拼接字符串传过去，前端只能整段显示，没法给出针对性提示。
type ErrorInfo struct {
	Status  int    `json:"status"`
	Kind    string `json:"kind"`
	Message string `json:"message"`
}

// ChatCompletion 把一次 /chat/completions 请求代理出去，返回原始响应体。
//
// baseURL 由前端传入（各家服务商的端点或用户自定义），apiKey 走 Authorization
// 头。返回原始 body 而不是解析后的内容，是为了不在 Go 侧重复实现一遍各家
// 响应结构的差异 —— 前端本来就在解析 choices[0].message.content。
func (s *AIService) ChatCompletion(baseURL, apiKey string, bodyJSON string) (string, *ErrorInfo) {
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if base == "" {
		return "", &ErrorInfo{Kind: "config", Message: "接口地址为空，请在设置里填写 Base URL。"}
	}
	if strings.TrimSpace(apiKey) == "" {
		return "", &ErrorInfo{Kind: "config", Message: "API Key 为空，请在设置里填写。"}
	}

	endpoint := base + "/chat/completions"
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, endpoint,
		bytes.NewReader([]byte(bodyJSON)))
	if err != nil {
		return "", &ErrorInfo{Kind: "config", Message: "接口地址无效：" + err.Error()}
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("User-Agent", userAgent)
	// opencode 需要这个才能路由；别的服务商会忽略未知头。
	req.Header.Set("x-opencode-session", s.sessionID)

	slog.Debug("proxy request", "endpoint", endpoint, "bytes", len(bodyJSON))

	resp, err := s.client.Do(req)
	if err != nil {
		return "", &ErrorInfo{Kind: "network", Message: "请求失败：" + err.Error()}
	}
	defer resp.Body.Close()

	raw, readErr := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if readErr != nil {
		return "", &ErrorInfo{Status: resp.StatusCode, Kind: "network", Message: "读取响应失败：" + readErr.Error()}
	}

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return string(raw), nil
	}

	return "", &ErrorInfo{
		Status:  resp.StatusCode,
		Kind:    classify(resp.StatusCode, raw),
		Message: extractServerMessage(raw, resp.Status),
	}
}

// classify 把 HTTP 状态码翻译成前端能分辨的类别。
func classify(status int, raw []byte) string {
	switch {
	case status == http.StatusUnauthorized || status == http.StatusForbidden:
		return "auth"
	case status == http.StatusTooManyRequests:
		return "ratelimit"
	case status >= 500:
		return "server"
	case status == http.StatusBadRequest:
		// opencode 用 400 + 自定义 type 表达「缺 session 头」这类协议错误，
		// 归到 request 里让前端原样展示即可。
		return "request"
	default:
		return "request"
	}
}

// extractServerMessage 尽量从响应体里挖出一句人能看懂的说明。
//
// 各家格式不一样：OpenAI 系是 {"error":{"message":...}}，opencode 是
// {"type":"error","error":{"type":"...","message":...}}，还有的直接返回纯文本。
// 按顺序试，最后兜底用 HTTP 状态行。
func extractServerMessage(raw []byte, statusLine string) string {
	var payload map[string]any
	if json.Unmarshal(raw, &payload) == nil {
		if e, ok := payload["error"].(map[string]any); ok {
			if m, ok := e["message"].(string); ok && m != "" {
				if t, ok := e["type"].(string); ok && t != "" {
					return fmt.Sprintf("%s（%s）", m, t)
				}
				return m
			}
		}
		if m, ok := payload["message"].(string); ok && m != "" {
			return m
		}
	}
	text := strings.TrimSpace(string(raw))
	if text != "" && len(text) <= 400 {
		return text
	}
	return statusLine
}
