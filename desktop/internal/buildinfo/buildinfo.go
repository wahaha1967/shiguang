// Package buildinfo carries the version facts stamped into the binary at link
// time and served to the page through the app-build meta tag.
package buildinfo

import (
	"strconv"
	"strings"
	"time"
)

// Version is the product version; keep it in step with build/windows/info.json
// and build/windows/app.manifest.
var Version = "3.0.1"

// Commit and BuiltAt are stamped by `wails3 task windows:build` via -ldflags -X.
// BuiltAt is Unix seconds in the build machine's zone; empty values mean a
// plain `go run` development build, which the page renders as 开发构建.
var (
	Commit  = ""
	BuiltAt = ""
)

// Date renders BuiltAt as "2006-01-02 上午/下午" in local time, or "" when the
// binary was not stamped.
func Date() string {
	sec, err := strconv.ParseInt(strings.TrimSpace(BuiltAt), 10, 64)
	if err != nil {
		return ""
	}
	t := time.Unix(sec, 0)
	half := "上午"
	if t.Hour() >= 12 {
		half = "下午"
	}
	return t.Format("2006-01-02") + " " + half
}

// Meta is the pipe-separated content of the app-build meta tag.
func Meta() string {
	return strings.Join([]string{Version, Commit, Date()}, "|")
}
