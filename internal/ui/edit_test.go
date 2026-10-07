package ui

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/raccoon-overlord-dev/ticktick-tui/internal/api"
	"github.com/raccoon-overlord-dev/ticktick-tui/internal/auth"
	"github.com/raccoon-overlord-dev/ticktick-tui/internal/config"
	"github.com/raccoon-overlord-dev/ticktick-tui/internal/parse"
	"github.com/raccoon-overlord-dev/ticktick-tui/internal/store"
	"github.com/raccoon-overlord-dev/ticktick-tui/internal/theme"
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
		if a.detailEnd {
			break
		}
		a.move("detail", 1)
		a.viewMain()
	}
	if v, _ := a.viewMain(); !strings.Contains(ansi.Strip(v), "LAST") {
		t.Fatal("end of notes not reachable")
	}
	if a.move("detail", 1); a.df != 0 || a.offDetail != 0 { // ↓ at the end wraps to the title
		t.Fatalf("no wrap at the end of the notes: df %d off %d", a.df, a.offDetail)
	}
	a.df = len(detailKeys(a.st.Task("a"))) - 1
	a.viewMain()
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
	if n := len(a.cmd.pick.items); n != len(a.cfg.Tasks.DueMenu)+3 {
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

func TestStepWraps(t *testing.T) {
	for _, c := range []struct{ i, d, n, want int }{
		{4, 1, 5, 0}, {0, -1, 5, 4}, {2, 1, 5, 3}, {3, 10, 5, 4}, {1, -10, 5, 0}, {0, 1, 0, 0},
	} {
		if got := step(c.i, c.d, c.n); got != c.want {
			t.Errorf("step(%d, %d, %d) = %d, want %d", c.i, c.d, c.n, got, c.want)
		}
	}
}

// Selection (ctrl+a, shift+arrows), copy / cut, and undo / redo grouped by word.
func TestInputSelectUndo(t *testing.T) {
	press := func(in *textInput, keys ...tea.KeyPressMsg) {
		for _, k := range keys {
			in.key(k, true)
		}
	}
	typ := func(in *textInput, s string) {
		for _, r := range s {
			press(in, tea.KeyPressMsg{Code: r, Text: string(r)})
		}
	}
	ctrl := func(r rune) tea.KeyPressMsg { return tea.KeyPressMsg{Code: r, Mod: tea.ModCtrl} }
	shift := func(c rune) tea.KeyPressMsg { return tea.KeyPressMsg{Code: c, Mod: tea.ModShift} }

	in := newInput("old text")
	press(&in, ctrl('a'))
	if in.selected() != "old text" {
		t.Fatalf("ctrl+a selected %q", in.selected())
	}
	typ(&in, "hello world")
	if in.value() != "hello world" {
		t.Fatalf("typing didn't replace the selection: %q", in.value())
	}
	press(&in, ctrl('z'))
	if in.value() != "hello " {
		t.Fatalf("undo one word: %q", in.value())
	}
	press(&in, ctrl('z'), ctrl('z'))
	if in.value() != "old text" {
		t.Fatalf("undo to the start: %q", in.value())
	}
	press(&in, ctrl('y'))
	if in.value() != "hello " {
		t.Fatalf("redo: %q", in.value())
	}

	in = newInput("abc def")
	press(&in, shift(tea.KeyLeft), shift(tea.KeyLeft), shift(tea.KeyLeft))
	if in.selected() != "def" {
		t.Fatalf("shift+left selected %q", in.selected())
	}
	press(&in, tea.KeyPressMsg{Code: tea.KeyBackspace})
	if in.value() != "abc " {
		t.Fatalf("backspace on selection: %q", in.value())
	}

	a := testApp(t)
	a.screen, a.w, a.h = screenMain, 120, 30
	a.th, _ = theme.Load("terminal")
	tk := a.st.Task("a")
	tk.Items = []api.Item{{Title: "item"}}
	a.taskID, a.focus, a.df = "a", "detail", slices.Index(detailKeys(tk), "c0")
	a.mainKey(ctrl('a'))
	if a.edit != "c0" || a.in.selected() != "item" {
		t.Fatalf("ctrl+a on an item: edit=%q selected=%q", a.edit, a.in.selected())
	}
	if _, cmd := a.Update(ctrl('x')); cmd == nil || a.in.value() != "" || a.edit != "c0" {
		t.Fatalf("ctrl+x: value=%q edit=%q", a.in.value(), a.edit)
	}
	if _, cmd := a.Update(ctrl('c')); cmd != nil || a.edit == "" {
		t.Fatal("ctrl+c while editing must not quit")
	}
}

// n → New task: fields are set on a draft, Create sends everything in one create.
func TestNewTaskPanel(t *testing.T) {
	a := testApp(t)
	a.th, _ = theme.Load("terminal")
	a.screen, a.w, a.h, a.now, a.list = screenMain, 120, 40, time.Now(), "today"
	key := func(k tea.KeyPressMsg) { a.Update(k) }
	press := func(s string) {
		for _, r := range s {
			key(tea.KeyPressMsg{Code: r, Text: string(r)})
		}
	}
	enter, esc := tea.KeyPressMsg{Code: tea.KeyEnter}, tea.KeyPressMsg{Code: tea.KeyEscape}

	key(tea.KeyPressMsg{Code: 'n', Text: "n"})
	if a.draft == nil || a.draft.DueDate == "" {
		t.Fatalf("n didn't open a draft due today: %+v", a.draft)
	}
	if v, _ := a.viewMain(); !strings.Contains(ansi.Strip(v), "New task") {
		t.Fatal("panel not drawn")
	}
	if a.edit != "title" {
		t.Fatalf("title not in edit: %q", a.edit)
	}
	key(esc) // unchanged: closes at once
	if a.draft != nil {
		t.Fatal("esc on an untouched draft should close it")
	}

	key(tea.KeyPressMsg{Code: 'n', Text: "n"}) // opens with the title being edited
	press("Buy milk")
	key(enter)
	press("p") // high
	press("c")
	press("eggs")
	key(enter)
	key(esc)
	if a.draft.Title != "Buy milk" || a.draft.Priority != store.PrioHigh || len(a.draft.Items) != 1 || len(a.queue) != 0 {
		t.Fatalf("draft: %+v queue %d", a.draft, len(a.queue))
	}
	key(esc) // changed: asks first
	if !a.draftAsk {
		t.Fatal("no discard prompt")
	}
	key(tea.KeyPressMsg{Code: 'n', Text: "n"}) // no
	key(tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl})
	if a.draft != nil || len(a.queue) != 1 {
		t.Fatalf("not created: draft %v queue %d", a.draft, len(a.queue))
	}
	f := a.queue[0].fields
	if f["title"] != "Buy milk" || f["priority"] != store.PrioHigh || f["kind"] != "CHECKLIST" || f["dueDate"] == nil || f["projectId"] != nil {
		t.Fatalf("create body: %v", f)
	}
	if nt := a.st.Task(a.taskID); nt == nil || nt.Title != "Buy milk" {
		t.Fatal("new task not selected")
	}
}

