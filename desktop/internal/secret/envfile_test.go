package secret

import (
	"os"
	"path/filepath"
	"testing"
)

// unsetEnv removes a variable for the duration of the test. t.Setenv(name, "")
// would leave LookupEnv reporting "exists", which would mask the trust logic in
// loadLocked.
func unsetEnv(t *testing.T, name string) {
	t.Helper()
	old, had := os.LookupEnv(name)
	if err := os.Unsetenv(name); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if had {
			_ = os.Setenv(name, old)
		} else {
			_ = os.Unsetenv(name)
		}
	})
}

// withExecutable points os.Executable at a fake binary inside dir.
func withExecutable(t *testing.T, dir string) {
	t.Helper()
	original := executable
	executable = func() (string, error) { return filepath.Join(dir, "shiguang-study.exe"), nil }
	t.Cleanup(func() { executable = original })
}

func TestExeAdjacentEnvOnlySuppliesTheSecret(t *testing.T) {
	unsetEnv(t, EnvFileEnv)
	unsetEnv(t, SecretEnv)
	unsetEnv(t, "SHIGUANG_TOKEN")
	unsetEnv(t, "SHIGUANG_DATA_DIR")

	exeDir := t.TempDir()
	env := SecretEnv + "=exe-secret\n" +
		"SHIGUANG_TOKEN=hijack\n" +
		"SHIGUANG_DATA_DIR=" + filepath.Join(t.TempDir(), "elsewhere") + "\n"
	if err := os.WriteFile(filepath.Join(exeDir, ".env"), []byte(env), 0o600); err != nil {
		t.Fatal(err)
	}
	withExecutable(t, exeDir)

	m := NewManager(t.TempDir())
	path, trusted := m.envFile()
	if trusted {
		t.Fatal("an exe-adjacent .env must be untrusted")
	}
	if path != filepath.Join(exeDir, ".env") {
		t.Fatalf("envFile = %q", path)
	}
	m.LoadEnvFile()
	if got := os.Getenv(SecretEnv); got != "exe-secret" {
		t.Fatalf("secret = %q, want exe-secret", got)
	}
	if _, ok := os.LookupEnv("SHIGUANG_TOKEN"); ok {
		t.Fatal("an untrusted .env must not set SHIGUANG_TOKEN")
	}
	if _, ok := os.LookupEnv("SHIGUANG_DATA_DIR"); ok {
		t.Fatal("an untrusted .env must not set SHIGUANG_DATA_DIR")
	}
}

func TestDataDirEnvIsFullyTrusted(t *testing.T) {
	unsetEnv(t, EnvFileEnv)
	unsetEnv(t, SecretEnv)
	unsetEnv(t, "SHIGUANG_TOKEN")

	dataDir := t.TempDir()
	env := SecretEnv + "=data-secret\nSHIGUANG_TOKEN=data-token\n"
	if err := os.WriteFile(filepath.Join(dataDir, ".env"), []byte(env), 0o600); err != nil {
		t.Fatal(err)
	}
	withExecutable(t, t.TempDir()) // no .env next to the executable

	m := NewManager(dataDir)
	path, trusted := m.envFile()
	if !trusted {
		t.Fatal("a data-dir .env must be trusted")
	}
	if path != filepath.Join(dataDir, ".env") {
		t.Fatalf("envFile = %q", path)
	}
	m.LoadEnvFile()
	if got := os.Getenv(SecretEnv); got != "data-secret" {
		t.Fatalf("secret = %q", got)
	}
	if got := os.Getenv("SHIGUANG_TOKEN"); got != "data-token" {
		t.Fatalf("token = %q", got)
	}
}

func TestExplicitEnvFileIsFullyTrusted(t *testing.T) {
	unsetEnv(t, SecretEnv)
	unsetEnv(t, "SHIGUANG_TOKEN")

	path := filepath.Join(t.TempDir(), "custom.env")
	env := SecretEnv + "=explicit-secret\nSHIGUANG_TOKEN=explicit-token\n"
	if err := os.WriteFile(path, []byte(env), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(EnvFileEnv, path)
	withExecutable(t, t.TempDir())

	m := NewManager(t.TempDir())
	resolved, trusted := m.envFile()
	if !trusted {
		t.Fatal("an explicit SHIGUANG_ENV_FILE must be trusted")
	}
	if resolved != path {
		t.Fatalf("envFile = %q, want %q", resolved, path)
	}
	m.LoadEnvFile()
	if got := os.Getenv(SecretEnv); got != "explicit-secret" {
		t.Fatalf("secret = %q", got)
	}
	if got := os.Getenv("SHIGUANG_TOKEN"); got != "explicit-token" {
		t.Fatalf("token = %q", got)
	}
}
