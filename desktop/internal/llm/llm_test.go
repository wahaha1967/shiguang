package llm

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
)

const secret = "sk-TESTKEY0123456789ABCDEF"

var body = map[string]any{
	"model":           "qwen-plus",
	"messages":        []any{map[string]any{"role": "user", "content": "hi"}},
	"response_format": map[string]any{"type": "json_object"},
	"max_tokens":      json.Number("8192"),
}

type endpointsFixture struct {
	Endpoints []struct {
		Value    any    `json:"value"`
		Provider string `json:"provider"`
		Error    string `json:"error"`
	} `json:"endpoints"`
	Upstream []struct {
		Name         string          `json:"name"`
		Endpoint     string          `json:"endpoint"`
		Body         json.RawMessage `json:"body"`
		Status       int             `json:"status"`
		Sent         json.RawMessage `json:"sent"`
		Result       json.RawMessage `json:"result"`
		ErrorMessage string          `json:"error_message"`
	} `json:"upstream"`
}

func loadFixture(t *testing.T) endpointsFixture {
	t.Helper()
	path := filepath.Join("..", "..", "fixtures", "llm.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	var fixture endpointsFixture
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := decoder.Decode(&fixture); err != nil {
		t.Fatalf("decode fixture: %v", err)
	}
	return fixture
}

func decodeRaw(t *testing.T, raw json.RawMessage) any {
	t.Helper()
	if len(raw) == 0 {
		return nil
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		t.Fatalf("decode raw fixture: %v", err)
	}
	return value
}

func TestEndpointAllowlistMatchesPython(t *testing.T) {
	for _, testCase := range loadFixture(t).Endpoints {
		provider, err := EndpointProvider(testCase.Value)
		if testCase.Error == "" {
			if err != nil {
				t.Errorf("%v: unexpected error %v", testCase.Value, err)
			} else if provider != testCase.Provider {
				t.Errorf("%v: provider %q, want %q", testCase.Value, provider, testCase.Provider)
			}
			continue
		}
		if err == nil {
			t.Errorf("%v: want error %q, got provider %q", testCase.Value, testCase.Error, provider)
		} else if err.Error() != testCase.Error {
			t.Errorf("%v: error %q, want %q", testCase.Value, err, testCase.Error)
		}
	}
}

func TestEndpointURLAppendsChatCompletions(t *testing.T) {
	cases := map[string]string{
		"https://api.deepseek.com":                           "https://api.deepseek.com/chat/completions",
		"https://api.deepseek.com/":                          "https://api.deepseek.com/chat/completions",
		"https://dashscope.aliyuncs.com/compatible-mode/v1":  "https://dashscope.aliyuncs.com/compatible-mode/v1/chat/completions",
		"https://dashscope.aliyuncs.com/compatible-mode/v1/": "https://dashscope.aliyuncs.com/compatible-mode/v1/chat/completions",
		"https://biz-123.aliyuncs.com/compatible-mode/v1":    "https://biz-123.aliyuncs.com/compatible-mode/v1/chat/completions",
	}
	for base, want := range cases {
		got, err := EndpointURL(base)
		if err != nil {
			t.Fatalf("%s: %v", base, err)
		}
		if got != want {
			t.Errorf("%s: url %q, want %q", base, got, want)
		}
	}
	if _, err := EndpointURL("https://api.openai.com/v1"); err == nil {
		t.Error("unknown endpoint must be rejected")
	}
}

type fakeTransport struct {
	mu       sync.Mutex
	requests []*http.Request
	bodies   [][]byte
	respond  func(*http.Request, []byte) (*http.Response, error)
}

func (f *fakeTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	body, _ := io.ReadAll(request.Body)
	f.mu.Lock()
	f.requests = append(f.requests, request)
	f.bodies = append(f.bodies, body)
	f.mu.Unlock()
	return f.respond(request, body)
}

func (f *fakeTransport) sent(t *testing.T, index int) map[string]any {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	if index >= len(f.bodies) {
		t.Fatalf("request %d not recorded", index)
	}
	decoder := json.NewDecoder(bytes.NewReader(f.bodies[index]))
	decoder.UseNumber()
	var parsed map[string]any
	if err := decoder.Decode(&parsed); err != nil {
		t.Fatalf("decode sent body: %v", err)
	}
	return parsed
}

func (f *fakeTransport) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.requests)
}