// The calendar sets a due date with a time; the repeat form writes the web app's rule.
func TestCalendarAndRepeatForm(t *testing.T) {
	a := testApp(t)
	a.th, _ = theme.Load("terminal")
	a.screen, a.w, a.h, a.now, a.focus = screenMain, 120, 40, time.Now(), "detail"
	tk := a.st.Task("a")
	a.pickDue(tk)
	key := func(s string) { a.Update(tea.KeyPressMsg{Code: []rune(s)[0], Text: s}) }
	special := func(c rune) { a.Update(tea.KeyPressMsg{Code: c}) }
	if v, _ := a.viewMain(); !strings.Contains(ansi.Strip(v), time.Now().Format("January 2006")) {
		t.Fatal("calendar not drawn")
	}
	special(tea.KeyRight)
	key("t")
	for _, r := range "17:30" {
		key(string(r))
	}
	special(tea.KeyEnter) // set the time
	special(tea.KeyEnter) // pick
	due, _ := store.DueTime(tk)
	if a.cal != nil || tk.IsAllDay || due.Hour() != 17 || due.Minute() != 30 || daysFrom(time.Now(), due) != 1 {
		t.Fatalf("due %v allDay %v", due, tk.IsAllDay)
	}

	a.openRepeatForm(tk)
	r := a.rep
	special(tea.KeyDown)  // every
	special(tea.KeyDown)  // unit
	special(tea.KeyRight) // week → month
	special(tea.KeyDown)  // month by: each
	special(tea.KeyDown)  // grid, cursor on 1
	key(" ")              // toggle the 1st (the due day is on already)
	if v, _ := a.viewMain(); !strings.Contains(ansi.Strip(v), "Custom repeat") {
		t.Fatal("repeat form not drawn")
	}
	a.Update(tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl})
	want := []int{due.Day()}
	if due.Day() != 1 {
		want = []int{1, due.Day()}
	} else {
		want = nil
	}
	got := parse.FormFromRule(tk.RepeatFlag, tk.RepeatFrom, due)
	if a.rep != nil || tk.RepeatFlag == "" || got.Unit != "month" || (want != nil && !slices.Equal(got.MonthDays, want)) {
		t.Fatalf("repeat %q (form %+v)", tk.RepeatFlag, r.f)
	}
}

