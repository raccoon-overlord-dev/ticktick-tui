package ui

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/raccoon-overlord-dev/ticktick-tui/internal/api"
	"github.com/raccoon-overlord-dev/ticktick-tui/internal/auth"
	"github.com/raccoon-overlord-dev/ticktick-tui/internal/config"
	"github.com/raccoon-overlord-dev/ticktick-tui/internal/store"
)

// p has subtasks k (with its own g) and k2; s is a top-level task below them.
func subApp(t *testing.T) *App {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	config.Path = t.TempDir() + "/config.toml"
	ts := []api.Task{
		{ID: "p", ProjectID: "inbox1", Title: "Parent", Priority: 5},
		{ID: "k", ProjectID: "inbox1", Title: "Kid", ParentID: "p", Priority: 5},
		{ID: "g", ProjectID: "inbox1", Title: "Grandkid", ParentID: "k", Priority: 5},
		{ID: "k2", ProjectID: "inbox1", Title: "Kid 2", ParentID: "p", Priority: 5},
		{ID: "s", ProjectID: "inbox1", Title: "Sibling", Priority: 5},
	}
	st := store.New(nil, nil, ts, "", time.Now())
	cfg := config.Default()
	cfg.Tasks.Sort = config.Sort{GroupBy: "none", SortBy: "title"}
	return &App{cfg: cfg, st: st, signed: &auth.Auth{AccessToken: "fake"}, list: "inbox", idMap: map[string]string{}, w: 160, h: 40}
}

func order(a *App) (out string) {
	for _, g := range a.groups() {
		for i, t := range g.Items {
			out += string(rune('0'+g.Depth[i])) + t.ID + " "
		}
	}
	return
}

// The API doesn't cascade: completing or deleting a parent does it to its subtasks too,
// and undo / restore bring them back.
func TestSubtaskCascade(t *testing.T) {
	a := subApp(t)
	a.taskID = "p"
	a.toggleDone(a.st.Task("p"))
	if len(a.queue) != 4 || !store.Done(a.st.Task("g")) || a.taskID != "s" {
		t.Fatalf("complete: queue=%d g done=%v sel=%s", len(a.queue), store.Done(a.st.Task("g")), a.taskID)
	}
	a.undoDone()
	for _, id := range []string{"p", "k", "g", "k2"} {
		if store.Done(a.st.Task(id)) {
			t.Fatalf("undo left %s done", id)
		}
	}

	b := subApp(t)
	b.askDelete(b.st.Task("p"))
	b.deleteKey(tea.KeyPressMsg{Code: 'y', Text: "y"})
	if len(b.st.Tasks) != 1 || len(b.trash) != 4 {
		t.Fatalf("delete: tasks=%d trash=%d", len(b.st.Tasks), len(b.trash))
	}
	b.restore(b.trash[0]) // the parent, deleted last: its subtasks come back under it
	if got := order(b); len(b.trash) != 0 || len(b.st.Descendants(b.taskID)) != 3 {
		t.Fatalf("restore: trash=%d order=%q", len(b.trash), got)
	}
}

func TestIndentOutdent(t *testing.T) {
	a := subApp(t)
	if got := order(a); got != "0p 1k 2g 1k2 0s " {
		t.Fatalf("tree = %q", got)
	}
	a.indent(a.st.Task("s"), false) // > under p
	if a.st.Task("s").ParentID != "p" || a.queue[0].fields["parentId"] != "p" {
		t.Fatalf("indent: %+v", a.queue[0].fields)
	}
	a.indent(a.st.Task("g"), true) // < next to k
	if a.st.Task("g").ParentID != "p" {
		t.Fatalf("outdent g: parent %q", a.st.Task("g").ParentID)
	}
	a.indent(a.st.Task("k"), true) // < to the top
	if a.st.Task("k").ParentID != "" || a.queue[2].fields["parentId"] != "" {
		t.Fatalf("outdent k: %+v", a.queue[2].fields)
	}
	if got := order(a); got != "0k 0p 1g 1k2 1s " {
		t.Fatalf("tree after = %q", got)
	}
	// the update response leaves parentId out: keep the one sent
	a.onOpDone(opDoneMsg{op: a.queue[0], t: &api.Task{ID: "s", ProjectID: "inbox1", Title: "Sibling"}})
	if a.st.Task("s").ParentID != "p" {
		t.Fatal("parentId lost on the update response")
	}

	a.toggleFold(a.st.Task("g")) // on a subtask: folds the parent
	if got := order(a); got != "0k 0p " || a.taskID != "p" {
		t.Fatalf("fold = %q sel=%s", got, a.taskID)
	}
	a.toggleFold(a.st.Task("p"))
	if got := order(a); got != "0k 0p 1g 1k2 1s " {
		t.Fatalf("unfold = %q", got)
	}
}

