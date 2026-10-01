package ui

import (
	"fmt"
	"slices"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"ttui/internal/api"
	"ttui/internal/parse"
	"ttui/internal/store"
)

// cmdBar is the centered command bar (handoff §2). The query's first character sets the mode.
type cmdBar struct {
	in  textInput
	idx int
	off int
}

type cmdItem struct {
	icon, iconRole string
	label          string
	pos            []int // fuzzy-matched rune indexes
	hint           string
	run            func() tea.Cmd
	add            *parse.Add // quick-add preview
}

type cmdGroup struct {
	label string
	items []cmdItem
}

func (a *App) openCmd(q string) {
	a.cmd, a.settings = &cmdBar{in: newInput(q)}, false
}

// cmdMode returns the mode ("search", "cmd", "add", "list", "tag") and the query without its prefix.
func cmdMode(q string) (string, string) {
	if q == "" {
		return "search", ""
	}
	switch q[0] {
	case '>':
		return "cmd", strings.TrimSpace(q[1:])
	case '+':
		return "add", strings.TrimSpace(q[1:])
	case '@':
		return "list", strings.TrimSpace(q[1:])
	case '#':
		return "tag", strings.TrimSpace(q[1:])
	}
	return "search", strings.TrimSpace(q)
}

var modeLabel = map[string][2]string{
	"search": {"search", "info"}, "cmd": {"command", "secondary"}, "add": {"quick add", "ok"},
	"list": {"lists", "accent"}, "tag": {"tags", "secondary"},
}

func (a *App) taskItem(t *api.Task, pos []int) cmdItem {
	list := "Inbox"
	if p := a.st.Project(t.ProjectID); p != nil {
		list = p.Name
	}
	it := cmdItem{icon: "●", iconRole: store.PrioRole(t.Priority), label: t.Title, pos: pos, hint: list}
	if store.Done(t) {
		it.icon, it.iconRole = "✓", "dim"
	}
	id := t.ID
	it.run = func() tea.Cmd { a.gotoTask(id); return nil }
	return it
}

func (a *App) allLists() []string {
	ids := []string{"inbox", "today", "tomorrow", "next7"}
	for _, p := range a.st.Projects {
		ids = append(ids, "p:"+p.ID)
	}
	return append(ids, "f-high", "f-nodate", "completed")
}

func (a *App) listItem(id string, pos []int) cmdItem {
	name, icon, role := a.listMeta(id)
	hint := fmt.Sprint(a.st.OpenCount(id, a.now))
	if p := a.st.Project(strings.TrimPrefix(id, "p:")); p != nil && strings.HasPrefix(id, "p:") {
		if g := a.st.Group(p.GroupID); g != nil {
			hint = g.Name + " · " + hint
		}
	}
	return cmdItem{icon: icon, iconRole: role, label: name, pos: pos, hint: hint, run: func() tea.Cmd { a.gotoList(id); return nil }}
}

func (a *App) commands() []cmdItem {
	c := a.cfg
	cur := func(v, want string) string {
		if v == want {
			return "current"
		}
		return ""
	}
	items := []cmdItem{
		{icon: a.icon("", ","), label: "Open settings", hint: ",", run: func() tea.Cmd { a.settings, a.sIdx = true, 1; return nil }},
		{icon: a.icon("", "t"), label: "Toggle due-date labels (" + onOff(c.Tasks.DueLabel) + ")", hint: "t", run: func() tea.Cmd { a.toggleDueLabels(); return nil }},
	}
	for _, th := range []string{"terminal", "colorful", "lotr"} {
		items = append(items, cmdItem{icon: a.icon("", "~"), label: "Theme: " + th, hint: cur(c.Appearance.Theme, th),
			run: func() tea.Cmd { return a.setOption("theme", th) }})
	}
	for _, l := range [][2]string{{"auto", "Layout: auto (by width)"}, {"3", "Layout: 3 columns"}, {"2", "Layout: 2 columns"}, {"1", "Layout: 1 column"}} {
		items = append(items, cmdItem{icon: a.icon("", "|"), label: l[1], hint: cur(c.Layout.Columns, l[0]),
			run: func() tea.Cmd { return a.setOption("columns", l[0]) }})
	}
	for _, b := range []string{"solid", "transparent", "blur"} {
		items = append(items, cmdItem{icon: a.icon("", "~"), label: "Background: " + b, hint: cur(c.Appearance.Background, b),
			run: func() tea.Cmd { return a.setOption("bg", b) }})
	}
	shown := map[bool]string{true: "shown", false: "hidden"}[c.Layout.ShowCompleted]
	items = append(items,
		cmdItem{icon: a.icon("", "✓"), label: "Toggle completed tasks (" + shown + ")", run: func() tea.Cmd {
			return a.setOption("completed", onOff(!c.Layout.ShowCompleted))
		}},
		cmdItem{icon: a.icon("", "⟳"), label: "Sync now", hint: "ctrl+r", run: a.syncNow},
		cmdItem{icon: a.icon("", "<"), label: "Sign out", run: a.signOut},
		cmdItem{icon: a.icon("", "q"), label: "Quit ttui", hint: "q", run: func() tea.Cmd { return tea.Quit }},
	)
	for i := range items {
		items[i].iconRole = "secondary"
	}
	return items
}

