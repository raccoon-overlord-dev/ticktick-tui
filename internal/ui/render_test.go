package ui

import (
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestLayout(t *testing.T) {
	cases := []struct {
		cols    int
		setting string
		n, l, t int
	}{
		{160, "auto", 3, 28, 64}, // 158-28-2=128 → 64/64
		{120, "auto", 3, 24, 46}, // 118-24-2=92 → 46/46
		{119, "auto", 2, 0, 60},  // 116 → 59.4 rounds to 59? see below
		{100, "auto", 2, 0, 50},
		{79, "auto", 1, 0, 77},
		{160, "1", 1, 0, 158},
		{90, "3", 3, 24, 31},
	}
	for _, c := range cases {
		n, l, tk, d := layout(c.cols, c.setting)
		if n != c.n || l != c.l {
			t.Errorf("layout(%d,%s) n=%d lists=%d", c.cols, c.setting, n, l)
		}
		// Panes + gutters + outer padding must fill the terminal exactly.
		used := 2 + tk
		switch n {
		case 3:
			used += l + d + 2
		case 2:
			used += d + 1
		}
		if used != c.cols {
			t.Errorf("layout(%d,%s) uses %d cols", c.cols, c.setting, used)
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