// A subtask of a task still being created waits for its id; the create response
// leaves parentId out.
func TestSubtaskOfNewTask(t *testing.T) {
	a := subApp(t)
	a.addTask(api.Task{Title: "New parent", ProjectID: "inbox1"})
	tmp := a.taskID
	a.addTask(api.Task{Title: "New kid", ProjectID: "inbox1", ParentID: tmp})
	kid := a.taskID
	a.onOpDone(opDoneMsg{op: a.queue[0], t: &api.Task{ID: "real", ProjectID: "inbox1", Title: "New parent"}})
	if a.st.Task(kid).ParentID != "real" {
		t.Fatalf("local kid parent = %q", a.st.Task(kid).ParentID)
	}
	a.pump() // what the in-flight create would send
	if a.queue[0].fields["parentId"] != "real" {
		t.Fatalf("create sends parentId %v", a.queue[0].fields["parentId"])
	}
	a.onOpDone(opDoneMsg{op: a.queue[0], t: &api.Task{ID: "real2", ProjectID: "inbox1", Title: "New kid"}})
	if a.st.Task("real2").ParentID != "real" {
		t.Fatal("parentId lost on the create response")
	}
}

// Moving a subtask to another list makes it a normal task; moving a parent takes its subtasks.
func TestMoveSubtask(t *testing.T) {
	a := subApp(t)
	a.st = store.New([]api.Project{{ID: "p2", Name: "Two"}}, nil, a.st.Tasks, "", time.Now())
	a.moveTask(a.st.Task("p"), "p:p2")
	if a.st.Task("g").ProjectID != "p2" || len(a.queue) != 1 {
		t.Fatalf("parent move: g in %s, queue %d", a.st.Task("g").ProjectID, len(a.queue))
	}
	a.moveTask(a.st.Task("k"), "inbox")
	if a.st.Task("k").ParentID != "" || a.queue[len(a.queue)-1].fields["parentId"] != "" {
		t.Fatalf("subtask move kept its parent: %+v", a.queue)
	}
}

// The Parent field's menu offers the list's tasks except t and its subtasks; u undoes.
func TestParentPicker(t *testing.T) {
	a := subApp(t)
	a.parentPicker(a.st.Task("k"))
	var labels []string
	for _, it := range a.cmd.pick.items {
		labels = append(labels, it.label)
	}
	if got := strings.Join(labels, "|"); got != "Open “Parent”|None|Parent|  Kid 2|Sibling" {
		t.Fatalf("items = %q", got)
	}
	a.cmd.pick.items[4].run() // under Sibling
	if a.st.Task("k").ParentID != "s" {
		t.Fatalf("parent = %q", a.st.Task("k").ParentID)
	}

	// < again right after undoes it, as x does after completing
	b := subApp(t)
	b.indent(b.st.Task("g"), true)
	if b.st.Task("g").ParentID != "p" || !strings.HasSuffix(b.flash, "< to undo") {
		t.Fatalf("outdent: parent %q flash %q", b.st.Task("g").ParentID, b.flash)
	}
	b.indent(b.st.Task("g"), true)
	if b.st.Task("g").ParentID != "k" || len(b.queue) != 2 {
		t.Fatalf("undo: parent %q queue %d", b.st.Task("g").ParentID, len(b.queue))
	}
	b.indent(b.st.Task("g"), true) // the message is gone: a new outdent
	if b.st.Task("g").ParentID != "p" {
		t.Fatalf("after undo: parent %q", b.st.Task("g").ParentID)
	}
}
