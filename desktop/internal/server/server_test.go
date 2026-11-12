package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"shiguang-study/desktop/internal/config"
	"shiguang-study/desktop/internal/llm"
	"shiguang-study/desktop/internal/ops"
	"shiguang-study/desktop/internal/secret"
	"shiguang-study/desktop/internal/store"
)

const (
	testToken = "test-token"
	qwenBase  = "https://dashscope.aliyuncs.com/compatible-mode/v1"
)

type fixture struct {
	t       *testing.T
	server  *Server
	http    *httptest.Server
	dataDir string
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	dataDir := t.TempDir()
	t.Setenv(config.EnvToken, testToken)
	t.Setenv(config.EnvDataDir, dataDir)
	t.Setenv(secret.EnvFileEnv, filepath.Join(dataDir, ".env"))
	t.Setenv(secret.SecretEnv, "http-test-secret")

	st, err := store.Open(filepath.Join(dataDir, "study.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	cfg := &config.Config{
		DataDir: dataDir,
		DBPath:  filepath.Join(dataDir, "study.db"),
		Token:   testToken,
		OwnerID: config.OwnerID(testToken),
	}
	srv, err := New(cfg, st, secret.NewManager(dataDir), false)
	if err != nil {
		t.Fatalf("new server: %v", err)
	}
	httpServer := httptest.NewServer(srv.Handler())
	t.Cleanup(httpServer.Close)
	return &fixture{t: t, server: srv, http: httpServer, dataDir: dataDir}
}

func (f *fixture) request(method, path string, body any, headers map[string]string) (*http.Response, []byte) {
	f.t.Helper()
	var reader *bytes.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			f.t.Fatalf("encode body: %v", err)
		}
		reader = bytes.NewReader(encoded)
	} else {
		reader = bytes.NewReader(nil)
	}
	request, err := http.NewRequest(method, f.http.URL+path, reader)
	if err != nil {
		f.t.Fatalf("new request: %v", err)
	}
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	for name, value := range headers {
		request.Header.Set(name, value)
	}
	response, err := f.http.Client().Do(request)
	if err != nil {
		f.t.Fatalf("do request: %v", err)
	}
	defer response.Body.Close()
	data := new(bytes.Buffer)
	if _, err := data.ReadFrom(response.Body); err != nil {
		f.t.Fatalf("read response: %v", err)
	}
	return response, data.Bytes()
}

func (f *fixture) auth(path string, body any) (*http.Response, map[string]any) {
	f.t.Helper()
	return f.authorized("POST", path, body)
}

func (f *fixture) authorized(method, path string, body any) (*http.Response, map[string]any) {
	f.t.Helper()
	response, data := f.request(method, path, body, map[string]string{
		"Authorization": "Bearer " + testToken,
	})
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var payload map[string]any
	if len(data) > 0 {
		if err := decoder.Decode(&payload); err != nil {
			f.t.Fatalf("decode %s %s: %v (%s)", method, path, err, data)
		}
	}
	return response, payload
}

func (f *fixture) state() map[string]any {
	f.t.Helper()
	response, payload := f.authorized("GET", "/api/state", nil)
	if response.StatusCode != http.StatusOK {
		f.t.Fatalf("state status %d: %v", response.StatusCode, payload)
	}
	return payload
}

func (f *fixture) storeKey(provider, key string) {
	f.t.Helper()
	response, payload := f.auth("/api/ai-key", map[string]any{"provider": provider, "apiKey": key})
	if response.StatusCode != http.StatusOK {
		f.t.Fatalf("save key status %d: %v", response.StatusCode, payload)
	}
}

func question() map[string]any {
	return map[string]any{
		"id": "q-1", "subject": "数学", "title": "1+1", "answer": "2",
		"reason": "", "tag": "", "cause": "概念不清", "difficulty": "基础",
		"image": "", "favorite": false, "stage": 0,
		"due": "2026-11-10", "created": "2026-11-10", "demo": false,
	}
}

func detail(t *testing.T, payload map[string]any) string {
	t.Helper()
	value, _ := payload["detail"].(string)
	return value
}

func TestHealthNeedsNoToken(t *testing.T) {
	f := newFixture(t)
	response, data := f.request("GET", "/api/health", nil, nil)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status %d", response.StatusCode)
	}
	if !bytes.Contains(data, []byte(`"app":"shiguang-study"`)) {
		t.Errorf("health payload %s", data)
	}
}

