package proxy

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/tidwall/gjson"
)

const defaultProxyReadTimeout = 120 * time.Second

// StartProxy 启动本地 Anthropic 兼容代理，转发到 OpenAI 兼容上游。
func StartProxy(targetBaseURL, authToken string) (int, func(context.Context) error, error) {
	baseURL := strings.TrimSpace(strings.TrimSuffix(targetBaseURL, "/"))
	if baseURL == "" {
		return 0, nil, fmt.Errorf("上游 Base URL 不能为空")
	}

	client := &http.Client{}
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/messages", func(w http.ResponseWriter, r *http.Request) {
		handleMessages(w, r, client, baseURL, authToken)
	})
	mux.HandleFunc("/v1/messages/count_tokens", handleCountTokens)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, nil, fmt.Errorf("启动本地代理监听失败: %w", err)
	}

	server := &http.Server{
		Handler:     mux,
		ReadTimeout: defaultProxyReadTimeout,
		// 不设置 WriteTimeout：流式响应可能持续很长时间
	}

	go func() {
		_ = server.Serve(ln)
	}()

	port := ln.Addr().(*net.TCPAddr).Port
	shutdown := func(ctx context.Context) error {
		return server.Shutdown(ctx)
	}
	return port, shutdown, nil
}

func handleMessages(w http.ResponseWriter, r *http.Request, client *http.Client, baseURL, authToken string) {
	if r.Method != http.MethodPost {
		writeAnthropicError(w, http.StatusMethodNotAllowed, "只支持 POST /v1/messages")
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeAnthropicError(w, http.StatusBadRequest, "读取请求体失败")
		return
	}
	defer r.Body.Close()

	if !json.Valid(body) {
		writeAnthropicError(w, http.StatusBadRequest, "请求 JSON 无效")
		return
	}

	stream := gjson.GetBytes(body, "stream").Bool()
	openAIReqBody := ConvertClaudeRequestToResponses(body, stream)

	upstreamURL := baseURL + "/responses"
	upReq, err := http.NewRequestWithContext(r.Context(), http.MethodPost, upstreamURL, bytes.NewReader(openAIReqBody))
	if err != nil {
		writeAnthropicError(w, http.StatusBadGateway, fmt.Sprintf("构造上游请求失败: %v", err))
		return
	}

	upReq.Header.Set("Content-Type", "application/json")
	if authToken != "" {
		upReq.Header.Set("X-Api-Key", authToken)
		upReq.Header.Set("Authorization", "Bearer "+authToken)
	}

	upResp, err := client.Do(upReq)
	if err != nil {
		writeAnthropicError(w, http.StatusBadGateway, fmt.Sprintf("请求上游失败: %v", err))
		return
	}
	defer upResp.Body.Close()

	if upResp.StatusCode >= 400 {
		upErrBody, _ := io.ReadAll(upResp.Body)
		msg := extractUpstreamErrorMessage(upErrBody)
		if msg == "" {
			msg = fmt.Sprintf("上游返回错误状态 %d", upResp.StatusCode)
		}
		writeAnthropicError(w, mapUpstreamStatus(upResp.StatusCode), msg)
		return
	}

	if stream {
		handleStreamingResponse(w, upResp)
	} else {
		handleNonStreamingResponse(w, upResp)
	}
}

func handleNonStreamingResponse(w http.ResponseWriter, upResp *http.Response) {
	respBody, err := io.ReadAll(upResp.Body)
	if err != nil {
		writeAnthropicError(w, http.StatusBadGateway, "读取上游响应失败")
		return
	}

	anthropicJSON := ConvertResponsesToClaudeNonStream(respBody)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(w, anthropicJSON)
}

