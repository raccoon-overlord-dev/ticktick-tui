package ui

import (
	"errors"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"ttui/internal/api"
	"ttui/internal/auth"
	"ttui/internal/config"
	"ttui/internal/parse"
	"ttui/internal/store"
	"ttui/internal/theme"
)

func testApp(t *testing.T) *App {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	st := store.New(nil, nil, []api.Task{{ID: "a", ProjectID: "inbox1", Title: "Task A", Priority: 5}}, "", time.Now())
	return &App{cfg: config.Default(), st: st, signed: &auth.Auth{AccessToken: "fake"}, list: "inbox",
		folded: map[string]bool{}, idMap: map[string]string{}}
}

func TestOptimisticRevert(t *testing.T) {
	a := testApp(t)
	a.cyclePrio(a.st.Task("a")) // high → medium, queued and "in flight"
	if a.st.Task("a").Priority != store.PrioMed || !a.busy {
		t.Fatalf("not applied optimistically: %+v busy=%v", a.st.Task("a"), a.busy)
	}
	a.onOpDone(opDoneMsg{op: a.queue[0], err: errors.New("boom")})
	if a.st.Task("a").Priority != store.PrioHigh || a.flashRole != "error" {
		t.Fatalf("not reverted: %+v flash=%q", a.st.Task("a"), a.flash)
	}
}

func TestCreateReplacesTempID(t *testing.T) {
	a := testApp(t)
	a.createTask(parse.Add{Title: "New", List: "inbox"})
	tmp := a.taskID
	if a.st.Task(tmp) == nil || a.queue[0].fields["projectId"] != nil {
		t.Fatalf("temp task missing or inbox projectId sent: %+v", a.queue[0].fields)
	}
	// An edit made before the create returns is queued against the temp id.
	a.cyclePrio(a.st.Task(tmp))
	a.onOpDone(opDoneMsg{op: a.queue[0], t: &api.Task{ID: "real", ProjectID: "inbox1", Title: "New"}})
	if a.taskID != "real" || a.st.Task(tmp) != nil || a.idMap[tmp] != "real" || a.st.Task("real") == nil {
		t.Fatalf("temp id not replaced: sel=%s idMap=%v", a.taskID, a.idMap)
	}
	if !a.busy || a.queue[0].taskID != tmp { // the queued edit is sent next, resolved via idMap
		t.Fatalf("queued edit lost: %+v", a.queue)
	}

	b := testApp(t)
	b.createTask(parse.Add{Title: "Fails", List: "inbox"})
	tmp = b.taskID
	b.onOpDone(opDoneMsg{op: b.queue[0], err: errors.New("boom")})
	if b.st.Task(tmp) != nil {
		t.Fatal("failed create not removed")
	}
}

// After a line break the cursor must sit at column 0 of the new line.
func TestNotesCursorAfterNewline(t *testing.T) {
	th, err := theme.Load("terminal")
	if err != nil {
		t.Fatal(err)
	}
	in := newInput("hello")
	in.insert("\n")
	ls := in.lines(pen{th: th}, "text", 40)
	if len(ls) != 2 || ansi.Strip(ls[1]) != " " {
		t.Fatalf("got %q", ls)
	}
}

func TestMoveTask(t *testing.T) {
	a := testApp(t)
	a.st.Projects = []api.Project{{ID: "work", Name: "Work"}}
	a.taskID = "a"
	a.movePicker(a.st.Task("a"))
	if a.cmd == nil || len(a.cmd.pick.items) != 2 { // Inbox + Work
		t.Fatalf("picker: %+v", a.cmd)
	}
	a.cmd.in = newInput("wor")
	a.cmdKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	if a.st.Task("a").ProjectID != "work" || a.queue[0].kind != "move" || a.queue[0].fields["from"] != "inbox1" {
		t.Fatalf("not moved: %+v %+v", a.st.Task("a"), a.queue)
	}
	a.onOpDone(opDoneMsg{op: a.queue[0], err: errors.New("boom")})
	if a.st.Task("a").ProjectID != "inbox1" {
		t.Fatal("failed move not reverted")
	}
	a.moveTask(a.st.Task("a"), "p:work")
	a.moveTask(a.st.Task("a"), "inbox") // the Inbox id comes from a synced task in it (none now)
	if a.st.Task("a").ProjectID != "work" || a.flashRole != "error" {
		t.Fatalf("moved to an unknown Inbox: %+v", a.st.Task("a"))
	}
}

func TestAddChecklistItems(t *testing.T) {
	a := testApp(t)
	a.w, a.taskID = 160, "a"
	a.startEdit("additem")
	for _, s := range []string{"one", "two"} {
		a.in = newInput(s)
		a.editKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	}
	if a.edit != "additem" { // stays open for the next item
		t.Fatalf("editor closed: %q", a.edit)
	}
	a.editKey(tea.KeyPressMsg{Code: tea.KeyEscape})
	items := a.st.Task("a").Items
	if a.edit != "" || len(items) != 2 || items[1].Title != "two" || items[1].ID != "" {
		t.Fatalf("items: %+v edit=%q", items, a.edit)
	}
}

func TestDuePickerKeepsTime(t *testing.T) {
	a := testApp(t)
	task := a.st.Task("a")
	task.DueDate, task.IsAllDay = time.Now().Add(-48*time.Hour).Format(api.DateLayout), false
	at, _ := store.DueTime(task)
	a.duePicker(task)
	if n := len(a.cmd.pick.items); n != len(a.cfg.Tasks.DueMenu)+2 {
		t.Fatalf("%d items", n)
	}
	a.cmdKey(tea.KeyPressMsg{Code: tea.KeyEnter}) // first entry: today
	got, _ := store.DueTime(task)
	if task.IsAllDay || got.Hour() != at.Hour() || got.Minute() != at.Minute() || got.YearDay() != time.Now().YearDay() {
		t.Fatalf("due %v, want today at %s", got, at.Format("15:04"))
	}
}
