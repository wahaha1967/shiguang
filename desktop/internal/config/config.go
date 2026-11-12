// Package config resolves the data directory, database path and access token.
package config

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
)

const (
	EnvToken   = "SHIGUANG_TOKEN"
	EnvDataDir = "SHIGUANG_DATA_DIR"
	EnvDB      = "SHIGUANG_DB"
)

type Config struct {
	DataDir string
	DBPath  string
	Token   string
	OwnerID string
}

// BaseDataDir returns the configured data directory or the desktop default.
func BaseDataDir() (string, error) {
	if value := strings.TrimSpace(os.Getenv(EnvDataDir)); value != "" {
		return ExpandPath(value), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local", "share", "shiguang-study"), nil
}

// Load reads the token and derives the owner key. The caller must load the
// .env file before calling so that SHIGUANG_TOKEN inside it is visible.
func Load(dataDir string) (*Config, error) {
	token, err := loadToken(dataDir)
	if err != nil {
		return nil, err
	}
	dbPath := filepath.Join(dataDir, "study.db")
	if value := strings.TrimSpace(os.Getenv(EnvDB)); value != "" {
		dbPath = ExpandPath(value)
	}
	return &Config{
		DataDir: dataDir,
		DBPath:  dbPath,
		Token:   token,
		OwnerID: OwnerID(token),
	}, nil
}

// OwnerID is the stable per-token data space key: the first 16 hex chars of sha256(token).
func OwnerID(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])[:16]
}

func ExpandPath(path string) string {
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

func loadToken(dataDir string) (string, error) {
	if value := strings.TrimSpace(os.Getenv(EnvToken)); value != "" {
		return value, nil
	}
	path := filepath.Join(dataDir, "token")
	if stored, err := os.ReadFile(path); err == nil {
		if value := strings.TrimSpace(string(stored)); value != "" {
			return value, nil
		}
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	if err := os.MkdirAll(dataDir, 0o700); err == nil {
		if err := os.WriteFile(path, []byte(token), 0o600); err == nil {
			_ = os.Chmod(path, 0o600)
		}
	}
	return token, nil
}
