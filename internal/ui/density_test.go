package ui

import (
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"

	"github.com/raccoon-overlord-dev/ticktick-tui/internal/auth"
	"github.com/raccoon-overlord-dev/ticktick-tui/internal/config"
	"github.com/raccoon-overlord-dev/ticktick-tui/internal/i18n"
	"github.com/raccoon-overlord-dev/ticktick-tui/internal/store"
	"github.com/raccoon-overlord-dev/ticktick-tui/internal/theme"
)

// Density scales the space around the rows; the selection index must keep pointing at the
// selected task's line, or the tasks pane scrolls to the wrong row.
func TestDensityTasksPane(t *testing.T) {
	for _, c := range []struct {
		density string
		rowGap  int
	}{
		{"compact", 0},
		{"normal", 0},
		{"roomy", 1},
	} {
		a := subApp(t)
		a.th, _ = theme.Load("terminal", true)
		a.cfg.Appearance.Density = c.density
		a.taskID = "s"
		lines, sel, _, total := a.tasksLines(a.pen(), 60, true)
		if total != 5 || sel < 0 {
			t.Fatalf("%s: total=%d sel=%d", c.density, total, sel)
		}
		if !strings.Contains(lines[sel], "Sibling") {
			t.Errorf("%s: sel=%d points at %q, want the selected task", c.density, sel, lines[sel])
		}
		// 5 tasks in one group: rowGap blanks between each pair, no group separator.
		want := 5 + 4*c.rowGap
		if len(lines) != want {
			t.Errorf("%s: %d lines, want %d", c.density, len(lines), want)
		}
	}
}

// Every rendered row must fill its pane edge to edge whatever the padding, or the frame's
// right border shifts and the panes overlap.
func TestDensityRowWidth(t *testing.T) {
	t.Cleanup(func() { i18n.Lang = "en" })
	for _, lang := range []string{"en", "it"} { // Italian runs 15-20% longer
		i18n.Lang = lang
		densityRowWidths(t)
	}
}

func densityRowWidths(t *testing.T) {
	for _, d := range []string{"compact", "normal", "roomy"} {
		for _, cols := range []int{160, 100, 60} {
			t.Setenv("XDG_CACHE_HOME", t.TempDir())
			t.Setenv("XDG_CONFIG_HOME", t.TempDir())
			config.Path = t.TempDir() + "/config.toml"
			th, _ := theme.Load("terminal", true)
			a := &App{cfg: config.Default(), th: th, st: store.Demo(time.Now()), signed: &auth.Auth{AccessToken: "x"},
				idMap: map[string]string{}, list: "inbox", w: cols, h: 24, now: time.Now()}
			a.cfg.Appearance.Density = d
			_, lw, tw, dw := layout(cols, a.cfg.Layout.Columns, dens(d))
			p := a.pen()
			check := func(what string, w int, lines []string) {
				for i, l := range lines {
					if l == "" {
						continue
					}
					if got := lipgloss.Width(l); got != w-2 {
						t.Errorf("%s %dcols %s line %d: width %d, want %d: %q", d, cols, what, i, got, w-2, l)
					}
				}
			}
			if lw > 0 {
				ll, _ := a.listsLines(p, lw, false)
				check("lists", lw, ll)
			}
			tl, _, _, _ := a.tasksLines(p, tw, false)
			check("tasks", tw, tl)
			if dw > 0 {
				dl, _ := a.detailLines(p, a.selTask(), dw, false, 0, new(int))
				check("detail", dw, dl)
			}
		}
	}
}
