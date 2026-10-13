package ui

import (
	"fmt"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/raccoon-overlord-dev/ticktick-tui/internal/api"
	"github.com/raccoon-overlord-dev/ticktick-tui/internal/store"
)

// Subtasks are tasks with a parentId (see docs/api-notes.md). The tasks pane shows them
// indented under their parent (store.TaskGroups); z folds them (Z all), > and < change the level,
// A opens the New task panel for one.

// row finds t in the tasks pane: its group and index there.
func (a *App) row(t *api.Task) (store.Group, int, bool) {
	for _, g := range a.groups() {
		if i := slices.Index(g.Items, t); i >= 0 {
			return g, i, true
		}
	}
	return store.Group{}, -1, false
}

// above is the index of the nearest row above i at depth d, or -1 if a row less deep
// (the parent) comes first.
func above(g store.Group, i, d int) int {
	for j := i - 1; j >= 0; j-- {
		if g.Depth[j] < d {
			return -1
		}
		if g.Depth[j] == d {
			return j
		}
	}
	return -1
}

// parentRow is the index of the row i is a subtask of, or -1 at the top level.
func parentRow(g store.Group, i int) int {
	for j := i - 1; j >= 0; j-- {
		if g.Depth[j] < g.Depth[i] {
			return j
		}
	}
	return -1
}

// toggleFold hides or shows t's subtasks; on a subtask without its own, it folds the parent.
func (a *App) toggleFold(t *api.Task) {
	if a.folded(t.ID) {
		a.setFolded(t.ID, false)
		return
	}
	g, i, ok := a.row(t)
	switch {
	case !ok:
	case i+1 < len(g.Items) && g.Depth[i+1] > g.Depth[i]:
		a.setFolded(t.ID, true)
	case parentRow(g, i) >= 0:
		p := g.Items[parentRow(g, i)]
		a.setFolded(p.ID, true)
		a.taskID = p.ID
	default:
		a.setFlash("no subtasks to fold")
	}
}

// toggleFoldAll folds every task with subtasks in the current list (Z), or unfolds them all
// when they're already folded. The cursor moves up to the top-level task it was under.
func (a *App) toggleFoldAll() {
	var parents []string
	fold := false
	for _, g := range a.st.TaskGroups(a.st.TasksFor(a.list, a.now), a.sortFor(a.list), a.cfg.Layout.ShowCompleted || a.list == "completed", a.now) {
		for i, t := range g.Items {
			if i+1 < len(g.Items) && g.Depth[i+1] > g.Depth[i] {
				parents = append(parents, t.ID)
				fold = fold || !a.folded(t.ID)
			}
		}
	}
	if len(parents) == 0 {
		a.setFlash("no subtasks to fold")
		return
	}
	if fold {
		if t := a.selTask(); t != nil {
			for p, n := a.st.Parent(t), 0; p != nil && n < 100; p, n = a.st.Parent(p), n+1 {
				a.taskID = p.ID
			}
		}
	}
	f := slices.DeleteFunc(a.cfg.Layout.Folded, func(id string) bool { return slices.Contains(parents, id) })
	if fold {
		f = append(f, parents...)
	}
	a.cfg.Layout.Folded = f
	a.save()
	a.setFlash(map[bool]string{true: "folded all subtasks · Z to unfold", false: "unfolded all subtasks"}[fold])
}

func (a *App) setFolded(id string, on bool) {
	f := &a.cfg.Layout.Folded
	if i := slices.Index(*f, id); i >= 0 && !on {
		*f = slices.Delete(*f, i, i+1)
	} else if i < 0 && on {
		*f = append(*f, id)
	} else {
		return
	}
	a.save()
}

// unfoldAbove shows the folded parents of t, so the cursor can go to it.
func (a *App) unfoldAbove(t *api.Task) {
	for p, n := a.st.Parent(t), 0; p != nil && n < 100; p, n = a.st.Parent(p), n+1 {
		a.setFolded(p.ID, false)
	}
}

// indent makes t a subtask of the task above it at its level (>), or with out (<) moves it
// up a level, next to its parent. Pressing the same key again right after undoes it, as x
// does after completing.
func (a *App) indent(t *api.Task, out bool) tea.Cmd {
	key := map[bool]string{false: ">", true: "<"}[out]
	if cmd, ok := a.undoParent(t, key); ok {
		return cmd
	}
	if store.Done(t) {
		a.setFlash("reopen it first")
		return nil
	}
	g, i, ok := a.row(t)
	if !ok {
		return nil
	}
	if out {
		pi := parentRow(g, i)
		if pi < 0 {
			a.setFlash("already a top-level task")
			return nil
		}
		var parent *api.Task
		if gi := parentRow(g, pi); gi >= 0 {
			parent = g.Items[gi]
		}
		return a.setParent(t, parent, key)
	}
	j := above(g, i, g.Depth[i])
	if j < 0 {
		a.setFlash("no task above at this level to nest it under")
		return nil
	}
	if g.Items[j].ProjectID != t.ProjectID {
		a.setFlash("the task above is in another list")
		return nil
	}
	return a.setParent(t, g.Items[j], key)
}

