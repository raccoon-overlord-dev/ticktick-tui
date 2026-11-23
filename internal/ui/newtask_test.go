package ui

import (
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/raccoon-overlord-dev/ticktick-tui/internal/theme"
)

// The New task panel borrows df/offDetail, so the details pane behind it must keep rendering
// from the values openDraft saved: arrows in the panel used to move the pane behind as well.
func TestDraftLeavesDetailsPaneAlone(t *testing.T) {
	a := testApp(t)
	a.th, _ = theme.Load("terminal", true)
	a.screen, a.w, a.h, a.now = screenMain, 120, 40, time.Now()
	a.taskID, a.focus = "a", "tasks"
	key := func(k tea.KeyPressMsg) { a.Update(k) }
	down, esc := tea.KeyPressMsg{Code: tea.KeyDown}, tea.KeyPressMsg{Code: tea.KeyEscape}

	// focus the details pane and move its field cursor off the title
	key(tea.KeyPressMsg{Code: tea.KeyEnter})
	if a.focus != "detail" {
		t.Fatalf("focus %q, want detail", a.focus)
	}
	key(down)
	key(down)
	key(down)
	want := a.df
	if want == 0 {
		t.Fatal("details cursor didn't move")
	}

	key(tea.KeyPressMsg{Code: 'n', Text: "n"})
	for _, r := range "Buy milk" { // a title, so esc later asks instead of closing
		key(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	key(tea.KeyPressMsg{Code: tea.KeyEnter}) // commit the title, stay in the panel
	if a.draft == nil {
		t.Fatal("panel closed")
	}
	key(down) // one field, so the panel's cursor and the pane's can't coincide
	if a.df == 0 || a.df == want {
		t.Fatalf("the panel's own cursor didn't move: %d (pane at %d)", a.df, want)
	}
	if got, _ := a.mainDetail(); got != want {
		t.Errorf("details pane behind moved to %d, want %d", got, want)
	}
	a.viewMain() // the background render must not clobber the saved values either
	if got, _ := a.mainDetail(); got != want {
		t.Errorf("after render: %d, want %d", got, want)
	}

	key(esc)
	key(tea.KeyPressMsg{Code: 'y', Text: "y"}) // discard
	if a.draft != nil || a.df != want {
		t.Errorf("after discard: draft %v df %d, want %d", a.draft, a.df, want)
	}
}