func handleStreamingResponse(w http.ResponseWriter, upResp *http.Response) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeAnthropicError(w, http.StatusInternalServerError, "当前响应器不支持流式输出")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	converter := NewResponsesStreamConverter()
	scanner := bufio.NewScanner(upResp.Body)
	// 增大缓冲区以处理长 SSE 行（如 base64 图片等）
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	var currentEvent string

	for scanner.Scan() {
		line := scanner.Text()

		// SSE 事件以 "event: xxx" 开头
		if strings.HasPrefix(line, "event: ") {
			currentEvent = strings.TrimPrefix(line, "event: ")
			continue
		}

		// SSE 数据以 "data: " 开头
		if strings.HasPrefix(line, "data: ") {
			dataJSON := strings.TrimPrefix(line, "data: ")

			eventType := currentEvent
			if eventType == "" {
				// 没有 event 行，尝试从 data 中推断
				if t := gjson.Get(dataJSON, "type"); t.Exists() {
					eventType = t.String()
				}
			}

			if eventType != "" {
				events := converter.ProcessEvent(eventType, dataJSON)
				for _, ev := range events {
					_, _ = io.WriteString(w, ev)
				}
				if len(events) > 0 {
					flusher.Flush()
				}
			}

			currentEvent = ""
			continue
		}

		// 空行是 SSE 事件分隔符，重置当前事件
		if line == "" {
			currentEvent = ""
		}
	}

	// 上游断开后，确保所有块已关闭
	if !converter.IsDone() {
		events := converter.Finish()
		for _, ev := range events {
			_, _ = io.WriteString(w, ev)
		}
		if len(events) > 0 {
			flusher.Flush()
		}
	}
}

func handleCountTokens(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeAnthropicError(w, http.StatusMethodNotAllowed, "只支持 POST /v1/messages/count_tokens")
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeAnthropicError(w, http.StatusBadRequest, "读取请求体失败")
		return
	}
	defer r.Body.Close()

	count := estimateInputTokens(body)
	if count < 1 {
		count = 1
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(w, ClaudeTokenCount(r.Context(), int64(count)))
}

func estimateInputTokens(body []byte) int {
	if len(body) == 0 {
		return 1
	}

	textLen := 0
	root := gjson.ParseBytes(body)

	if sys := root.Get("system"); sys.Exists() {
		if sys.Type == gjson.String {
			textLen += len(sys.String())
		} else if sys.IsArray() {
			sys.ForEach(func(_, item gjson.Result) bool {
				if t := item.Get("text"); t.Exists() {
					textLen += len(t.String())
				}
				return true
			})
		}
	}

	if msgs := root.Get("messages"); msgs.Exists() && msgs.IsArray() {
		msgs.ForEach(func(_, msg gjson.Result) bool {
			content := msg.Get("content")
			if content.Type == gjson.String {
				textLen += len(content.String())
				return true
			}
			if content.IsArray() {
				content.ForEach(func(_, part gjson.Result) bool {
					if t := part.Get("text"); t.Exists() {
						textLen += len(t.String())
					}
					if thinking := part.Get("thinking"); thinking.Exists() && thinking.Type == gjson.String {
						textLen += len(thinking.String())
					}
					return true
				})
			}
			return true
		})
	}

	if textLen <= 0 {
		textLen = len(body)
	}

	estimated := textLen / 4
	if estimated < 1 {
		estimated = 1
	}
	return estimated
}

func writeAnthropicError(w http.ResponseWriter, status int, message string) {
	if status <= 0 {
		status = http.StatusInternalServerError
	}
	payload := map[string]any{
		"type": "error",
		"error": map[string]any{
			"type":    "api_error",
			"message": message,
		},
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func extractUpstreamErrorMessage(body []byte) string {
	if len(body) == 0 {
		return ""
	}
	root := gjson.ParseBytes(body)
	if msg := root.Get("error.message"); msg.Exists() {
		return msg.String()
	}
	if msg := root.Get("message"); msg.Exists() {
		return msg.String()
	}
	return strings.TrimSpace(string(body))
}

func mapUpstreamStatus(status int) int {
	switch status {
	case http.StatusUnauthorized, http.StatusForbidden:
		return http.StatusUnauthorized
	case http.StatusTooManyRequests:
		return http.StatusTooManyRequests
	case http.StatusBadRequest:
		return http.StatusBadRequest
	default:
		if status >= 500 {
			return http.StatusBadGateway
		}
		if status < 100 {
			return http.StatusBadGateway
		}
		return status
	}
}

// MaskToken 遮蔽 token，仅保留前 8 个字符。
func MaskToken(token string) string {
	token = strings.TrimSpace(token)
	if token == "" {
		return ""
	}
	if len(token) <= 8 {
		return token + "***"
	}
	return token[:8] + "***"
}

// ProxyURL 返回给定端口的 localhost URL。
func ProxyURL(port int) string {
	return "http://127.0.0.1:" + strconv.Itoa(port)
}
