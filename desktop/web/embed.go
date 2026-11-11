// Package web embeds the single-file application for release builds.
package web

import (
	"embed"
	"os"
	"path/filepath"
)

//go:embed index.html
var assets embed.FS

// IndexHTML returns the page from disk in development mode and from the
// binary otherwise.
func IndexHTML(dev bool) ([]byte, error) {
	if dev {
		for _, path := range devCandidates() {
			if data, err := os.ReadFile(path); err == nil {
				return data, nil
			}
		}
	}
	return assets.ReadFile("index.html")
}

func devCandidates() []string {
	candidates := []string{filepath.Join("web", "index.html")}
	if exe, err := os.Executable(); err == nil {
		candidates = append(candidates, filepath.Join(filepath.Dir(exe), "web", "index.html"))
	}
	return candidates
}