func TestStateRequiresAToken(t *testing.T) {
	f := newFixture(t)
	response, data := f.request("GET", "/api/state", nil, nil)
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("missing token status %d", response.StatusCode)
	}
	if !bytes.Contains(data, []byte("需要访问令牌")) {
		t.Errorf("detail %s", data)
	}
	response, data = f.request("GET", "/api/state", nil,
		map[string]string{"Authorization": "Bearer wrong"})
	if response.StatusCode != http.StatusForbidden {
		t.Fatalf("wrong token status %d", response.StatusCode)
	}
	if !bytes.Contains(data, []byte("访问令牌无效")) {
		t.Errorf("detail %s", data)
	}
	response, _ = f.request("GET", "/api/state", nil,
		map[string]string{"Authorization": "Bearer " + testToken})
	if response.StatusCode != http.StatusOK {
		t.Fatalf("valid token status %d", response.StatusCode)
	}
}

func TestEveryDataRouteIsProtected(t *testing.T) {
	f := newFixture(t)
	for _, route := range []struct{ method, path string }{
		{"GET", "/api/status"},
		{"GET", "/api/state"},
		{"POST", "/api/ops"},
		{"POST", "/api/import"},
		{"POST", "/api/ai-key"},
		{"DELETE", "/api/ai-key/qwen"},
		{"POST", "/api/chat"},
	} {
		response, _ := f.request(route.method, route.path, nil, nil)
		if response.StatusCode != http.StatusUnauthorized {
			t.Errorf("%s %s: status %d, want 401", route.method, route.path, response.StatusCode)
		}
	}
}

func TestOpsRoundTrip(t *testing.T) {
	f := newFixture(t)
	response, payload := f.auth("/api/ops", map[string]any{
		"ops": []any{map[string]any{"t": "questions", "op": "upsert", "ord": 0, "row": question()}},
	})
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status %d: %v", response.StatusCode, payload)
	}
	if payload["ok"] != true || payload["applied"] != json.Number("1") {
		t.Errorf("payload %v", payload)
	}
	state := f.state()
	questions := state["questions"].([]any)
	if len(questions) != 1 || questions[0].(map[string]any)["title"] != "1+1" {
		t.Errorf("state %v", questions)
	}
}

func TestTimerAcceptsEpochMilliseconds(t *testing.T) {
	f := newFixture(t)
	end := time.Now().UnixMilli() + 60_000
	response, payload := f.auth("/api/ops", map[string]any{
		"ops": []any{map[string]any{"t": "timer", "op": "replace",
			"row": map[string]any{"running": true, "remaining": 60, "duration": 60, "end": end}}},
	})
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status %d: %v", response.StatusCode, payload)
	}
	timer := f.state()["timer"].(map[string]any)
	if timer["end"] != json.Number(strconv.FormatInt(end, 10)) {
		t.Errorf("end %v, want %d", timer["end"], end)
	}
}

func TestImportReplacesTheOwnerState(t *testing.T) {
	f := newFixture(t)
	state := map[string]any{
		"schema": 1, "questions": []any{question()}, "trash": []any{}, "logs": []any{},
		"rewards": []any{}, "redemptions": []any{},
		"prefs": map[string]any{"name": "同学", "goal": 5, "minutes": 25, "theme": "light"},
		"timer": nil, "aiSets": []any{},
	}
	response, payload := f.auth("/api/import", map[string]any{"state": state})
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status %d: %v", response.StatusCode, payload)
	}
	if len(f.state()["questions"].([]any)) != 1 {
		t.Error("import did not persist the question")
	}
	response, payload = f.auth("/api/import", map[string]any{"state": map[string]any{"questions": []any{}}})
	if response.StatusCode != http.StatusBadRequest {
		t.Errorf("incomplete backup status %d", response.StatusCode)
	}
	if detail(t, payload) != "备份格式不正确" {
		t.Errorf("detail %q", detail(t, payload))
	}
}

func TestNewOwnerIsNotInitialized(t *testing.T) {
	f := newFixture(t)
	if f.state()["initialized"] != false {
		t.Error("a new owner must not be initialized")
	}
}

