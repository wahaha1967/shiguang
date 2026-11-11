package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOwnerIDMatchesPython(t *testing.T) {
	if got := OwnerID("test-token"); got != "4c5dc9b7708905f7" {
		t.Fatalf("OwnerID = %q", got)
	}
}

func TestLoadGeneratesAndReusesTheToken(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(EnvToken, "")
	t.Setenv(EnvDB, "")
	first, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Token) != 43 {
		t.Fatalf("token length = %d", len(first.Token))
	}
	stored, err := os.ReadFile(filepath.Join(dir, "token"))
	if err != nil {
		t.Fatal(err)
	}
	if string(stored) != first.Token {
		t.Errorf("stored token %q != %q", stored, first.Token)
	}
	second, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if second.Token != first.Token {
		t.Errorf("a restart generated a new token")
	}
}

func TestEnvironmentTokenWins(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(EnvToken, "  env-token  ")
	cfg, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Token != "env-token" {
		t.Errorf("token = %q", cfg.Token)
	}
	if _, err := os.Stat(filepath.Join(dir, "token")); !os.IsNotExist(err) {
		t.Error("the environment token must not be persisted")
	}
}

func TestDatabasePathOverride(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(EnvToken, "env-token")
	custom := filepath.Join(dir, "custom.db")
	t.Setenv(EnvDB, custom)
	cfg, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DBPath != custom {
		t.Errorf("DBPath = %q", cfg.DBPath)
	}
}
