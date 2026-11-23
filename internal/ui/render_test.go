package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/raccoon-overlord-dev/ticktick-tui/internal/api"
	"github.com/raccoon-overlord-dev/ticktick-tui/internal/auth"
	"github.com/raccoon-overlord-dev/ticktick-tui/internal/config"
	"github.com/raccoon-overlord-dev/ticktick-tui/internal/store"
	"github.com/raccoon-overlord-dev/ticktick-tui/internal/theme"
	"github.com/raccoon-overlord-dev/ticktick-tui/internal/update"
)

func TestLayout(t *testing.T) {
	cases := []struct {
		cols    int
		setting string
		density string
		n, l, t int
	}{
		{160, "auto", "normal", 3, 28, 64}, // 158-28-2=128 → 64/64
		{120, "auto", "normal", 3, 24, 46}, // 118-24-2=92 → 46/46
		{119, "auto", "normal", 2, 0, 59},  // 116 → 59.4 rounds to 59
		{100, "auto", "normal", 2, 0, 50},
		{79, "auto", "normal", 1, 0, 77},
		{160, "1", "normal", 1, 0, 158},
		{90, "3", "normal", 3, 24, 31},
		{160, "auto", "compact", 3, 20, 70}, // no gutters: 160-20=140 → 70/70
		{160, "auto", "roomy", 3, 30, 61},   // 156-30-4=122 → 61/61
		{100, "auto", "compact", 2, 0, 51},
		{100, "auto", "roomy", 2, 0, 48},
		{160, "1", "compact", 1, 0, 160},
	}
	for _, c := range cases {
		dy := dens(c.density)
		n, l, tk, d := layout(c.cols, c.setting, dy)
		if n != c.n || l != c.l || tk != c.t {
			t.Errorf("layout(%d,%s,%s) n=%d lists=%d tasks=%d", c.cols, c.setting, c.density, n, l, tk)
		}
		// Panes + gutters + outer padding must fill the terminal exactly.
		used := 2*dy.gutter + tk
		switch n {
		case 3:
			used += l + d + 2*dy.gutter
		case 2:
			used += d + dy.gutter
		}
		if used != c.cols {
			t.Errorf("layout(%d,%s,%s) uses %d cols", c.cols, c.setting, c.density, used)
		}
		if n == 2 && tk < d {
			t.Errorf("2 panes: tasks %d should be ≥ detail %d", tk, d)
		}
	}
}

func TestWindow(t *testing.T) {
	lines := []string{"0", "1", "2", "3", "4", "5"}
	off := 0
	if got := window(lines, 4, 3, &off); got[0] != "2" || off != 2 {
		t.Fatalf("scroll down: %v off=%d", got, off)
	}
	if got := window(lines, 1, 3, &off); got[0] != "1" || off != 1 {
		t.Fatalf("scroll up: %v off=%d", got, off)
	}
	off = 10
	if got := window(lines, -1, 3, &off); got[0] != "3" {
		t.Fatalf("clamp: %v", got)
	}
}

// Rows must measure the same with grapheme and wcwidth counting, or they spill into the next column.
func TestWcSafe(t *testing.T) {
	for _, s := range []string{"\x1b[1m✓ ⚠️ FIESTA ⚠️\x1b[0m", "👨🏻‍💻Coding", "♻️Other 🧔🪒", "plain àè"} {
		got := wcSafe(s)
		if ansi.StringWidth(got) != ansi.StringWidth(s) || ansi.StringWidthWc(got) != ansi.StringWidth(s) {
			t.Errorf("wcSafe(%q) = %q", s, got)
		}
	}
}

// Note and checklist URLs, and the About box's repo link, reach the screen as OSC 8 links.
func TestLinksRendered(t *testing.T) {
	th, _ := theme.Load("terminal", true)
	task := api.Task{ID: "a", ProjectID: "inbox1", Title: "Task A", Content: "see [docs](https://docs.dev)",
		Items: []api.Item{{ID: "i", Title: "buy https://shop.dev"}}}
	a := &App{cfg: config.Default(), th: th, st: store.New(nil, nil, []api.Task{task}, "", time.Now()), signed: &auth.Auth{AccessToken: "fake"},
		list: "inbox", taskID: "a", focus: "tasks", sideKey: "l:inbox", screen: screenMain, idMap: map[string]string{}, w: 160, h: 45, now: time.Now(), version: "v9.9.9"}
	out := a.View().Content
	for _, u := range []string{"https://docs.dev", "https://shop.dev"} {
		if !strings.Contains(out, "\x1b]8;;"+u+"\a") {
			t.Errorf("no link to %s", u)
		}
	}
	a.settings, a.about = true, true
	if out := a.View().Content; !strings.Contains(out, "\x1b]8;;"+update.Repo+"\a") || !strings.Contains(out, "v9.9.9") {
		t.Error("About box missing version or repo link")
	}
}