func TestAIConfigRoundTrip(t *testing.T) {
	f := newFixture(t)
	aiConfig := map[string]any{
		"provider": "deepseek",
		"base":     qwenBase,
		"vision":   "qwen3-vl-plus", "text": "qwen-plus",
		"deepseekBase":   "https://api.deepseek.com",
		"deepseekVision": "deepseek-flash",
		"deepseekText":   "deepseek-flash",
	}
	response, payload := f.auth("/api/ops", map[string]any{
		"ops": []any{map[string]any{"t": "aiConfig", "op": "replace", "row": aiConfig}},
	})
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status %d: %v", response.StatusCode, payload)
	}
	state := f.state()
	stored := state["aiConfig"].(map[string]any)
	if stored["provider"] != "deepseek" || stored["deepseekText"] != "deepseek-flash" {
		t.Errorf("aiConfig %v", stored)
	}
	if state["initialized"] != true {
		t.Error("a saved AI config marks the owner initialized")
	}
}

func TestSavingAKeyStoresItEncrypted(t *testing.T) {
	f := newFixture(t)
	plain := "sk-SECRET0123456789"
	f.storeKey("qwen", plain)
	ciphertext, err := ops.ReadKey(f.server.store.DB(), f.server.config.OwnerID, "qwen")
	if err != nil {
		t.Fatalf("read key: %v", err)
	}
	if ciphertext == "" || strings.Contains(ciphertext, plain) {
		t.Fatalf("stored value is not ciphertext: %q", ciphertext)
	}
	decrypted, err := f.server.secrets.Decrypt(ciphertext)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if decrypted != plain {
		t.Errorf("decrypted %q", decrypted)
	}
	response, data := f.request("GET", "/api/status", nil,
		map[string]string{"Authorization": "Bearer " + testToken})
	if response.StatusCode != http.StatusOK || bytes.Contains(data, []byte(plain)) {
		t.Errorf("status response leaked key material: %s", data)
	}
}

func TestTheStatusReturnsAMaskedPreview(t *testing.T) {
	f := newFixture(t)
	f.storeKey("qwen", "sk-SECRET0123456789")
	_, payload := f.authorized("GET", "/api/status", nil)
	providers := payload["providers"].(map[string]any)
	qwen := providers["qwen"].(map[string]any)
	if qwen["hasKey"] != true || qwen["preview"] != "sk-S****6789" {
		t.Errorf("qwen status %v", qwen)
	}
	if providers["deepseek"].(map[string]any)["hasKey"] != false {
		t.Error("deepseek must not report a key")
	}
	if payload["hasKey"] != true {
		t.Error("hasKey must be true when any provider is configured")
	}
	ciphertext, err := ops.ReadKey(f.server.store.DB(), f.server.config.OwnerID, "qwen")
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(encoded, []byte("SECRET0123456789")) ||
		(ciphertext != "" && bytes.Contains(encoded, []byte(ciphertext))) {
		t.Error("the status leaked key material")
	}
}

func TestAShortKeyIsMaskedCompletely(t *testing.T) {
	f := newFixture(t)
	f.storeKey("qwen", "sk-short")
	_, payload := f.authorized("GET", "/api/status", nil)
	preview := payload["providers"].(map[string]any)["qwen"].(map[string]any)["preview"]
	if preview != "****" {
		t.Errorf("preview %q", preview)
	}
}

func TestSavingAKeyReturnsItsMaskedPreview(t *testing.T) {
	f := newFixture(t)
	response, payload := f.auth("/api/ai-key", map[string]any{
		"provider": "deepseek", "apiKey": "sk-OTHER0123456789",
	})
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status %d", response.StatusCode)
	}
	if payload["preview"] != "sk-O****6789" {
		t.Errorf("preview %v", payload["preview"])
	}
}

func TestBadKeyRequestsAreRejected(t *testing.T) {
	f := newFixture(t)
	cases := []map[string]any{
		{"provider": "openai", "apiKey": "sk-x"},
		{"provider": "qwen", "apiKey": "  "},
		{"provider": "qwen", "apiKey": strings.Repeat("a", 501)},
		{"provider": "qwen", "apiKey": "line\nbreak"},
	}
	for _, body := range cases {
		response, _ := f.auth("/api/ai-key", body)
		if response.StatusCode != http.StatusBadRequest {
			t.Errorf("%v: status %d", body, response.StatusCode)
		}
	}
}

