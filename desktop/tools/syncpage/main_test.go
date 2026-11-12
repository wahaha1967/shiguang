package main

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

const lockFixture = `package web

import "testing"

const (
	indexSHA256 = "0000000000000000000000000000000000000000000000000000000000000000"
	indexSize   = 1
)

func TestNothing(t *testing.T) {}
`

const pageFixture = "<html><body><script>console.log(1)</script></body></html>\n"

type fixture struct {
	dir  string
	src  string
	dst  string
	lock string
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	dir := t.TempDir()
	f := fixture{
		dir:  dir,
		src:  filepath.Join(dir, "src", "index.html"),
		dst:  filepath.Join(dir, "web", "index.html"),
		lock: filepath.Join(dir, "web", "embed_test.go"),
	}
	mkdir(t, filepath.Dir(f.src))
	mkdir(t, filepath.Dir(f.dst))
	write(t, f.src, pageFixture)
	write(t, f.dst, "<html>stale</html>")
	write(t, f.lock, lockFixture)
	return f
}

func mkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
}

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func read(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(raw)
}

func wantHash(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}

func TestSyncCopiesThePageAndRefreshesTheLock(t *testing.T) {
	f := newFixture(t)

	if err := sync(f.src, f.dst, f.lock, false); err != nil {
		t.Fatalf("sync: %v", err)
	}

	if got := read(t, f.dst); got != pageFixture {
		t.Errorf("the embedded copy was not updated: %q", got)
	}

	hash, size, err := readLock(f.lock)
	if err != nil {
		t.Fatalf("read lock: %v", err)
	}
	if hash != wantHash(pageFixture) {
		t.Errorf("locked hash %s, want %s", hash, wantHash(pageFixture))
	}
	if size != len(pageFixture) {
		t.Errorf("locked size %d, want %d", size, len(pageFixture))
	}

	lock := read(t, f.lock)
	if !strings.Contains(lock, "indexSHA256 = \""+wantHash(pageFixture)+"\"") {
		t.Errorf("the lock file lost its layout:\n%s", lock)
	}
	if !strings.Contains(lock, "indexSize   = "+strconv.Itoa(len(pageFixture))) {
		t.Errorf("the lock file lost its layout:\n%s", lock)
	}
}

func TestSyncIsIdempotent(t *testing.T) {
	f := newFixture(t)

	if err := sync(f.src, f.dst, f.lock, false); err != nil {
		t.Fatalf("first sync: %v", err)
	}
	first := read(t, f.lock)
	if err := sync(f.src, f.dst, f.lock, false); err != nil {
		t.Fatalf("second sync: %v", err)
	}
	if second := read(t, f.lock); second != first {
		t.Errorf("a second sync rewrote the lock file:\n%s", second)
	}
}

func TestCheckPassesWhenEverythingIsInSync(t *testing.T) {
	f := newFixture(t)
	if err := sync(f.src, f.dst, f.lock, false); err != nil {
		t.Fatalf("sync: %v", err)
	}
	if err := sync(f.src, f.dst, f.lock, true); err != nil {
		t.Fatalf("check: %v", err)
	}
}

func TestCheckReportsAStaleCopy(t *testing.T) {
	f := newFixture(t)
	if err := sync(f.src, f.dst, f.lock, false); err != nil {
		t.Fatalf("sync: %v", err)
	}
	write(t, f.src, pageFixture+"<!-- edited -->\n")

	err := sync(f.src, f.dst, f.lock, true)
	if err == nil {
		t.Fatal("check must fail when the embedded copy is stale")
	}
	if !strings.Contains(err.Error(), "stale") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestCheckReportsAStaleLock(t *testing.T) {
	f := newFixture(t)
	if err := sync(f.src, f.dst, f.lock, false); err != nil {
		t.Fatalf("sync: %v", err)
	}
	write(t, f.lock, strings.Replace(read(t, f.lock), wantHash(pageFixture), "deadbeef", 1))

	err := sync(f.src, f.dst, f.lock, true)
	if err == nil {
		t.Fatal("check must fail when the hash lock is stale")
	}
	if !strings.Contains(err.Error(), "stale") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestUpdateLockNeedsExactlyOneConstant(t *testing.T) {
	f := newFixture(t)
	write(t, f.lock, "package web\n")

	if err := updateLock(f.lock, wantHash(pageFixture), len(pageFixture)); err == nil {
		t.Fatal("updateLock must fail when the constants are missing")
	}
}
