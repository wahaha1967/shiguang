// Package server exposes the loopback HTTP API consumed by the embedded page.
package server

import (
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"

	"shiguang-study/desktop/internal/config"
	"shiguang-study/desktop/internal/llm"
	"shiguang-study/desktop/internal/ops"
	"shiguang-study/desktop/internal/secret"
	"shiguang-study/desktop/internal/store"
	"shiguang-study/desktop/web"
)

const TokenMeta = `<meta name="api-token" content="%s">`

var providers = []string{"qwen", "deepseek"}

// maxRequestBody bounds one request body. A full backup with many compressed
// images stays far below it; the limit only stops a runaway request. It is a
// variable so that the tests can exercise the rejection path.
var maxRequestBody int64 = 256 * 1024 * 1024

// chatUpstream is a test seam.
var chatUpstream = llm.Upstream

type Server struct {
	config  *config.Config
	store   *store.Store
	secrets *secret.Manager
	page    []byte
	etag    string
	dev     bool
}

func New(cfg *config.Config, st *store.Store, secrets *secret.Manager, dev bool) (*Server, error) {
	index, err := web.IndexHTML(dev)
	if err != nil {
		return nil, err
	}
	s := &Server{config: cfg, store: st, secrets: secrets, dev: dev}
	if !dev {
		// The embedded page never changes, so render and hash it once instead
		// of on every navigation.
		s.page = injectToken(index, cfg.Token)
		s.etag = pageETag(s.page)
	}
	return s, nil
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", s.health)
	mux.HandleFunc("GET /api/status", s.authorized(s.status))
	mux.HandleFunc("GET /api/state", s.authorized(s.state))
	mux.HandleFunc("POST /api/ops", s.authorized(s.applyOps))
	mux.HandleFunc("POST /api/import", s.authorized(s.importState))
	mux.HandleFunc("POST /api/ai-key", s.authorized(s.saveKey))
	mux.HandleFunc("DELETE /api/ai-key/{provider}", s.authorized(s.deleteKey))
	mux.HandleFunc("POST /api/chat", s.authorized(s.chat))
	mux.HandleFunc("/", s.indexPage)
	return loopbackOnly(mux)
}

// loopbackOnly drops requests that do not address the loopback interface. The
// page carries the API token, so a website that rebinds its own domain to
// 127.0.0.1 must not be able to read it.
func loopbackOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host := r.Host
		if trimmed, _, err := net.SplitHostPort(host); err == nil {
			host = trimmed
		}
		switch host {
		case "127.0.0.1", "localhost", "::1", "[::1]":
			next.ServeHTTP(w, r)
		default:
			writeError(w, http.StatusForbidden, "只接受本机访问")
		}
	})
}

func (s *Server) authorized(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		header := r.Header.Get("Authorization")
		if !strings.HasPrefix(header, "Bearer ") {
			writeError(w, http.StatusUnauthorized, "需要访问令牌，请在页面的 AI 设置中填写")
			return
		}
		supplied := strings.TrimSpace(strings.TrimPrefix(header, "Bearer "))
		if subtle.ConstantTimeCompare([]byte(supplied), []byte(s.config.Token)) != 1 {
			writeError(w, http.StatusForbidden, "访问令牌无效")
			return
		}
		next(w, r)
	}
}

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"app":      "shiguang-study",
		"version":  3,
		"database": s.store.Exists(),
	})
}

func (s *Server) status(w http.ResponseWriter, _ *http.Request) {
	stored, err := ops.ReadKeys(s.store.DB(), s.config.OwnerID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "读取密钥状态失败")
		return
	}
	result := map[string]any{}
	hasKey := false
	for _, name := range providers {
		ciphertext := stored[name]
		preview := ""
		if ciphertext != "" {
			if plain, err := s.secrets.Decrypt(ciphertext); err == nil {
				preview = secret.Preview(plain)
			}
		}
		providerHasKey := ciphertext != ""
		hasKey = hasKey || providerHasKey
		result[name] = map[string]any{"hasKey": providerHasKey, "preview": preview}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"app":       "shiguang-study",
		"version":   3,
		"hasKey":    hasKey,
		"providers": result,
	})
}

func (s *Server) state(w http.ResponseWriter, _ *http.Request) {
	state, err := ops.ReadState(s.store.DB(), s.config.OwnerID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "读取学习数据失败")
		return
	}
	writeJSON(w, http.StatusOK, state)
}

func (s *Server) applyOps(w http.ResponseWriter, r *http.Request) {
	payload, err := decodeObject(w, r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "请求格式不正确")
		return
	}
	raw, ok := payload["ops"]
	if !ok {
		raw = []any{}
	}
	list, isList := raw.([]any)
	if !isList {
		if !falsy(raw) {
			writeError(w, http.StatusBadRequest, "ops 必须是数组")
			return
		}
		list = []any{}
	}
	applied := 0
	err = s.store.WithTx(func(tx *sql.Tx) error {
		count, err := ops.ApplyOps(tx, s.config.OwnerID, list)
		applied = count
		return err
	})
	if err != nil {
		writeOperationError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "applied": applied})
}