func TestClearingAKeyRemovesIt(t *testing.T) {
	f := newFixture(t)
	f.storeKey("qwen", "sk-STORED0123456789")
	response, payload := f.authorized("DELETE", "/api/ai-key/qwen", nil)
	if response.StatusCode != http.StatusOK || payload["hasKey"] != false {
		t.Fatalf("delete status %d: %v", response.StatusCode, payload)
	}
	_, status := f.authorized("GET", "/api/status", nil)
	if status["providers"].(map[string]any)["qwen"].(map[string]any)["hasKey"] != false {
		t.Error("the key was not removed")
	}
	response, _ = f.authorized("DELETE", "/api/ai-key/openai", nil)
	if response.StatusCode != http.StatusBadRequest {
		t.Errorf("unknown provider delete status %d", response.StatusCode)
	}
}

func TestChatRejectsAnEndpointOutsideTheAllowlist(t *testing.T) {
	f := newFixture(t)
	response, payload := f.auth("/api/chat", map[string]any{
		"provider": "qwen", "endpoint": "https://api.openai.com/v1",
		"request": map[string]any{"model": "x", "messages": []any{}},
	})
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("status %d", response.StatusCode)
	}
	if detail(t, payload) != llm.ErrEndpointKnown.Error() {
		t.Errorf("detail %q", detail(t, payload))
	}
	response, payload = f.auth("/api/chat", map[string]any{
		"provider": "deepseek", "endpoint": qwenBase,
		"request": map[string]any{"model": "x", "messages": []any{}},
	})
	if response.StatusCode != http.StatusBadRequest || detail(t, payload) != "服务商与接口地址不一致" {
		t.Errorf("mismatch status %d: %v", response.StatusCode, payload)
	}
}

func TestChatRequiresAStoredKey(t *testing.T) {
	f := newFixture(t)
	response, payload := f.auth("/api/chat", map[string]any{
		"provider": "qwen", "endpoint": qwenBase,
		"request": map[string]any{"model": "qwen-plus", "messages": []any{}},
	})
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("status %d", response.StatusCode)
	}
	if !strings.Contains(detail(t, payload), "API Key") {
		t.Errorf("detail %q", detail(t, payload))
	}
}

func TestTheBrowserCannotInjectAKeyPerRequest(t *testing.T) {
	f := newFixture(t)
	var called bool
	original := chatUpstream
	t.Cleanup(func() { chatUpstream = original })
	chatUpstream = func(string, string, any) (int, any, error) {
		called = true
		return 200, map[string]any{}, nil
	}
	response, _ := f.auth("/api/chat", map[string]any{
		"provider": "qwen", "endpoint": qwenBase, "apiKey": "sk-injected",
		"request": map[string]any{"model": "qwen-plus", "messages": []any{}},
	})
	if response.StatusCode != http.StatusBadRequest || called {
		t.Errorf("injected key status %d, upstream called %v", response.StatusCode, called)
	}
}

func TestChatForwardsTheStoredKey(t *testing.T) {
	f := newFixture(t)
	plain := "sk-stored-key"
	f.storeKey("qwen", plain)
	var forwarded string
	original := chatUpstream
	t.Cleanup(func() { chatUpstream = original })
	chatUpstream = func(endpoint, key string, body any) (int, any, error) {
		if endpoint != qwenBase {
			t.Errorf("endpoint %q", endpoint)
		}
		forwarded = key
		return 200, map[string]any{"choices": []any{map[string]any{
			"message": map[string]any{"content": "{}"}}}}, nil
	}
	response, payload := f.auth("/api/chat", map[string]any{
		"provider": "qwen", "endpoint": qwenBase, "apiKey": "sk-injected",
		"request": map[string]any{"model": "qwen-plus", "messages": []any{}},
	})
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status %d: %v", response.StatusCode, payload)
	}
	if forwarded != plain {
		t.Errorf("forwarded key %q, want the stored key", forwarded)
	}
	if _, ok := payload["choices"]; !ok {
		t.Errorf("upstream payload not returned: %v", payload)
	}
}

func TestChatPassesUpstreamFailuresThrough(t *testing.T) {
	f := newFixture(t)
	f.storeKey("qwen", "sk-stored-key")
	original := chatUpstream
	t.Cleanup(func() { chatUpstream = original })
	chatUpstream = func(string, string, any) (int, any, error) {
		return 502, map[string]any{"error": map[string]any{"message": "模型响应过大，请减少题量"}}, nil
	}
	response, payload := f.auth("/api/chat", map[string]any{
		"provider": "qwen", "endpoint": qwenBase,
		"request": map[string]any{"model": "qwen-plus", "messages": []any{}},
	})
	if response.StatusCode != 502 {
		t.Fatalf("status %d: %v", response.StatusCode, payload)
	}
	if _, ok := payload["error"]; !ok {
		t.Errorf("upstream error payload lost: %v", payload)
	}
}