// withTransport swaps only the transport of the production client, keeping the
// real timeout and no-redirect policy under test.
func withTransport(t *testing.T, transport http.RoundTripper) {
	t.Helper()
	original := newHTTPClient
	t.Cleanup(func() { newHTTPClient = original })
	newHTTPClient = func() *http.Client {
		client := original()
		client.Transport = transport
		return client
	}
}

func jsonResponse(status int, payload []byte) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(bytes.NewReader(payload)),
	}
}

func TestUpstreamMatchesPythonFixtures(t *testing.T) {
	for _, testCase := range loadFixture(t).Upstream {
		t.Run(testCase.Name, func(t *testing.T) {
			var payload []byte
			switch testCase.Name {
			case "oversized":
				payload = bytes.Repeat([]byte("x"), MaxResponse+2)
			case "upstream-401":
				payload = []byte(`{"error":{"message":"invalid api key ` + secret + `"}}`)
			default:
				payload = []byte(`{"choices":[{"message":{"content":"{}"}}]}`)
			}
			transport := &fakeTransport{respond: func(_ *http.Request, _ []byte) (*http.Response, error) {
				if testCase.Name == "network" {
					return nil, errors.New("connection refused")
				}
				return jsonResponse(200, payload), nil
			}}
			if testCase.Name == "upstream-401" {
				transport.respond = func(_ *http.Request, _ []byte) (*http.Response, error) {
					return jsonResponse(401, payload), nil
				}
			}
			withTransport(t, transport)

			var requestBody map[string]any
			decoder := json.NewDecoder(bytes.NewReader(testCase.Body))
			decoder.UseNumber()
			if err := decoder.Decode(&requestBody); err != nil {
				t.Fatalf("decode request fixture: %v", err)
			}

			status, result, err := Upstream(testCase.Endpoint, secret, requestBody)
			if err != nil {
				t.Fatalf("unexpected caller error: %v", err)
			}
			if status != testCase.Status {
				t.Fatalf("status %d, want %d (result %v)", status, testCase.Status, result)
			}
			if len(testCase.Sent) > 0 {
				want := decodeRaw(t, testCase.Sent)
				got := transport.sent(t, 0)
				if !reflect.DeepEqual(got, want) {
					t.Errorf("sent body\n got: %#v\nwant: %#v", got, want)
				}
				if auth := transport.requests[0].Header.Get("Authorization"); auth != "Bearer "+secret {
					t.Errorf("Authorization %q", auth)
				}
			}
			if len(testCase.Result) > 0 {
				want := decodeRaw(t, testCase.Result)
				if !reflect.DeepEqual(result, want) {
					t.Errorf("result\n got: %#v\nwant: %#v", result, want)
				}
			}
			if testCase.ErrorMessage != "" {
				payload, ok := result.(map[string]any)
				if !ok {
					t.Fatalf("error payload missing: %#v", result)
				}
				message := payload["error"].(map[string]any)["message"]
				if message != testCase.ErrorMessage {
					t.Errorf("message %q, want %q", message, testCase.ErrorMessage)
				}
				if strings.Contains(message.(string), secret) {
					t.Error("the key leaked into the error message")
				}
			}
		})
	}
}

func TestOnlyWhitelistedFieldsAreForwarded(t *testing.T) {
	transport := &fakeTransport{respond: func(_ *http.Request, _ []byte) (*http.Response, error) {
		return jsonResponse(200, []byte(`{}`)), nil
	}}
	withTransport(t, transport)
	status, _, err := Upstream("https://dashscope.aliyuncs.com/compatible-mode/v1", secret,
		map[string]any{
			"model":       "qwen-plus",
			"messages":    []any{},
			"temperature": 0.7,
			"top_p":       1,
			"stream":      true,
		})
	if status != 200 || err != nil {
		t.Fatalf("status %d, err %v", status, err)
	}
	sent := transport.sent(t, 0)
	for _, key := range []string{"temperature", "top_p"} {
		if _, ok := sent[key]; ok {
			t.Errorf("%s must not be forwarded", key)
		}
	}
	if sent["stream"] != false {
		t.Errorf("stream must be forced to false, got %v", sent["stream"])
	}
	if sent["enable_thinking"] != false {
		t.Errorf("qwen must disable thinking, got %v", sent["enable_thinking"])
	}
}