func TestTagSuggest(t *testing.T) {
	a := testApp(t)
	a.st.Upsert(api.Task{ID: "b", ProjectID: "inbox1", Title: "B", Tags: []string{"work", "weekend", "home"}})
	a.taskID = "b"
	a.startEdit("tags") // "#work #weekend #home"
	if s := a.tagSuggest(); s != nil {
		t.Fatalf("suggested for a full tag: %v", s)
	}
	a.in = newInput("#home #w")
	if s := a.tagSuggest(); !slices.Equal(s, []string{"weekend", "work"}) {
		t.Fatalf("got %v", s)
	}
	a.edit = "tags"
	a.editKey(tea.KeyPressMsg{Code: tea.KeyDown})
	a.editKey(tea.KeyPressMsg{Code: tea.KeyTab})
	if v := a.in.value(); v != "#home #work " {
		t.Fatalf("accepted %q", v)
	}
	a.in = newInput("#work wo") // already has it
	if s := a.tagSuggest(); s != nil {
		t.Fatalf("suggested a tag the task has: %v", s)
	}
}

// D keeps a copy in the trash; restoring creates the task again (the API can't undelete).
func TestTrash(t *testing.T) {
	a := testApp(t)
	a.st.Upsert(api.Task{ID: "b", ProjectID: "gone", Title: "B", Status: 2, Items: []api.Item{{ID: "i1", Title: "x"}}})
	a.delID = "b"
	a.deleteKey(tea.KeyPressMsg{Code: 'y', Text: "y"})
	if a.st.Task("b") != nil || len(a.trash) != 1 || len(store.LoadTrash()) != 1 {
		t.Fatalf("not trashed: %v", a.trash)
	}
	a.onOpDone(opDoneMsg{op: a.queue[0]}) // deleted on the server

	a.trashPicker()
	if a.cmd == nil || len(a.cmd.pick.items) != 2 { // B + Empty trash
		t.Fatal("no trash picker")
	}
	a.cmd = nil
	a.restore(a.trash[0])
	f := a.queue[0].fields
	if len(a.trash) != 0 || f["title"] != "B" || f["projectId"] != nil || f["items"].([]api.Item)[0].ID != "" {
		t.Fatalf("restore: trash %v body %v", a.trash, f)
	}
	if nt := a.st.Task(a.taskID); nt == nil || store.Done(nt) {
		t.Fatalf("restored task not open: %+v", nt)
	}

	c := testApp(t)
	c.delID = "a"
	c.deleteKey(tea.KeyPressMsg{Code: 'y', Text: "y"})
	c.onOpDone(opDoneMsg{op: c.queue[0], err: errors.New("boom")})
	if c.st.Task("a") == nil || len(c.trash) != 0 {
		t.Fatalf("failed delete: task %v trash %v", c.st.Task("a"), c.trash)
	}
}

