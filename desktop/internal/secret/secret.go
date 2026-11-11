// Package secret loads the .env file and encrypts provider keys with
// AES-256-GCM in exactly the format written by the Python backend.
package secret

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"unicode/utf8"
)

const (
	SecretEnv  = "SHIGUANG_SECRET"
	EnvFileEnv = "SHIGUANG_ENV_FILE"
	NonceSize  = 12
)

var (
	ErrCorrupt = errors.New("密钥数据损坏，无法解密")
	ErrDecrypt = errors.New("密钥解密失败，请检查 .env 中的加密串是否被改动")
)

type Manager struct {
	DataDir string
	mu      sync.Mutex
}

func NewManager(dataDir string) *Manager {
	return &Manager{DataDir: dataDir}
}

// ParseEnv parses KEY=VALUE lines the way the Python backend does.
func ParseEnv(text string) map[string]string {
	values := map[string]string{}
	for _, raw := range strings.Split(text, "\n") {
		line := strings.TrimSpace(strings.TrimSuffix(raw, "\r"))
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		line = strings.TrimSpace(line)
		name, value, found := strings.Cut(line, "=")
		if !found {
			continue
		}
		name = strings.TrimSpace(name)
		value = strings.TrimSpace(value)
		if len(value) >= 2 && value[0] == value[len(value)-1] &&
			(value[0] == '"' || value[0] == '\'') {
			value = value[1 : len(value)-1]
		}
		if name != "" {
			values[name] = value
		}
	}
	return values
}

// EnvFile resolves the .env path: explicit override, data directory, then the
// directory holding the executable (read-only).
func (m *Manager) EnvFile() string {
	path, _ := m.envFile()
	return path
}

// executable is a test seam for os.Executable.
var executable = os.Executable

// envFile resolves the .env path and reports whether it may set arbitrary
// configuration. A file next to the executable is untrusted: whoever can write
// there must not be able to redirect the data directory or token, so it may
// only supply SHIGUANG_SECRET. The data directory and an explicit
// SHIGUANG_ENV_FILE are trusted.
func (m *Manager) envFile() (string, bool) {
	if value := strings.TrimSpace(os.Getenv(EnvFileEnv)); value != "" {
		return expand(value), true
	}
	dataFile := filepath.Join(m.DataDir, ".env")
	if fileExists(dataFile) {
		return dataFile, true
	}
	if exe, err := executable(); err == nil {
		exeFile := filepath.Join(filepath.Dir(exe), ".env")
		if fileExists(exeFile) {
			return exeFile, false
		}
	}
	return dataFile, true
}

// LoadEnvFile loads the file into the process environment. Real environment
// variables win over values from the file.
func (m *Manager) LoadEnvFile() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.loadLocked()
}

// Secret returns SHIGUANG_SECRET, generating and persisting one on first use.
func (m *Manager) Secret() (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.loadLocked()
	if value := strings.TrimSpace(os.Getenv(SecretEnv)); value != "" {
		return value, nil
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	value := base64.RawURLEncoding.EncodeToString(raw)
	if err := m.persist(value); err != nil {
		return "", err
	}
	_ = os.Setenv(SecretEnv, value)
	return value, nil
}

func (m *Manager) loadLocked() {
	path, trusted := m.envFile()
	text, err := os.ReadFile(path)
	if err != nil {
		return
	}
	for name, value := range ParseEnv(string(text)) {
		if !trusted && name != SecretEnv {
			continue
		}
		if _, exists := os.LookupEnv(name); !exists {
			_ = os.Setenv(name, value)
		}
	}
}

func (m *Manager) persist(value string) error {
	target := m.EnvFile()
	explicit := strings.TrimSpace(os.Getenv(EnvFileEnv)) != ""
	if !explicit && !sameDir(filepath.Dir(target), m.DataDir) {
		target = filepath.Join(m.DataDir, ".env")
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		return err
	}
	handle, err := os.OpenFile(target, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer handle.Close()
	_, err = fmt.Fprintf(handle,
		"\n# %s：用于加密数据库中的 API Key，请勿泄露或提交\n%s=%s\n",
		SecretEnv, SecretEnv, value)
	if err != nil {
		return err
	}
	_ = os.Chmod(target, 0o600)
	return nil
}

func (m *Manager) Encrypt(plaintext string) (string, error) {
	secret, err := m.Secret()
	if err != nil {
		return "", err
	}
	gcm, err := newGCM(secret)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, NonceSize)
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	sealed := gcm.Seal(nil, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(append(nonce, sealed...)), nil
}

func (m *Manager) Decrypt(ciphertext string) (string, error) {
	secret, err := m.Secret()
	if err != nil {
		return "", err
	}
	if strings.ContainsAny(ciphertext, "\r\n") {
		return "", ErrCorrupt
	}
	raw, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return "", ErrCorrupt
	}
	if len(raw) <= NonceSize {
		return "", ErrCorrupt
	}
	gcm, err := newGCM(secret)
	if err != nil {
		return "", err
	}
	plain, err := gcm.Open(nil, raw[:NonceSize], raw[NonceSize:], nil)
	if err != nil {
		return "", ErrDecrypt
	}
	if !utf8.Valid(plain) {
		return "", ErrDecrypt
	}
	return string(plain), nil
}

// Preview masks a key: first and last four characters, or **** when short.
func Preview(plaintext string) string {
	text := strings.TrimSpace(plaintext)
	runes := []rune(text)
	if len(runes) <= 12 {
		return "****"
	}
	return string(runes[:4]) + "****" + string(runes[len(runes)-4:])
}

func newGCM(secret string) (cipher.AEAD, error) {
	key := sha256.Sum256([]byte(secret))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func sameDir(a, b string) bool {
	absA, errA := filepath.Abs(a)
	absB, errB := filepath.Abs(b)
	if errA != nil || errB != nil {
		return filepath.Clean(a) == filepath.Clean(b)
	}
	return strings.EqualFold(filepath.Clean(absA), filepath.Clean(absB))
}

func expand(path string) string {
	if path == "~" || strings.HasPrefix(path, "~/") || strings.HasPrefix(path, `~\`) {
		if home, err := os.UserHomeDir(); err == nil {
			if path == "~" {
				return home
			}
			return filepath.Join(home, path[2:])
		}
	}
	return path
}
