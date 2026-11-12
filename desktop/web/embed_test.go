package web

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"regexp"
	"strings"
	"testing"
)

// The page is a byte-for-byte copy of src/app/index.html. Any functional edit
// must be made upstream and the hash below updated deliberately; `wails3 task
// sync` performs the copy and the hash update in one step.
const (
	indexSHA256 = "a3b07590fd5b9dd97e26c024af81fc6f911540d347d807a730c430bc081cae70"
	indexSize   = 7532726
)

func diskIndex(t *testing.T) []byte {
	t.Helper()
	data, err := os.ReadFile("index.html")
	if err != nil {
		t.Fatalf("read index.html: %v", err)
	}
	return data
}

func TestTheEmbeddedPageMatchesTheDiskCopy(t *testing.T) {
	embedded, err := IndexHTML(false)
	if err != nil {
		t.Fatalf("embedded index: %v", err)
	}
	disk := diskIndex(t)
	if len(embedded) != indexSize || len(disk) != indexSize {
		t.Fatalf("size embedded %d, disk %d, want %d", len(embedded), len(disk), indexSize)
	}
	if string(embedded) != string(disk) {
		t.Fatal("the embedded page differs from web/index.html")
	}
}

func TestThePageIsHashLocked(t *testing.T) {
	disk := diskIndex(t)
	sum := sha256.Sum256(disk)
	if got := hex.EncodeToString(sum[:]); got != indexSHA256 {
		t.Fatalf("index.html changed: sha256 %s, want %s\n"+
			"editing the page requires an explicit hash update in this test", got, indexSHA256)
	}
}

func TestDevModeReadsThePageFromDisk(t *testing.T) {
	data, err := IndexHTML(true)
	if err != nil {
		t.Fatalf("dev index: %v", err)
	}
	if string(data) != string(diskIndex(t)) {
		t.Error("dev mode must serve the disk copy")
	}
}

func appHTML(t *testing.T) string {
	t.Helper()
	return string(diskIndex(t))
}

func TestSingleInlineApplicationScript(t *testing.T) {
	blocks := regexp.MustCompile(`(?s)<script>(.*?)</script>`).FindAllString(appHTML(t), -1)
	if len(blocks) != 1 {
		t.Fatalf("the app must keep exactly one inline <script> block, found %d", len(blocks))
	}
}

func TestPdfjsResourcesAreEmbedded(t *testing.T) {
	html := appHTML(t)
	for _, id := range []string{"pdfjs-core", "pdfjs-worker", "pdfjs-resources"} {
		needle := `<script id="` + id + `" type="application/octet-stream">`
		if !strings.Contains(html, needle) {
			t.Errorf("missing embedded resource %s", id)
		}
	}
	if !strings.Contains(html, `<script id="PDFJS_LICENSE" type="text/plain">`) {
		t.Error("the PDF.js license is not embedded")
	}
}

func TestAiModuleFollowsTheCoreModule(t *testing.T) {
	html := appHTML(t)
	core := strings.Index(html, "function render(){")
	ai := strings.Index(html, "const aiNavBase=nav")
	if core < 0 || ai < 0 || core >= ai {
		t.Errorf("the AI module must follow the core module (core %d, ai %d)", core, ai)
	}
	if !strings.Contains(html, "NAV.splice(4,0,['capture'") {
		t.Error("the AI pages are not registered in the navigation")
	}
}

func TestNoInlineEventHandlersOutsideTheAllowlist(t *testing.T) {
	html := appHTML(t)
	allowed := []string{
		`onclick="window.print()"`,
		`$('#topfocus').onclick=`,
		`$('#mobilemenu').onclick=`,
		`$('#overlay').onclick=`,
	}
	if got := strings.Count(html, "onclick="); got != len(allowed) {
		t.Errorf("found %d onclick handlers, want %d", got, len(allowed))
	}
	for _, needle := range allowed {
		if !strings.Contains(html, needle) {
			t.Errorf("missing allowed handler %s", needle)
		}
	}
}

func TestNoBrowserStorageAccess(t *testing.T) {
	html := appHTML(t)
	for _, needle := range []string{"localStorage.getItem", "localStorage.setItem"} {
		if strings.Contains(html, needle) {
			t.Errorf("the page must not use %s", needle)
		}
	}
	for _, key := range []string{
		"'shiguang-study-v1'",
		"'shiguang-ai-config-v1'",
		"'shiguang-sync-pending-v1'",
	} {
		if !strings.Contains(html, key) {
			t.Errorf("the retired storage key %s must be cleaned up once", key)
		}
	}
}

func TestNoCommittedApiKeyLiterals(t *testing.T) {
	html := appHTML(t)
	if regexp.MustCompile(`sk-[A-Za-z0-9_-]{16,}`).MatchString(html) {
		t.Fatal("a literal API key was found in the app source")
	}
}

func TestProviderWiring(t *testing.T) {
	html := appHTML(t)
	for _, needle := range []string{"'qwen'", "'deepseek'", "enable_thinking",
		"{thinking:{type:'disabled'}}", "'/api/ai-key'", `meta[name="api-token"]`,
		"verified", "aiReview"} {
		if !strings.Contains(html, needle) {
			t.Errorf("missing provider wiring marker %q", needle)
		}
	}
	if strings.Contains(html, "/chat/completions") {
		t.Error("the browser must not talk to the provider itself")
	}
	for _, needle := range []string{"aiKey", "aiDeepseekKey"} {
		if strings.Contains(html, needle) {
			t.Errorf("the page must not contain %q", needle)
		}
	}
}