func (s *Server) importState(w http.ResponseWriter, r *http.Request) {
	payload, err := decodeObject(w, r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "请求格式不正确")
		return
	}
	state, ok := payload["state"].(map[string]any)
	if !ok {
		writeError(w, http.StatusBadRequest, "备份格式不正确")
		return
	}
	if _, ok := state["questions"].([]any); !ok {
		writeError(w, http.StatusBadRequest, "备份格式不正确")
		return
	}
	err = s.store.WithTx(func(tx *sql.Tx) error {
		return ops.ReplaceState(tx, s.config.OwnerID, state)
	})
	if err != nil {
		writeOperationError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) saveKey(w http.ResponseWriter, r *http.Request) {
	payload, err := decodeObject(w, r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "请求格式不正确")
		return
	}
	provider, _ := payload["provider"].(string)
	if provider != "qwen" && provider != "deepseek" {
		writeError(w, http.StatusBadRequest, "服务商只能是 qwen 或 deepseek")
		return
	}
	apiKey, ok := payload["apiKey"].(string)
	trimmed := strings.TrimSpace(apiKey)
	if !ok || trimmed == "" || len([]rune(apiKey)) > 500 ||
		strings.ContainsAny(apiKey, "\r\n") {
		writeError(w, http.StatusBadRequest, "请填写有效的 API Key")
		return
	}
	ciphertext, err := s.secrets.Encrypt(trimmed)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "密钥加密失败，请检查 .env 配置")
		return
	}
	if err := s.store.WithTx(func(tx *sql.Tx) error {
		return ops.SetKey(tx, s.config.OwnerID, provider, ciphertext)
	}); err != nil {
		writeError(w, http.StatusInternalServerError, "保存密钥失败")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true, "provider": provider, "hasKey": true, "preview": secret.Preview(trimmed),
	})
}

func (s *Server) deleteKey(w http.ResponseWriter, r *http.Request) {
	provider := r.PathValue("provider")
	if provider != "qwen" && provider != "deepseek" {
		writeError(w, http.StatusBadRequest, "服务商只能是 qwen 或 deepseek")
		return
	}
	if err := s.store.WithTx(func(tx *sql.Tx) error {
		_, err := ops.DeleteKey(tx, s.config.OwnerID, provider)
		return err
	}); err != nil {
		writeError(w, http.StatusInternalServerError, "删除密钥失败")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "provider": provider, "hasKey": false})
}

func (s *Server) chat(w http.ResponseWriter, r *http.Request) {
	payload, err := decodeObject(w, r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "请求格式不正确")
		return
	}
	provider, err := llm.EndpointProvider(payload["endpoint"])
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if submitted, ok := payload["provider"]; ok && submitted != any(provider) {
		writeError(w, http.StatusBadRequest, "服务商与接口地址不一致")
		return
	}
	ciphertext, err := ops.ReadKey(s.store.DB(), s.config.OwnerID, provider)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "读取密钥失败")
		return
	}
	if ciphertext == "" {
		writeError(w, http.StatusBadRequest, "尚未保存所选服务商的 API Key，请在 AI 连接设置中保存")
		return
	}
	apiKey, err := s.secrets.Decrypt(ciphertext)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	endpoint, _ := payload["endpoint"].(string)
	status, result, err := chatUpstream(endpoint, apiKey, payload["request"])
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, status, result)
}

func (s *Server) indexPage(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" && r.URL.Path != "/index.html" {
		writeError(w, http.StatusNotFound, "Not Found")
		return
	}
	page, etag := s.page, s.etag
	if s.dev {
		index, err := web.IndexHTML(true)
		if err != nil || index == nil {
			writeError(w, http.StatusNotFound, "未找到应用页面")
			return
		}
		page = injectToken(index, s.config.Token)
		etag = pageETag(page)
	}
	if page == nil {
		writeError(w, http.StatusNotFound, "未找到应用页面")
		return
	}
	w.Header().Set("ETag", etag)
	w.Header().Set("Cache-Control", "no-cache")
	if etagMatches(r.Header.Get("If-None-Match"), etag) {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(page)
}

func injectToken(page []byte, token string) []byte {
	meta := strings.ReplaceAll(TokenMeta, "%s", token)
	return []byte(strings.ReplaceAll(string(page), "</head>", meta+"</head>"))
}

func pageETag(page []byte) string {
	sum := sha256.Sum256(page)
	return `"` + hex.EncodeToString(sum[:]) + `"`
}

func etagMatches(header, etag string) bool {
	for _, candidate := range strings.Split(header, ",") {
		value := strings.TrimSpace(candidate)
		if value == etag || value == "W/"+etag {
			return true
		}
	}
	return false
}

func decodeObject(w http.ResponseWriter, r *http.Request) (map[string]any, error) {
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBody)
	decoder := json.NewDecoder(r.Body)
	decoder.UseNumber()
	var payload map[string]any
	if err := decoder.Decode(&payload); err != nil {
		return nil, err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return nil, errors.New("trailing data")
	}
	if payload == nil {
		return nil, errors.New("body must be a JSON object")
	}
	return payload, nil
}

// falsy mirrors Python's truthiness so that `payload.get('ops') or []` keeps
// accepting `false`, `0`, `""` and `{}` as an empty batch.
func falsy(value any) bool {
	switch value := value.(type) {
	case nil:
		return true
	case bool:
		return !value
	case string:
		return value == ""
	case json.Number:
		parsed, err := value.Float64()
		return err == nil && parsed == 0
	case map[string]any:
		return len(value) == 0
	}
	return false
}

func writeOperationError(w http.ResponseWriter, err error) {
	var invalid *ops.InvalidOperation
	if errors.As(err, &invalid) {
		writeError(w, http.StatusBadRequest, invalid.Message)
		return
	}
	writeError(w, http.StatusInternalServerError, "服务器内部错误")
}

func writeError(w http.ResponseWriter, status int, detail string) {
	writeJSON(w, status, map[string]any{"detail": detail})
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
