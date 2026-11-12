// Command syncpage keeps the page that ships inside the binary in step with
// the UI source of truth.
//
// The application page has no build step, so the workflow is deliberate:
// functional edits land in src/app/index.html, this tool copies them byte for
// byte into desktop/web/index.html, and it rewrites the SHA-256 and the size
// in desktop/web/embed_test.go, which is the hash lock the web package tests
// enforce.
//
// Usage:
//
//	go run ./tools/syncpage          copy the page and refresh the hash lock
//	go run ./tools/syncpage -check   verify only; exit 1 when something is stale
package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
)

const (
	hashLabel = "indexSHA256"
	sizeLabel = "indexSize"
)

var (
	// The identifier is part of the match so that rewriting keeps it in place
	// and leaves the gofmt alignment of the const block untouched.
	hashRe = regexp.MustCompile(`(indexSHA256\s*=\s*")([^"]*)(")`)
	sizeRe = regexp.MustCompile(`(indexSize\s*=\s*)(\d+)`)
)

func main() {
	var (
		src       = flag.String("src", filepath.Join("..", "src", "app", "index.html"), "path to the UI source of truth")
		dst       = flag.String("dst", filepath.Join("web", "index.html"), "path to the copy embedded into the binary")
		lock      = flag.String("lock", filepath.Join("web", "embed_test.go"), "path to the file that holds the hash lock")
		checkOnly = flag.Bool("check", false, "verify only; write nothing")
	)
	flag.Parse()

	if err := sync(*src, *dst, *lock, *checkOnly); err != nil {
		fmt.Fprintln(os.Stderr, "syncpage: "+err.Error())
		os.Exit(1)
	}
}

// sync copies src over dst and refreshes the hash lock, or verifies both when
// checkOnly is set.
func sync(src, dst, lock string, checkOnly bool) error {
	source, err := os.ReadFile(src)
	if err != nil {
		return fmt.Errorf("read source page %s: %w", src, err)
	}
	sum := sha256.Sum256(source)
	hash := hex.EncodeToString(sum[:])
	size := len(source)

	if checkOnly {
		return verify(src, dst, lock, source, hash, size)
	}
	return apply(src, dst, lock, source, hash, size)
}

// apply writes the embedded copy and refreshes the hash lock.
func apply(src, dst, lock string, source []byte, hash string, size int) error {
	current, err := os.ReadFile(dst)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("read embedded copy %s: %w", dst, err)
	}
	if bytes.Equal(current, source) {
		fmt.Printf("copy is already up to date: %s\n", dst)
	} else {
		if err := os.WriteFile(dst, source, 0o644); err != nil {
			return fmt.Errorf("write embedded copy %s: %w", dst, err)
		}
		fmt.Printf("copied %s -> %s (%d bytes)\n", src, dst, size)
	}
	if err := updateLock(lock, hash, size); err != nil {
		return err
	}
	fmt.Printf("hash lock refreshed in %s: sha256=%s size=%d\n", lock, hash, size)
	return nil
}

// verify reports whether the embedded copy and the hash lock are current.
func verify(src, dst, lock string, source []byte, hash string, size int) error {
	current, err := os.ReadFile(dst)
	if err != nil {
		return fmt.Errorf("read embedded copy %s: %w", dst, err)
	}
	if !bytes.Equal(current, source) {
		return fmt.Errorf("the embedded copy %s is stale; run `wails3 task sync`", dst)
	}
	lockedHash, lockedSize, err := readLock(lock)
	if err != nil {
		return err
	}
	if lockedHash != hash || lockedSize != size {
		return fmt.Errorf("the hash lock in %s is stale (locked %s/%d, want %s/%d); run `wails3 task sync`",
			lock, lockedHash, lockedSize, hash, size)
	}
	fmt.Printf("up to date: %s == %s (sha256=%s, size=%d)\n", dst, src, hash, size)
	return nil
}

// updateLock rewrites the hash and the size in the lock file, keeping the
// surrounding layout untouched so the file stays gofmt clean.
func updateLock(path, hash string, size int) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read hash lock %s: %w", path, err)
	}
	text := string(raw)
	if n := len(hashRe.FindAllStringSubmatch(text, -1)); n != 1 {
		return fmt.Errorf("%s: expected exactly one %s constant, found %d", path, hashLabel, n)
	}
	if n := len(sizeRe.FindAllStringSubmatch(text, -1)); n != 1 {
		return fmt.Errorf("%s: expected exactly one %s constant, found %d", path, sizeLabel, n)
	}
	updated := hashRe.ReplaceAllString(text, "${1}"+hash+"${3}")
	updated = sizeRe.ReplaceAllString(updated, "${1}"+strconv.Itoa(size))
	if updated == text {
		return nil
	}
	if err := os.WriteFile(path, []byte(updated), 0o644); err != nil {
		return fmt.Errorf("write hash lock %s: %w", path, err)
	}
	return nil
}

// readLock returns the hash and the size recorded in the lock file.
func readLock(path string) (string, int, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", 0, fmt.Errorf("read hash lock %s: %w", path, err)
	}
	text := string(raw)
	hashMatch := hashRe.FindStringSubmatch(text)
	sizeMatch := sizeRe.FindStringSubmatch(text)
	if hashMatch == nil {
		return "", 0, fmt.Errorf("%s: no %s constant found", path, hashLabel)
	}
	if sizeMatch == nil {
		return "", 0, fmt.Errorf("%s: no %s constant found", path, sizeLabel)
	}
	size, err := strconv.Atoi(sizeMatch[2])
	if err != nil {
		return "", 0, fmt.Errorf("%s: bad %s value %q", path, sizeLabel, sizeMatch[2])
	}
	return hashMatch[2], size, nil
}