// setParent makes t a subtask of p, or a top-level task with p nil. With an undo key, that
// key undoes it while the message shows.
func (a *App) setParent(t, p *api.Task, undo string) tea.Cmd {
	id, msg := "", "→ top-level task"
	if p != nil {
		id, msg = p.ID, fmt.Sprintf("→ subtask of “%s”", p.Title)
		a.setFolded(p.ID, false)
	}
	if t == a.draft { // sent with the create
		if p != nil {
			t.ProjectID = p.ProjectID
		}
		t.ParentID = id
		return nil
	}
	if id == t.ParentID {
		return nil
	}
	a.parentUndo = [2]string{}
	if undo != "" {
		a.parentUndo = [2]string{t.ID, t.ParentID}
		msg += " · " + undo + " to undo"
	}
	a.setFlash(msg)
	return a.update(t, map[string]any{"parentId": id}, func(t *api.Task) { t.ParentID = id })
}

// undoParent puts back t's parent if it was the task changed last and its "<key> to undo"
// message still shows.
func (a *App) undoParent(t *api.Task, key string) (tea.Cmd, bool) {
	id, old := a.parentUndo[0], a.parentUndo[1]
	if real, ok := a.idMap[id]; ok {
		id = real
	}
	if id != t.ID || !strings.HasSuffix(a.flash, " "+key+" to undo") {
		return nil, false
	}
	a.parentUndo = [2]string{}
	cmd := a.update(t, map[string]any{"parentId": old}, func(t *api.Task) { t.ParentID = old })
	a.unfoldAbove(t)
	a.taskID = t.ID
	a.setFlash("undone")
	return cmd, true
}

// parentPicker opens the Parent menu (⏎ on the Parent field): the open tasks of t's list as
// a tree, None for a top-level task and, on a subtask, a way to open its parent.
func (a *App) parentPicker(t *api.Task) {
	cur := a.st.Parent(t)
	var items []cmdItem
	if cur != nil && t != a.draft {
		items = append(items, cmdItem{icon: "↑", iconRole: "accent", label: "Open “" + cur.Title + "”", group: "parent",
			run: func() tea.Cmd { a.openTask(cur.ID); return nil }})
	}
	none := cmdItem{icon: "×", iconRole: "dim", label: "None", hint: "top-level task", group: "subtask of",
		run: func() tea.Cmd { return a.setParent(t, nil, "") }}
	if cur == nil {
		none.hint += " · current"
	}
	items = append(items, none)
	skip := map[string]bool{t.ID: true} // a task can't go under itself or its own subtasks
	for _, d := range a.st.Descendants(t.ID) {
		skip[d.ID] = true
	}
	list := listID(t.ProjectID)
	for _, g := range a.st.TaskGroups(a.st.TasksFor(list, a.now), a.sortFor(list), false, a.now) {
		for i, c := range g.Items {
			if skip[c.ID] {
				continue
			}
			it := cmdItem{icon: "●", iconRole: store.PrioRole(c.Priority), label: strings.Repeat("  ", min(g.Depth[i], 6)) + c.Title,
				group: "subtask of", run: func() tea.Cmd { return a.setParent(t, c, "") }}
			if cur != nil && c.ID == cur.ID {
				it.hint = "current"
			}
			items = append(items, it)
		}
	}
	a.openPick("parent", items)
}

// openSubDraft opens the New task panel for a subtask of the selected task (A).
func (a *App) openSubDraft() {
	p := a.selTask()
	switch {
	case p == nil:
		a.setFlash("select a task first")
		return
	case store.Done(p):
		a.setFlash("reopen it first · a completed task can't take subtasks")
		return
	}
	a.openDraft()
	a.draft.ProjectID, a.draft.ParentID = p.ProjectID, p.ID
	a.draftInit = store.Clone(*a.draft)
	a.setFolded(p.ID, false)
}

// openTask selects the task with id and opens its details (⏎ on a parent or subtask row).
func (a *App) openTask(id string) {
	n, _, _, _ := layout(a.w, a.cfg.Layout.Columns)
	a.gotoTask(id)
	a.openDetail(n)
}

// pruneFolded forgets folded tasks that are gone.
func (a *App) pruneFolded() {
	f := slices.DeleteFunc(slices.Clone(a.cfg.Layout.Folded), func(id string) bool { return a.st.Task(id) == nil })
	if len(f) != len(a.cfg.Layout.Folded) {
		a.cfg.Layout.Folded = f
		a.save()
	}
}