func TestPruneTrash(t *testing.T) {
	now := time.Now()
	ts := []store.Trashed{{Task: api.Task{ID: "new"}, Deleted: now.AddDate(0, 0, -29)}, {Task: api.Task{ID: "old"}, Deleted: now.AddDate(0, 0, -31)}}
	if got := store.PruneTrash(ts, now); len(got) != 1 || got[0].Task.ID != "new" {
		t.Fatalf("got %v", got)
	}
}

// C turns each line of the notes into a checklist item, and back.
func TestConvert(t *testing.T) {
	a := testApp(t)
	task := a.st.Task("a")
	task.Content = "milk\n- [x] eggs\n\n* bread"
	a.convert(task)
	want := []api.Item{{Title: "milk"}, {Title: "eggs", Status: 1}, {Title: "bread"}}
	if task.Kind != "CHECKLIST" || task.Content != "" || !slices.Equal(task.Items, want) {
		t.Fatalf("to checklist: %+v", task)
	}
	if f := a.queue[0].fields; f["kind"] != "CHECKLIST" || f["desc"] != "" {
		t.Fatalf("sent %v", f)
	}
	task.Desc = "shop"
	a.convert(task)
	if task.Kind != "TEXT" || task.Content != "shop\nmilk\neggs\nbread" || task.Items != nil || task.Desc != "" {
		t.Fatalf("to note: %+v", task)
	}
	if items := a.queue[1].fields["items"].([]api.Item); items == nil || len(items) != 0 {
		t.Fatal("items must be sent as [] to clear them")
	}
}

func TestReminders(t *testing.T) {
	a := testApp(t)
	a.th, _ = theme.Load("terminal")
	a.w, a.h = 120, 40
	tk := a.st.Task("a")
	due := time.Now().Add(10 * time.Minute).UTC()
	tk.DueDate, tk.IsAllDay = due.Format(api.DateLayout), false

	// menu: toggling a preset sends the whole list
	a.reminderPicker(tk)
	a.cmd.pick.items[2].run() // 30 minutes early
	if !slices.Equal(tk.Reminders, []string{"TRIGGER:-PT30M"}) || a.cmd == nil {
		t.Fatalf("toggle: %v", tk.Reminders)
	}
	a.cmd = nil

	// Custom: 10 minutes early
	a.openReminderForm(tk)
	a.rem.row = 1
	a.remKey(tea.KeyPressMsg{Code: tea.KeyBackspace})
	a.remKey(tea.KeyPressMsg{Code: tea.KeyBackspace})
	a.remKey(tea.KeyPressMsg{Code: '1', Text: "1"})
	a.remKey(tea.KeyPressMsg{Code: '0', Text: "0"})
	if !strings.Contains(ansi.Strip(func() string { s, _, _ := a.viewReminder(); return s }()), "Remind at") {
		t.Fatal("no preview")
	}
	a.remKey(tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl})
	if !slices.Contains(tk.Reminders, "TRIGGER:-PT10M") {
		t.Fatalf("custom: %v", tk.Reminders)
	}

	// the -10m reminder is due now; notified once, only with notifications on
	a.cfg.Reminders.Notify = "terminal"
	a.remChecked = time.Now().Add(-time.Minute)
	if cmd := a.checkReminders(); cmd == nil || !strings.Contains(a.flash, "🔔") {
		t.Fatalf("not notified: %q", a.flash)
	}
	a.flash = ""
	if a.checkReminders(); a.flash != "" {
		t.Fatal("notified twice")
	}
	if plain("x\x1b]9;y\x07z") != "x]9;yz" {
		t.Fatal("control characters kept")
	}

	// clearing the due date drops the reminders
	a.setDue(tk, nil)
	if tk.Reminders != nil {
		t.Fatalf("reminders kept without a due date: %v", tk.Reminders)
	}
}
