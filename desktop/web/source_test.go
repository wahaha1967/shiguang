package web

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// sourcePage is the UI source of truth. The shipped copy in this directory
// must stay byte for byte identical to it; `wails3 task sync` enforces that.
func sourcePage(t *testing.T) []byte {
	t.Helper()
	path := filepath.Join("..", "..", "src", "app", "index.html")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			t.Skipf("the UI source of truth is not part of this checkout: %s", path)
		}
		t.Fatalf("read %s: %v", path, err)
	}
	return data
}

func TestTheShippedCopyMatchesTheSourceOfTruth(t *testing.T) {
	source := sourcePage(t)
	shipped := diskIndex(t)
	if !bytes.Equal(source, shipped) {
		t.Fatalf("web/index.html differs from src/app/index.html; run `wails3 task sync`")
	}
}
