package ui

import (
	"strings"
	"testing"

	"github.com/raccoon-overlord-dev/ticktick-tui/internal/api"
	"github.com/raccoon-overlord-dev/ticktick-tui/internal/theme"
)

// Attached files only show up as markdown links in the task body; the row gets a paperclip.
func TestAttachmentBadge(t *testing.T) {
	a := subApp(t)
	a.th, _ = theme.Load("terminal", true)
	a.cfg.Appearance.NerdFontIcons = true
	cases := []struct {
		name string
		task api.Task
		want bool
	}{
		{"content", api.Task{Title: "x", Content: "notes\n![file](6af8506f8c6c4f0220332154/piano.pdf)"}, true},
		{"desc on a checklist", api.Task{Title: "x", Kind: "CHECKLIST", Desc: "![file](a1/one.png)"}, true},
		{"plain task", api.Task{Title: "x", Content: "see https://example.com/a.pdf"}, false},
	}
	for _, c := range cases {
		got := strings.Contains(a.taskLine(a.pen(), &c.task, 0, 0, 80, false, false), "")
		if got != c.want {
			t.Errorf("%s: paperclip=%v, want %v", c.name, got, c.want)
		}
	}
}