func TestRedirectsAreNotFollowed(t *testing.T) {
	transport := &fakeTransport{respond: func(_ *http.Request, _ []byte) (*http.Response, error) {
		response := jsonResponse(302, []byte(`{"error":{"message":"redirect"}}`))
		response.Header.Set("Location", "https://api.deepseek.com/chat/completions")
		return response, nil
	}}
	withTransport(t, transport)
	status, result, err := Upstream("https://api.deepseek.com", secret, body)
	if err != nil {
		t.Fatalf("unexpected caller error: %v", err)
	}
	if status != 502 {
		t.Errorf("redirect status %d must be reported as 502", status)
	}
	if transport.count() != 1 {
		t.Errorf("redirect was followed: %d requests", transport.count())
	}
	message := result.(map[string]any)["error"].(map[string]any)["message"]
	if message != "redirect" {
		t.Errorf("message %q", message)
	}
}

func TestRejectsBadKeys(t *testing.T) {
	for _, key := range []string{"", "   ", strings.Repeat("a", 501), "key\nwith-newline"} {
		if _, _, err := Upstream("https://api.deepseek.com", key, body); !errors.Is(err, ErrBadKey) {
			t.Errorf("key %q: err %v, want ErrBadKey", key, err)
		}
	}
}

func TestRejectsRequestWithoutModelOrMessages(t *testing.T) {
	cases := []any{
		"not-an-object",
		map[string]any{"messages": []any{}},
		map[string]any{"model": "qwen-plus"},
		map[string]any{"model": "qwen-plus", "messages": "text"},
	}
	for _, request := range cases {
		if _, _, err := Upstream("https://api.deepseek.com", secret, request); !errors.Is(err, ErrBadRequest) {
			t.Errorf("%#v: err %v, want ErrBadRequest", request, err)
		}
	}
}

func TestUnparseableUpstreamErrorUsesGenericMessage(t *testing.T) {
	transport := &fakeTransport{respond: func(_ *http.Request, _ []byte) (*http.Response, error) {
		return jsonResponse(500, []byte("not json")), nil
	}}
	withTransport(t, transport)
	status, result, err := Upstream("https://api.deepseek.com", secret, body)
	if err != nil || status != 500 {
		t.Fatalf("status %d, err %v", status, err)
	}
	message := result.(map[string]any)["error"].(map[string]any)["message"]
	if message != "上游请求失败" {
		t.Errorf("message %q", message)
	}
}

func TestUpstreamMessageIsTruncatedTo500Runes(t *testing.T) {
	transport := &fakeTransport{respond: func(_ *http.Request, _ []byte) (*http.Response, error) {
		payload, _ := json.Marshal(map[string]any{"error": map[string]any{"message": strings.Repeat("错", 600)}})
		return jsonResponse(400, payload), nil
	}}
	withTransport(t, transport)
	status, result, err := Upstream("https://api.deepseek.com", secret, body)
	if err != nil || status != 400 {
		t.Fatalf("status %d, err %v", status, err)
	}
	message := result.(map[string]any)["error"].(map[string]any)["message"].(string)
	if len([]rune(message)) != 500 {
		t.Errorf("message has %d runes, want 500", len([]rune(message)))
	}
}

func TestInvalidSuccessBodyIsBadGateway(t *testing.T) {
	transport := &fakeTransport{respond: func(_ *http.Request, _ []byte) (*http.Response, error) {
		return jsonResponse(200, []byte("not json")), nil
	}}
	withTransport(t, transport)
	status, result, err := Upstream("https://api.deepseek.com", secret, body)
	if err != nil || status != 502 {
		t.Fatalf("status %d, err %v", status, err)
	}
	message := result.(map[string]any)["error"].(map[string]any)["message"]
	if message != "无法连接所选服务商，请检查网络和接口设置后重试" {
		t.Errorf("message %q", message)
	}
}

func TestOversizedRequestIsRejectedBeforeSending(t *testing.T) {
	transport := &fakeTransport{respond: func(_ *http.Request, _ []byte) (*http.Response, error) {
		return jsonResponse(200, []byte(`{}`)), nil
	}}
	withTransport(t, transport)
	status, result, err := Upstream("https://api.deepseek.com", secret, map[string]any{
		"model":    "deepseek-flash",
		"messages": []any{map[string]any{"role": "user", "content": strings.Repeat("x", MaxRequest)}},
	})
	if err != nil || status != 502 {
		t.Fatalf("status %d, err %v", status, err)
	}
	if transport.count() != 0 {
		t.Error("an oversized request must not reach the provider")
	}
	message := result.(map[string]any)["error"].(map[string]any)["message"]
	if message != "请求内容过大，请减少题量" {
		t.Errorf("message %q", message)
	}
}