// fuzzyPick filters and ranks labels by fuzzy score.
func fuzzyPick[T any](xs []T, label func(T) string, term string, mk func(T, []int) cmdItem) []cmdItem {
	type hit struct {
		x     T
		score int
		pos   []int
	}
	var hits []hit
	for _, x := range xs {
		if s, pos, ok := parse.Fuzzy(label(x), term); ok {
			hits = append(hits, hit{x, s, pos})
		}
	}
	slices.SortStableFunc(hits, func(a, b hit) int { return a.score - b.score })
	var out []cmdItem
	for _, h := range hits {
		out = append(out, mk(h.x, h.pos))
	}
	return out
}

func first[T any](xs []T, n int) []T { return xs[:min(n, len(xs))] }

func (a *App) quickAdd(term string) parse.Add {
	lists := []parse.List{{ID: "inbox", Name: "Inbox"}}
	for _, p := range a.st.Projects {
		lists = append(lists, parse.List{ID: "p:" + p.ID, Name: p.Name})
	}
	def := "inbox"
	if strings.HasPrefix(a.list, "p:") {
		def = a.list
	}
	var defDue *parse.Due
	switch a.list {
	case "today":
		defDue = &parse.Due{}
	case "tomorrow":
		defDue = &parse.Due{Day: 1}
	}
	return parse.QuickAdd(term, lists, def, defDue, time.Now())
}

