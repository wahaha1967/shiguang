package secret_test

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"shiguang-study/desktop/internal/secret"
)

type keyFixture struct {
	Samples []struct {
		Secret  string `json:"secret"`
		Plain   string `json:"plaintext"`
		Cipher  string `json:"ciphertext"`
		Preview string `json:"preview"`
	} `json:"samples"`
	Broken []struct {
		Cipher string `json:"ciphertext"`
		Error  string `json:"error"`
	} `json:"broken"`
}

func loadKeys(t *testing.T) keyFixture {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "fixtures", "keys.json"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	var fixture keyFixture
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatalf("decode fixture: %v", err)
	}
	return fixture
}

func TestFixtureCiphertextsDecrypt(t *testing.T) {
	fixture := loadKeys(t)
	for _, sample := range fixture.Samples {
		t.Setenv(secret.SecretEnv, sample.Secret)
		manager := secret.NewManager(t.TempDir())
		plain, err := manager.Decrypt(sample.Cipher)
		if err != nil {
			t.Fatalf("decrypt: %v", err)
		}
		if plain != sample.Plain {
			t.Fatalf("plain = %q, want %q", plain, sample.Plain)
		}
		if got := secret.Preview(plain); got != sample.Preview {
			t.Fatalf("preview = %q, want %q", got, sample.Preview)
		}
	}
}

func TestBrokenCiphertextsAreRejected(t *testing.T) {
	fixture := loadKeys(t)
	for _, broken := range fixture.Broken {
		t.Setenv(secret.SecretEnv, "fixture-secret")
		manager := secret.NewManager(t.TempDir())
		_, err := manager.Decrypt(broken.Cipher)
		if err == nil || err.Error() != broken.Error {
			t.Fatalf("cipher %q: error = %v, want %q", broken.Cipher, err, broken.Error)
		}
	}
}

func TestParseEnv(t *testing.T) {
	values := secret.ParseEnv("# comment\nexport SHIGUANG_OTHER=1\nQUOTED=\"two\"\nSINGLE='three'\nEMPTY=\nNOEQUALS\n")
	if values["SHIGUANG_OTHER"] != "1" || values["QUOTED"] != "two" ||
		values["SINGLE"] != "three" || values["EMPTY"] != "" {
		t.Fatalf("values = %v", values)
	}
	if _, ok := values["NOEQUALS"]; ok {
		t.Fatal("line without '=' must be ignored")
	}
}

func TestTheEnvironmentWinsOverTheEnvFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	if err := os.WriteFile(path, []byte(secret.SecretEnv+"=from-file\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(secret.EnvFileEnv, path)
	t.Setenv(secret.SecretEnv, "from-environment")
	manager := secret.NewManager(dir)
	manager.LoadEnvFile()
	value, err := manager.Secret()
	if err != nil {
		t.Fatal(err)
	}
	if value != "from-environment" {
		t.Fatalf("secret = %q", value)
	}
}

func TestAMissingSecretIsGeneratedAndPersisted(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	t.Setenv(secret.EnvFileEnv, path)
	t.Setenv(secret.SecretEnv, "")
	manager := secret.NewManager(dir)
	generated, err := manager.Secret()
	if err != nil {
		t.Fatal(err)
	}
	if generated == "" {
		t.Fatal("empty secret")
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), secret.SecretEnv+"="+generated) {
		t.Fatalf("secret not persisted: %s", content)
	}
	_ = os.Unsetenv(secret.SecretEnv)
	second, err := secret.NewManager(dir).Secret()
	if err != nil {
		t.Fatal(err)
	}
	if second != generated {
		t.Fatalf("generated secret is not stable: %q != %q", second, generated)
	}
}

func TestRoundTripAndFreshNonce(t *testing.T) {
	t.Setenv(secret.SecretEnv, "round-trip-secret")
	manager := secret.NewManager(t.TempDir())
	plain := "sk-TESTKEY0123456789ABCDEF"
	first, err := manager.Encrypt(plain)
	if err != nil {
		t.Fatal(err)
	}
	second, err := manager.Encrypt(plain)
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("nonce was reused")
	}
	if strings.Contains(first, plain) {
		t.Fatal("ciphertext contains the plaintext")
	}
	raw, err := base64.StdEncoding.DecodeString(first)
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) != secret.NonceSize+len(plain)+16 {
		t.Fatalf("unexpected blob length %d", len(raw))
	}
	decrypted, err := manager.Decrypt(first)
	if err != nil {
		t.Fatal(err)
	}
	if decrypted != plain {
		t.Fatalf("decrypted = %q", decrypted)
	}
}

func TestAnotherSecretCannotDecrypt(t *testing.T) {
	t.Setenv(secret.SecretEnv, "first-secret")
	manager := secret.NewManager(t.TempDir())
	ciphertext, err := manager.Encrypt("sk-x")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv(secret.SecretEnv, "another-secret")
	_, err = manager.Decrypt(ciphertext)
	if err == nil || !strings.Contains(err.Error(), "密钥解密失败") {
		t.Fatalf("error = %v", err)
	}
}

func TestPreviewBoundaries(t *testing.T) {
	cases := map[string]string{
		"":                      "****",
		"sk-short":              "****",
		strings.Repeat("a", 12): "****",
		"sk-abcdefghijklmnop":   "sk-a****mnop",
		"中文密钥示例0123456789":      "中文密钥****6789",
	}
	for value, expected := range cases {
		if got := secret.Preview(value); got != expected {
			t.Fatalf("Preview(%q) = %q, want %q", value, got, expected)
		}
	}
}

func TestTheDefaultEnvFileLivesInTheDataDirectory(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(secret.EnvFileEnv, "")
	t.Setenv(secret.SecretEnv, "x")
	manager := secret.NewManager(dir)
	if got, want := manager.EnvFile(), filepath.Join(dir, ".env"); !bytes.Equal([]byte(got), []byte(want)) {
		t.Fatalf("EnvFile = %q, want %q", got, want)
	}
}
