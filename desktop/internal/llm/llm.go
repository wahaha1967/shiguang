// Package llm implements the endpoint allowlist and the key-redacting proxy
// used by /api/chat.
package llm

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	MaxRequest  = 25 * 1024 * 1024
	MaxResponse = 3 * 1024 * 1024
	Timeout     = 100 * time.Second
)

// newHTTPClient is a test seam; production always disables redirects.
var newHTTPClient = func() *http.Client {
	return &http.Client{
		Timeout: Timeout,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

var (
	ErrEndpointEmpty = errors.New("接口地址不能为空")
	ErrEndpointHTTPS = errors.New("仅接受服务商官方 HTTPS 接口基础地址")
	ErrEndpointKnown = errors.New("仅接受百炼或 DeepSeek 官方接口基础地址")
	ErrBadKey        = errors.New("服务商密钥无效，请重新保存 API Key")
	ErrBadRequest    = errors.New("请求缺少模型或消息")
)

// EndpointProvider returns "qwen" or "deepseek" for an official base URL.
func EndpointProvider(value any) (string, error) {
	text, ok := value.(string)
	if !ok {
		return "", ErrEndpointEmpty
	}
	parsed, err := url.Parse(text)
	if err != nil {
		return "", ErrEndpointHTTPS
	}
	host := strings.ToLower(parsed.Hostname())
	port := parsed.Port()
	if parsed.Scheme != "https" || parsed.User != nil || parsed.RawQuery != "" ||
		parsed.Fragment != "" || (port != "" && port != "443") {
		return "", ErrEndpointHTTPS
	}
	path := strings.TrimRight(parsed.EscapedPath(), "/")
	if (host == "aliyuncs.com" || strings.HasSuffix(host, ".aliyuncs.com")) &&
		strings.HasSuffix(path, "/compatible-mode/v1") {
		return "qwen", nil
	}
	if host == "api.deepseek.com" && (path == "" || path == "/v1") {
		return "deepseek", nil
	}
	return "", ErrEndpointKnown
}

func EndpointURL(value string) (string, error) {
	if _, err := EndpointProvider(value); err != nil {
		return "", err
	}
	return strings.TrimRight(value, "/") + "/chat/completions", nil
}

// Upstream forwards one chat completion. A non-nil error is a caller problem
// (bad key/body); upstream and network failures come back as an HTTP status.
func Upstream(endpoint, key string, body any) (int, any, error) {
	target, err := EndpointURL(endpoint)
	if err != nil {
		return 0, nil, err
	}
	if !validKey(key) {
		return 0, nil, ErrBadKey
	}
	bodyMap, ok := body.(map[string]any)
	if !ok {
		return 0, nil, ErrBadRequest
	}
	if _, ok := bodyMap["messages"].([]any); !ok {
		return 0, nil, ErrBadRequest
	}
	if _, ok := bodyMap["model"].(string); !ok {
		return 0, nil, ErrBadRequest
	}
	provider, _ := EndpointProvider(endpoint)
	payload := map[string]any{}
	for _, name := range []string{"model", "messages", "response_format", "max_tokens", "stream"} {
		if value, ok := bodyMap[name]; ok {
			payload[name] = value
		}
	}
	if provider == "deepseek" {
		payload["thinking"] = map[string]any{"type": "disabled"}
	} else {
		payload["enable_thinking"] = false
	}
	payload["stream"] = false

	encoded, err := json.Marshal(payload)
	if err != nil {
		return 0, nil, err
	}
	if len(encoded) > MaxRequest {
		return 502, errorPayload("请求内容过大，请减少题量"), nil
	}
	request, err := http.NewRequest(http.MethodPost, target, bytes.NewReader(encoded))
	if err != nil {
		return 502, errorPayload("无法连接所选服务商，请检查网络和接口设置后重试"), nil
	}
	request.Header.Set("Authorization", "Bearer "+strings.TrimSpace(key))
	request.Header.Set("Content-Type", "application/json")

	response, err := newHTTPClient().Do(request)
	if err != nil {
		return 502, errorPayload("无法连接所选服务商，请检查网络和接口设置后重试"), nil
	}
	defer response.Body.Close()

	if response.StatusCode >= 200 && response.StatusCode < 300 {
		data, err := io.ReadAll(io.LimitReader(response.Body, MaxResponse+1))
		if err != nil {
			return 502, errorPayload("无法连接所选服务商，请检查网络和接口设置后重试"), nil
		}
		if len(data) > MaxResponse {
			return 502, errorPayload("模型响应过大，请减少题量"), nil
		}
		var result any
		decoder := json.NewDecoder(bytes.NewReader(data))
		decoder.UseNumber()
		if err := decoder.Decode(&result); err != nil {
			return 502, errorPayload("无法连接所选服务商，请检查网络和接口设置后重试"), nil
		}
		return 200, result, nil
	}

	data, _ := io.ReadAll(io.LimitReader(response.Body, MaxResponse))
	message := upstreamMessage(data)
	message = strings.ReplaceAll(message, key, "[密钥已隐藏]")
	message = truncateRunes(message, 500)
	status := response.StatusCode
	if status < 400 || status >= 600 {
		status = 502
	}
	return status, errorPayload(message), nil
}

func validKey(key string) bool {
	trimmed := strings.TrimSpace(key)
	return trimmed != "" && utf8.RuneCountInString(key) <= 500 &&
		!strings.ContainsAny(key, "\r\n")
}

func upstreamMessage(data []byte) string {
	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		return "上游请求失败"
	}
	if errorObject, ok := parsed["error"].(map[string]any); ok {
		if message, ok := errorObject["message"].(string); ok && message != "" {
			return message
		}
	}
	if message, ok := parsed["message"].(string); ok && message != "" {
		return message
	}
	return "上游请求失败"
}

func errorPayload(message string) map[string]any {
	return map[string]any{"error": map[string]any{"message": message}}
}

func truncateRunes(text string, limit int) string {
	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}
	return string(runes[:limit])
}
