package buildinfo

import (
	"strconv"
	"testing"
	"time"
)

func stamp(t *testing.T, commit string, at time.Time) {
	t.Helper()
	oldCommit, oldBuiltAt := Commit, BuiltAt
	t.Cleanup(func() { Commit, BuiltAt = oldCommit, oldBuiltAt })
	Commit = commit
	if at.IsZero() {
		BuiltAt = ""
	} else {
		BuiltAt = strconv.FormatInt(at.Unix(), 10)
	}
}

func TestDateSplitsTheDayAtNoon(t *testing.T) {
	cases := []struct {
		hour int
		want string
	}{
		{0, "2026-11-09 上午"},
		{9, "2026-11-09 上午"},
		{12, "2026-11-09 下午"},
		{23, "2026-11-09 下午"},
	}
	for _, c := range cases {
		stamp(t, "abc1234", time.Date(2026, 11, 9, c.hour, 30, 0, 0, time.Local))
		if got := Date(); got != c.want {
			t.Errorf("hour %d: Date() = %q, want %q", c.hour, got, c.want)
		}
	}
}

func TestUnstampedOrInvalidBuildHasNoDate(t *testing.T) {
	for _, value := range []string{"", "   ", "not-a-number"} {
		stamp(t, "", time.Time{})
		BuiltAt = value
		if got := Date(); got != "" {
			t.Errorf("BuiltAt %q: Date() = %q, want empty", value, got)
		}
	}
}

func TestMetaJoinsVersionCommitAndDate(t *testing.T) {
	stamp(t, "73fe8d", time.Date(2026, 11, 9, 15, 0, 0, 0, time.Local))
	if got, want := Meta(), Version+"|73fe8d|2026-11-09 下午"; got != want {
		t.Errorf("Meta() = %q, want %q", got, want)
	}
	stamp(t, "", time.Time{})
	if got, want := Meta(), Version+"||"; got != want {
		t.Errorf("unstamped Meta() = %q, want %q", got, want)
	}
}