func (a *App) cmdData() (string, []cmdGroup) {
	mode, term := cmdMode(a.cmd.in.value())
	var gs []cmdGroup
	tasks := make([]*api.Task, 0, len(a.st.Tasks))
	for i := range a.st.Tasks { // open tasks rank before done ones
		if !store.Done(&a.st.Tasks[i]) {
			tasks = append(tasks, &a.st.Tasks[i])
		}
	}
	for i := range a.st.Tasks {
		if store.Done(&a.st.Tasks[i]) {
			tasks = append(tasks, &a.st.Tasks[i])
		}
	}
	title := func(t *api.Task) string { return t.Title }

	switch mode {
	case "search":
		if term == "" {
			var recent []cmdItem
			for _, id := range a.recent {
				if t := a.st.Task(id); t != nil {
					recent = append(recent, a.taskItem(t, nil))
				}
			}
			if len(recent) == 0 {
				for _, t := range first(navTasks(a.groups()), 3) {
					recent = append(recent, a.taskItem(t, nil))
				}
			}
			gs = append(gs, cmdGroup{"RECENT", recent}, cmdGroup{"TRY", []cmdItem{
				{icon: "+", iconRole: "ok", label: "Quick add a task", hint: "+", run: func() tea.Cmd { a.openCmd("+ "); return nil }},
				{icon: "@", iconRole: "accent", label: "Jump to a list", hint: "@", run: func() tea.Cmd { a.openCmd("@"); return nil }},
				{icon: "#", iconRole: "secondary", label: "Filter by tag", hint: "#", run: func() tea.Cmd { a.openCmd("#"); return nil }},
				{icon: ">", iconRole: "secondary", label: "Run a command", hint: ">", run: func() tea.Cmd { a.openCmd(">"); return nil }},
			}})
			break
		}
		gs = append(gs,
			cmdGroup{"TASKS", first(fuzzyPick(tasks, title, term, a.taskItem), 6)},
			cmdGroup{"LISTS", first(fuzzyPick(a.allLists(), func(id string) string { n, _, _ := a.listMeta(id); return n }, term, a.listItem), 3)},
			cmdGroup{"COMMANDS", first(fuzzyPick(a.commands(), func(c cmdItem) string { return c.label }, term,
				func(c cmdItem, pos []int) cmdItem { c.pos = pos; return c }), 3)})
		if len(gs[0].items)+len(gs[1].items)+len(gs[2].items) == 0 {
			add := a.quickAdd(term)
			gs = append(gs, cmdGroup{"NO MATCHES", []cmdItem{{icon: "+", iconRole: "ok", label: "Add “" + term + "” as a task",
				run: func() tea.Cmd { return a.createTask(add) }}}})
		}
	case "cmd":
		gs = append(gs, cmdGroup{"COMMANDS", fuzzyPick(a.commands(), func(c cmdItem) string { return c.label }, term,
			func(c cmdItem, pos []int) cmdItem { c.pos = pos; return c })})
	case "list":
		gs = append(gs, cmdGroup{"LISTS", fuzzyPick(a.allLists(), func(id string) string { n, _, _ := a.listMeta(id); return n }, term, a.listItem)})
	case "tag":
		gs = append(gs, cmdGroup{"TAGS", fuzzyPick(a.st.Tags, func(t string) string { return t }, term,
			func(t string, pos []int) cmdItem { return a.listItem("tag:"+t, pos) })})
	case "add":
		add := a.quickAdd(term)
		label := add.Title
		if label == "" {
			label = "…"
		}
		gs = append(gs, cmdGroup{"QUICK ADD", []cmdItem{{label: label, add: &add, run: func() tea.Cmd { return a.createTask(add) }}}})
	}
	return mode, slices.DeleteFunc(gs, func(g cmdGroup) bool { return len(g.items) == 0 })
}

func (a *App) cmdKey(k tea.KeyPressMsg) tea.Cmd {
	_, gs := a.cmdData()
	var items []cmdItem
	for _, g := range gs {
		items = append(items, g.items...)
	}
	switch k.String() {
	case "esc":
		a.cmd = nil
	case "down", "tab", "ctrl+n", "ctrl+j":
		a.cmd.idx = min(a.cmd.idx+1, max(len(items)-1, 0))
	case "up", "shift+tab", "ctrl+p", "ctrl+k":
		a.cmd.idx = max(a.cmd.idx-1, 0)
	case "enter":
		if len(items) == 0 {
			return nil
		}
		it := items[min(a.cmd.idx, len(items)-1)]
		a.cmd = nil
		return it.run()
	default:
		if a.cmd.in.key(k, false) {
			a.cmd.idx = 0
		}
	}
	return nil
}

func (a *App) gotoList(id string) {
	a.list, a.sideKey, a.taskID, a.focus, a.sheet, a.offTasks = id, "l:"+id, "", "tasks", false, 0
}

func (a *App) gotoTask(id string) {
	t := a.st.Task(id)
	if t == nil {
		return
	}
	if !slices.Contains(navTasks(a.groups()), t) {
		list := "p:" + t.ProjectID
		switch {
		case store.Done(t):
			list = "completed"
		case store.IsInbox(t.ProjectID):
			list = "inbox"
		}
		a.gotoList(list)
	}
	a.taskID, a.focus, a.sheet = id, "tasks", false
	a.remember(id)
}

