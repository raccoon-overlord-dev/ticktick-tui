package ui

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/raccoon-overlord-dev/ticktick-tui/internal/i18n"
	"github.com/raccoon-overlord-dev/ticktick-tui/internal/theme"
)

// The strings declared in a table reach i18n.T through a variable, so internal/i18n's scanner
// can't see them. Check them here instead.
func TestItalianCoversTheTables(t *testing.T) {
	has := func(s string) {
		t.Helper()
		if s != "" && !i18n.Has("it", s) {
			t.Errorf("missing Italian for %q", s)
		}
	}
	for _, r := range setRows {
		has(r.sec)
		has(r.label)
		has(r.desc)
	}
	for _, r := range helpKeys {
		has(r.sec)
		has(r.desc)
	}
	for _, v := range repLabels {
		has(v)
	}
	for _, v := range ordLabel {
		has(v)
	}
	for _, v := range sortLabel {
		has(v)
	}
	for _, v := range modeLabel {
		has(v[0])
	}
	for _, u := range repUnits {
		has(u)
		has(u + "s")
	}
}

// Italian runs longer than English: every overlay must still fit the width it reports, in
// both languages and at a narrow terminal.
func TestOverlaysFitInItalian(t *testing.T) {
	t.Cleanup(func() { i18n.Lang = "en" })
	for _, lang := range []string{"en", "it"} {
		i18n.Lang = lang
		for _, cols := range []int{160, 100, 60} {
			a := subApp(t)
			a.th, _ = theme.Load("terminal", true)
			a.w, a.h = cols, 40
			for name, view := range map[string]func() (string, int, int){
				"settings": a.viewSettings, "keys": a.viewHelp, "about": a.viewAbout,
			} {
				s, w, _ := view()
				for i, l := range strings.Split(s, "\n") {
					if got := lipgloss.Width(l); got > w {
						t.Errorf("%s %dcols %s line %d: width %d > %d: %q", lang, cols, name, i, got, w, l)
					}
				}
			}
		}
	}
}