func TestIndexInjectsTheApiToken(t *testing.T) {
	f := newFixture(t)
	response, data := f.request("GET", "/", nil, nil)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status %d", response.StatusCode)
	}
	if !bytes.Contains(data, []byte(`<meta name="api-token" content="test-token">`)) {
		t.Error("the token meta tag is missing")
	}
	response, _ = f.request("GET", "/index.html", nil, nil)
	if response.StatusCode != http.StatusOK {
		t.Errorf("index.html status %d", response.StatusCode)
	}
	response, _ = f.request("GET", "/missing.js", nil, nil)
	if response.StatusCode != http.StatusNotFound {
		t.Errorf("unknown path status %d", response.StatusCode)
	}
}

func TestMalformedBodiesAreRejected(t *testing.T) {
	f := newFixture(t)
	for _, endpoint := range []string{"/api/ops", "/api/import", "/api/ai-key", "/api/chat"} {
		request, _ := http.NewRequest("POST", f.http.URL+endpoint,
			strings.NewReader(`{"bad"`))
		request.Header.Set("Authorization", "Bearer "+testToken)
		response, err := f.http.Client().Do(request)
		if err != nil {
			t.Fatalf("%s: %v", endpoint, err)
		}
		data := new(bytes.Buffer)
		_, _ = data.ReadFrom(response.Body)
		response.Body.Close()
		if response.StatusCode != http.StatusBadRequest {
			t.Errorf("%s: status %d", endpoint, response.StatusCode)
		}
		if !bytes.Contains(data.Bytes(), []byte("请求格式不正确")) {
			t.Errorf("%s: body %s", endpoint, data.String())
		}
	}
	response, payload := f.auth("/api/ops", map[string]any{"ops": "not-an-array"})
	if response.StatusCode != http.StatusBadRequest || detail(t, payload) != "ops 必须是数组" {
		t.Errorf("ops status %d: %v", response.StatusCode, payload)
	}
	response, payload = f.auth("/api/import", map[string]any{"state": []any{}})
	if response.StatusCode != http.StatusBadRequest || detail(t, payload) != "备份格式不正确" {
		t.Errorf("import status %d: %v", response.StatusCode, payload)
	}
}

func TestNullBodyIsRejected(t *testing.T) {
	f := newFixture(t)
	for _, endpoint := range []string{"/api/ops", "/api/import", "/api/ai-key", "/api/chat"} {
		request, err := http.NewRequest("POST", f.http.URL+endpoint, strings.NewReader("null"))
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("Authorization", "Bearer "+testToken)
		response, err := f.http.Client().Do(request)
		if err != nil {
			t.Fatalf("%s: %v", endpoint, err)
		}
		data := new(bytes.Buffer)
		_, _ = data.ReadFrom(response.Body)
		response.Body.Close()
		if response.StatusCode != http.StatusBadRequest {
			t.Errorf("%s: status %d", endpoint, response.StatusCode)
		}
		if !bytes.Contains(data.Bytes(), []byte("请求格式不正确")) {
			t.Errorf("%s: body %s", endpoint, data.String())
		}
	}
}

func TestFalsyOpsMeanAnEmptyBatch(t *testing.T) {
	f := newFixture(t)
	for _, body := range []string{`{"ops": false}`, `{"ops": 0}`, `{"ops": ""}`, `{"ops": {}}`} {
		request, err := http.NewRequest("POST", f.http.URL+"/api/ops", strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("Authorization", "Bearer "+testToken)
		response, err := f.http.Client().Do(request)
		if err != nil {
			t.Fatal(err)
		}
		data := new(bytes.Buffer)
		_, _ = data.ReadFrom(response.Body)
		response.Body.Close()
		var payload map[string]any
		_ = json.Unmarshal(data.Bytes(), &payload)
		if response.StatusCode != http.StatusOK || payload["applied"] != float64(0) {
			t.Errorf("%s: status %d, body %s", body, response.StatusCode, data.String())
		}
	}
}

func TestInvalidOperationLeavesStateUntouched(t *testing.T) {
	f := newFixture(t)
	response, payload := f.auth("/api/ops", map[string]any{
		"ops": []any{
			map[string]any{"t": "questions", "op": "upsert", "row": question()},
			map[string]any{"t": "unknown", "op": "upsert"},
		},
	})
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("status %d: %v", response.StatusCode, payload)
	}
	if detail(t, payload) == "" {
		t.Error("the failure must explain itself")
	}
	if len(f.state()["questions"].([]any)) != 0 {
		t.Error("a failed batch must not persist earlier operations")
	}
}

