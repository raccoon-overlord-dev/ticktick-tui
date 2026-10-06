package ui

import (
	"errors"
	"slices"
	"strings"
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
		idMap: map[string]string{}}
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
	ls, row := in.lines(pen{th: th}, "text", 40)
	if len(ls) != 2 || ansi.Strip(ls[1]) != " " || row != 1 {
		t.Fatalf("got %q row %d", ls, row)
	}
}

// ↑↓ keep the column, the cursor row accounts for wrapping, and long notes scroll.
func TestLongNotes(t *testing.T) {
	th, _ := theme.Load("terminal")
	in := newInput("abcdefghij\nxy\nklmnop")
	in.cur = 4
	in.vmove(true)
	if in.cur != 13 { // end of "xy"
		t.Fatalf("down: cur %d", in.cur)
	}
	in.vmove(true)
	in.vmove(false)
	in.vmove(false)
	if in.cur != 2 {
		t.Fatalf("up: cur %d", in.cur)
	}
	in.cur = 8 // "i" lands on the 2nd row when wrapped at 4
	if _, row := in.lines(pen{th: th}, "text", 4); row != 2 {
		t.Fatalf("row %d", row)
	}

	a := testApp(t)
	a.th, a.w, a.h, a.now = th, 120, 20, time.Now()
	note := strings.Repeat("line\n", 60) + "LAST"
	a.st.Task("a").Content = note
	a.taskID = "a"
	a.openDetail(3)
	a.df = len(detailKeys(a.st.Task("a"))) - 1
	a.viewMain()
	for range 100 {
		a.move("detail", 1)
		a.viewMain()
	}
	if v, _ := a.viewMain(); !strings.Contains(ansi.Strip(v), "LAST") {
		t.Fatal("end of notes not reachable")
	}
	for range 100 {
		a.move("detail", -1)
		a.viewMain()
	}
	if k := detailKeys(a.st.Task("a"))[a.df]; k == "notes" {
		t.Fatal("↑ never left the notes")
	}

	// pgdown scrolls the notes too
	a.df, a.focus = len(detailKeys(a.st.Task("a")))-1, "detail"
	a.screen = screenMain
	a.viewMain()
	off := a.offDetail
	a.mainKey(tea.KeyPressMsg{Code: tea.KeyPgDown})
	if a.offDetail != off+a.page() {
		t.Fatalf("off %d → %d", off, a.offDetail)
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

// Folders start closed; opening one shows its lists and is saved, and jumping to a list opens its folder.
func TestFolders(t *testing.T) {
	a := testApp(t)
	config.Path = t.TempDir() + "/config.toml"
	a.st.Groups = []api.Group{{ID: "g", Name: "Work"}}
	a.st.Projects = []api.Project{{ID: "x", Name: "X", GroupID: "g"}}
	shown := func() bool {
		return slices.ContainsFunc(a.sideItems(), func(r sideItem) bool { return r.list == "p:x" })
	}
	if shown() {
		t.Fatal("folder starts open")
	}
	a.sideKey = "f:g"
	a.openSide()
	if !shown() {
		t.Fatal("folder didn't open")
	}
	if c, _ := config.Load(); !slices.Equal(c.Layout.OpenFolders, []string{"g"}) {
		t.Fatalf("not saved: %v", c.Layout.OpenFolders)
	}
	a.openSide()
	a.gotoList("p:x")
	if !shown() {
		t.Fatal("jumping to a list didn't open its folder")
	}
}

// Ticking the last open checklist item completes the task; ⏎ on an item edits its title.
func TestChecklistItems(t *testing.T) {
	a := testApp(t)
	tk := a.st.Task("a")
	tk.Items = []api.Item{{Title: "one", Status: 1}, {Title: "two"}}
	a.focus, a.df = "detail", slices.Index(detailKeys(tk), "c1")
	a.activate()
	if a.edit != "c1" || a.in.value() != "two" {
		t.Fatalf("⏎ didn't edit the item: edit=%q value=%q", a.edit, a.in.value())
	}
	a.in = newInput("two https://x.dev")
	a.commitEdit()
	if tk.Items[1].Title != "two https://x.dev" || tk.Items[1].Status != 0 {
		t.Fatalf("item not renamed: %+v", tk.Items[1])
	}
	a.toggleCheck(tk, 1)
	if !store.Done(tk) || a.queue[len(a.queue)-1].kind != "complete" {
		t.Fatalf("task not completed with its last item: %+v", tk)
	}
	a.toggleCheck(tk, 0)
	if store.Done(tk) || a.queue[len(a.queue)-1].fields["status"] != 0 {
		t.Fatalf("unticking an item didn't reopen the task: %+v", tk)
	}
}

// Moving a task out of the list by its due date selects the one above it, or the new top.
func TestDueKeepsPlace(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	due := time.Now().Format(api.DateLayout)
	var ts []api.Task
	for _, id := range []string{"a", "b", "c"} {
		ts = append(ts, api.Task{ID: id, ProjectID: "p1", Title: id, DueDate: due})
	}
	a := &App{cfg: config.Default(), st: store.New(nil, nil, ts, "", time.Now()), signed: &auth.Auth{AccessToken: "fake"},
		list: "today", now: time.Now(), idMap: map[string]string{}}
	nav := navTasks(a.groups())
	tomorrow, _ := parse.ParseDue("tomorrow", time.Now())
	a.taskID = nav[1].ID
	a.setDue(nav[1], tomorrow)
	if a.taskID != nav[0].ID {
		t.Fatalf("middle task moved: selected %s, want %s", a.taskID, nav[0].ID)
	}
	a.taskID = nav[0].ID
	a.setDue(nav[0], tomorrow)
	if a.taskID != nav[2].ID {
		t.Fatalf("top task moved: selected %s, want %s", a.taskID, nav[2].ID)
	}

	// the same for deleting and moving, here in a list
	a.list = "p:p1"
	nav = navTasks(a.groups())
	ids := []string{nav[0].ID, nav[1].ID, nav[2].ID}
	a.delID = ids[2]
	a.deleteKey(tea.KeyPressMsg{Code: 'y', Text: "y"})
	if a.st.Task(ids[2]) != nil || a.taskID != ids[1] {
		t.Fatalf("delete: selected %s, want %s", a.taskID, ids[1])
	}
	a.st.Projects = []api.Project{{ID: "p2", Name: "Other"}}
	a.moveTask(a.st.Task(ids[0]), "p:p2")
	if a.taskID != ids[1] {
		t.Fatalf("move: selected %s, want %s", a.taskID, ids[1])
	}
}