// viewCmd renders the command bar; it returns the box and its size.
func (a *App) viewCmd() (string, int, int) {
	p := a.overlayPen()
	w := min(78, a.w-4)
	mode, gs := a.cmdData()
	ml := modeLabel[mode]

	// input row
	q := a.cmd.in.value()
	input := a.cmd.in.view(p, "text", w-4-2-len(ml[0])-2)
	if q == "" {
		input = p.s("text").Reverse(true).Render(" ") + p.s("dim").Render("search tasks · > command · + add · @ list · # tag")
	}
	lines := []string{p.line(w, p.s("accent").Bold(true).Render("❯")+p.sp(1)+input, p.s(ml[1]).Render(ml[0])),
		p.s("line").Render(strings.Repeat("─", w-2))}

	var body []string
	sel, n := -1, 0
	for gi, g := range gs {
		if gi > 0 {
			body = append(body, "")
		}
		body = append(body, p.line(w, p.s("dim").Render(g.label), ""))
		for _, it := range g.items {
			on := n == a.cmd.idx
			n++
			rp := p
			if on {
				sel, rp = len(body), p.on(a.selBg(true))
			}
			if it.add != nil {
				body = append(body, a.addRows(rp, p, it, w)...)
				continue
			}
			labelRole := "text"
			if on {
				labelRole = "accent"
			}
			hint := ""
			if it.hint != "" {
				hint = rp.s("muted").Render(it.hint)
			}
			room := w - 4 - 2 - lipgloss.Width(hint) - 1
			body = append(body, rp.line(w, rp.s(it.iconRole).Render(it.icon)+rp.sp(1)+a.highlight(rp, trunc(it.label, room), it.pos, labelRole), hint))
		}
	}
	if len(gs) == 0 {
		body = append(body, p.line(w, p.s("dim").Render("NO MATCHES"), ""))
	}

	footer := []string{p.s("line").Render(strings.Repeat("─", w-2))}
	var legend []string
	for _, pf := range [][2]string{{">", "command"}, {"+", "add"}, {"@", "list"}, {"#", "tag"}} {
		role := "dim"
		if q != "" && q[:1] == pf[0] {
			role = "accent"
		}
		legend = append(legend, p.s(role).Render(pf[0]+" "+pf[1]))
	}
	footer = append(footer, p.line(w, p.s("dim").Render("↑↓ select · ⏎ run · esc close"), strings.Join(legend, p.sp(2))))

	maxBody := max(a.h*76/100-2-len(lines)-len(footer), 3)
	body = window(body, sel, maxBody, &a.cmd.off)
	all := append(append(lines, body...), footer...)
	h := len(all) + 2
	return a.frame(p, frameOpts{w: w, h: h, body: all}), w, h
}

// addRows is the quick-add preview: "+ title", its chips, and the syntax hint.
func (a *App) addRows(rp, p pen, it cmdItem, w int) []string {
	add := it.add
	chip := func(set bool, role, s string) string {
		if !set {
			role = "dim"
		}
		return rp.s(role).Render(s)
	}
	due := "no date"
	if add.Due != nil {
		t := api.Task{DueDate: add.Due.At(time.Now()).Format(api.DateLayout), IsAllDay: !add.Due.HasTime}
		if d := store.DueLabel(&t, time.Now()); d != nil {
			due = strings.ToLower(d.Long)
		}
	}
	name, _, _ := a.listMeta(add.List)
	chips := []string{
		chip(add.PrioSet, store.PrioRole(add.Prio), "● "+strings.ToLower(store.PrioName(add.Prio))),
		chip(add.DueSet, "secondary", a.icon(" ", "")+due),
	}
	for _, t := range add.Tags {
		chips = append(chips, rp.s("secondary").Render("#"+t))
	}
	chips = append(chips, chip(add.ListSet, "accent", "@"+name))
	return []string{
		rp.line(w, rp.s("ok").Render("+")+rp.sp(1)+rp.s("text").Bold(true).Render(trunc(it.label, w-6)), ""),
		rp.line(w, rp.sp(2)+strings.Join(chips, rp.sp(2)), ""),
		"",
		p.line(w, p.s("dim").Render("!high !med !low · #tag · @list · today tomorrow fri · 17:00"), ""),
	}
}

// highlight renders label with the runes at pos in accent + bold + underline.
func (a *App) highlight(p pen, label string, pos []int, role string) string {
	var b strings.Builder
	for i, r := range []rune(label) {
		if slices.Contains(pos, i) {
			b.WriteString(p.s("accent").Bold(true).Underline(true).Render(string(r)))
		} else {
			b.WriteString(p.s(role).Render(string(r)))
		}
	}
	return b.String()
}