func TestNonLoopbackHostIsRejected(t *testing.T) {
	f := newFixture(t)
	request, err := http.NewRequest("GET", f.http.URL+"/", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Host = "rebind.example.com"
	response, err := f.http.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	data := new(bytes.Buffer)
	_, _ = data.ReadFrom(response.Body)
	if response.StatusCode != http.StatusForbidden {
		t.Fatalf("status %d", response.StatusCode)
	}
	if bytes.Contains(data.Bytes(), []byte(testToken)) {
		t.Fatal("a rejected request must not leak the token")
	}
	if !bytes.Contains(data.Bytes(), []byte("只接受本机访问")) {
		t.Fatalf("body %s", data.String())
	}
}

func TestLoopbackHostnamesAreAccepted(t *testing.T) {
	f := newFixture(t)
	port := f.http.URL[strings.LastIndex(f.http.URL, ":")+1:]
	for _, host := range []string{"127.0.0.1:" + port, "localhost:" + port, "[::1]:" + port} {
		request, err := http.NewRequest("GET", f.http.URL+"/api/health", nil)
		if err != nil {
			t.Fatal(err)
		}
		request.Host = host
		response, err := f.http.Client().Do(request)
		if err != nil {
			t.Fatalf("%s: %v", host, err)
		}
		response.Body.Close()
		if response.StatusCode != http.StatusOK {
			t.Errorf("%s: status %d", host, response.StatusCode)
		}
	}
}

func TestIndexPageCarriesETagAndNoCache(t *testing.T) {
	f := newFixture(t)
	response, data := f.request("GET", "/", nil, nil)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status %d", response.StatusCode)
	}
	if !bytes.Contains(data, []byte(testToken)) {
		t.Fatal("the served page must carry the token")
	}
	if got := response.Header.Get("Cache-Control"); got != "no-cache" {
		t.Fatalf("Cache-Control = %q", got)
	}
	etag := response.Header.Get("ETag")
	if etag == "" {
		t.Fatal("missing ETag")
	}
	cond, body := f.request("GET", "/", nil, map[string]string{"If-None-Match": etag})
	if cond.StatusCode != http.StatusNotModified {
		t.Fatalf("conditional status %d", cond.StatusCode)
	}
	if len(body) != 0 {
		t.Fatalf("304 must have no body, got %d bytes", len(body))
	}
	stale, staleBody := f.request("GET", "/", nil, map[string]string{"If-None-Match": `"deadbeef"`})
	if stale.StatusCode != http.StatusOK || len(staleBody) == 0 {
		t.Fatalf("stale status %d, %d bytes", stale.StatusCode, len(staleBody))
	}
}

func TestJSONResponsesAreNotStored(t *testing.T) {
	f := newFixture(t)
	response, _ := f.authorized("GET", "/api/state", nil)
	if got := response.Header.Get("Cache-Control"); got != "no-store" {
		t.Fatalf("Cache-Control = %q", got)
	}
}

func TestOversizedBodyIsRejected(t *testing.T) {
	f := newFixture(t)
	original := maxRequestBody
	maxRequestBody = 16
	t.Cleanup(func() { maxRequestBody = original })
	body := `{"ops":[{"t":"questions","op":"upsert","row":{"id":"` + strings.Repeat("a", 64) + `"}}]}`
	request, err := http.NewRequest("POST", f.http.URL+"/api/ops", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+testToken)
	response, err := f.http.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	data := new(bytes.Buffer)
	_, _ = data.ReadFrom(response.Body)
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("status %d", response.StatusCode)
	}
	if !bytes.Contains(data.Bytes(), []byte("请求格式不正确")) {
		t.Fatalf("body %s", data.String())
	}
	if len(f.state()["questions"].([]any)) != 0 {
		t.Error("an oversized body must not persist anything")
	}
}
