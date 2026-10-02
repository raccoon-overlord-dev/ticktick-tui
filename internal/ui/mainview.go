package ui

import (
	"fmt"
	"image/color"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"ttui/internal/api"
	"ttui/internal/config"
	"ttui/internal/parse"
	"ttui/internal/store"
)

// ---- lists pane ----

type sideItem struct {
	header   bool   // section header row ("" label = just a blank row)
	key      string // "l:<list>" or "f:<groupId>"
	list     string
	folder   string
	tree     string // "├" / "└" for folder children
	icon     string
	iconRole string
	label    string
	count    int
	active   bool
}

func (a *App) listMeta(id string) (name, icon, role string) {
	switch id {
	case "inbox":
		return "Inbox", a.icon("", "*"), "ok"
	case "today":
		return "Today", a.icon("", "*"), "warn"
	case "tomorrow":
		return "Tomorrow", a.icon("", "*"), "secondary"
	case "next7":
		return "Next 7 Days", a.icon("", "*"), "info"
	case "f-high":
		return "High · this week", a.icon("", "~"), "info"
	case "f-nodate":
		return "No due date", a.icon("", "~"), "info"
	case "completed":
		return "Completed", a.icon("", "✓"), "ok"
	}
	if tag, ok := strings.CutPrefix(id, "tag:"); ok {
		return tag, a.icon("", "#"), "secondary"
	}
	if p := a.st.Project(strings.TrimPrefix(id, "p:")); p != nil {
		return p.Name, a.icon("", "-"), "sub"
	}
	return id, a.icon("", "-"), "sub"
}

func (a *App) sideItems() []sideItem {
	var rows []sideItem
	sec := func(label string) { rows = append(rows, sideItem{header: true, label: label}) }
	item := func(id, tree string) sideItem {
		name, icon, role := a.listMeta(id)
		return sideItem{key: "l:" + id, list: id, tree: tree, icon: icon, iconRole: role, label: name,
			count: a.st.OpenCount(id, a.now), active: id == a.list}
	}

	sec("SMART")
	for _, id := range []string{"inbox", "today", "tomorrow", "next7"} {
		rows = append(rows, item(id, ""))
	}

	sec("LISTS")
	emitted := map[string]bool{}
	for _, p := range a.st.Projects {
		g := a.st.Group(p.GroupID)
		if g == nil {
			rows = append(rows, item("p:"+p.ID, ""))
			continue
		}
		if emitted[g.ID] {
			continue
		}
		emitted[g.ID] = true
		var kids []api.Project
		for _, q := range a.st.Projects {
			if q.GroupID == g.ID {
				kids = append(kids, q)
			}
		}
		open := a.folderOpen(g.ID)
		f := sideItem{key: "f:" + g.ID, folder: g.ID, icon: a.icon("", "▾"), iconRole: "accent", label: g.Name}
		if !open {
			f.icon = a.icon("", "▸")
			for _, q := range kids {
				f.count += a.st.OpenCount("p:"+q.ID, a.now)
				f.active = f.active || "p:"+q.ID == a.list
			}
		}
		rows = append(rows, f)
		if open {
			for i, q := range kids {
				tree := "├"
				if i == len(kids)-1 {
					tree = "└"
				}
				rows = append(rows, item("p:"+q.ID, tree))
			}
		}
	}

	if len(a.st.Tags) > 0 {
		sec("TAGS")
		for _, t := range a.st.Tags {
			rows = append(rows, item("tag:"+t, ""))
		}
	}
	sec("FILTERS")
	rows = append(rows, item("f-high", ""), item("f-nodate", ""))
	sec("")
	c := item("completed", "")
	c.count = 0 // the design shows no count on Completed
	rows = append(rows, c)
	return rows
}

func (a *App) listsLines(p pen, w int, focused bool) ([]string, int) {
	var lines []string
	sel := -1
	for i, r := range a.sideItems() {
		if r.header {
			if i > 0 {
				lines = append(lines, "")
			}
			if r.label != "" {
				lines = append(lines, p.line(w, p.s("dim").Render(r.label), ""))
			}
			continue
		}
		cur := r.key == a.sideKey
		rp := p
		if cur {
			sel = len(lines)
			rp = p.on(a.selBg(focused))
		}
		labelRole := "text"
		if (cur && focused) || r.active {
			labelRole = "accent"
		}
		prefix := ""
		if r.tree != "" {
			prefix = rp.s("dim").Render(r.tree) + rp.sp(1)
		}
		count := ""
		if r.count > 0 {
			cs := rp.s("dim")
			if r.active {
				cs = rp.s("accent").Bold(true)
			}
			count = cs.Render(fmt.Sprint(r.count))
		}
		room := w - 4 - lipgloss.Width(prefix) - 2 - lipgloss.Width(count) - 1
		ls := rp.s(labelRole).Bold(r.active)
		left := prefix + rp.s(r.iconRole).Render(r.icon) + rp.sp(1) + ls.Render(trunc(r.label, room))
		lines = append(lines, rp.line(w, left, count))
	}
	return lines, sel
}

// ---- tasks pane ----

func (a *App) groups() []store.Group {
	ts := a.st.TasksFor(a.list, a.now)
	if a.list == "completed" {
		if len(ts) == 0 {
			return nil
		}
		return []store.Group{{Label: "Completed", Role: "ok", Done: true, Items: ts}}
	}
	return a.st.TaskGroups(ts, a.sortFor(a.list), a.cfg.Layout.ShowCompleted, a.now)
}

// sortFor is the list's own sort (set with s), or the default from Settings.
func (a *App) sortFor(list string) config.Sort {
	if so, ok := a.cfg.Tasks.ListSort[list]; ok {
		return so
	}
	return a.cfg.Tasks.Sort
}

func navTasks(gs []store.Group) []*api.Task {
	var out []*api.Task
	for _, g := range gs {
		out = append(out, g.Items...)
	}
	return out
}

// selTask keeps the selection by id and falls back to the first task.
func (a *App) selTask() *api.Task {
	nav := navTasks(a.groups())
	for _, t := range nav {
		if t.ID == a.taskID {
			return t
		}
	}
	if len(nav) > 0 {
		return nav[0]
	}
	return nil
}

func (a *App) tasksLines(p pen, w int, focused bool) (lines []string, sel, pos, total int) {
	gs := a.groups()
	cur := a.selTask()
	sel = -1
	if len(gs) == 0 {
		return []string{p.line(w, p.s("dim").Render("Nothing here · a to add a task"), "")}, -1, 0, 0
	}
	for gi, g := range gs {
		if gi > 0 {
			lines = append(lines, "")
		}
		if g.Label != "" { // Group by: none
			lines = append(lines, a.groupHeader(p, g, w))
		}
		for _, t := range g.Items {
			total++
			isSel := cur != nil && t.ID == cur.ID
			if isSel {
				sel, pos = len(lines), total
			}
			lines = append(lines, a.taskLine(p, t, w, isSel, focused))
		}
	}
	return lines, sel, pos, total
}

func (a *App) groupHeader(p pen, g store.Group, w int) string {
	dot := "●"
	if g.Done {
		dot = "✓"
	}
	count := fmt.Sprint(len(g.Items))
	switch a.cfg.Appearance.PriorityHeaders {
	case "label":
		return p.line(w, p.s(g.Role).Bold(true).Render(g.Label)+p.sp(1)+p.s("dim").Render(count), "")
	case "tab":
		tp := p.on(tint(a.th, g.Role, 0.14))
		return tp.line(w, tp.s(g.Role).Bold(true).Render(dot+" "+g.Label), tp.s(g.Role).Bold(true).Render(count))
	}
	left := p.s(g.Role).Bold(true).Render(dot+" "+strings.ToUpper(g.Label)) + p.sp(1)
	fill := max(w-4-lipgloss.Width(left)-len(count)-1, 0)
	return p.line(w, left+p.s("line").Render(strings.Repeat("─", fill))+p.sp(1), p.s("dim").Render(count))
}

func (a *App) taskLine(p pen, t *api.Task, w int, sel, focused bool) string {
	rp := p
	if sel {
		rp = p.on(a.selBg(focused))
	}
	done := store.Done(t)
	dot := rp.s(store.PrioRole(t.Priority)).Render("●")
	if done {
		dot = rp.s("dim").Render("✓")
	}

	var meta []string
	if n := len(t.Items); n > 0 {
		d := 0
		for _, it := range t.Items {
			if it.Status != 0 {
				d++
			}
		}
		meta = append(meta, fmt.Sprintf("%d/%d", d, n))
	}
	if t.RepeatFlag != "" {
		meta = append(meta, "↻")
	}
	right := ""
	if len(meta) > 0 {
		right = rp.s("dim").Render(strings.Join(meta, " "))
	}
	if a.cfg.Tasks.DueLabel {
		if d := store.DueLabel(t, a.now); d != nil {
			role := d.Role
			if done {
				role = "dim"
			}
			if right != "" {
				right += rp.sp(1)
			}
			right += rp.s(role).Render(d.Short)
		}
	}

	titleRole := "text"
	switch {
	case sel && focused:
		titleRole = "accent"
	case done:
		titleRole = "dim"
	}
	room := w - 4 - 2 - lipgloss.Width(right) - 1
	title := rp.s(titleRole).Strikethrough(done).Render(trunc(t.Title, room))
	return rp.line(w, dot+rp.sp(1)+title, right)
}

// ---- details pane ----

func detailKeys(t *api.Task) []string {
	k := []string{"title", "due", "repeat", "list", "tags", "priority"}
	for i := range t.Items {
		k = append(k, fmt.Sprintf("c%d", i))
	}
	return append(k, "additem", "notes")
}

func (a *App) detailLines(p pen, t *api.Task, w int, focused bool) ([]string, int) {
	if t == nil {
		return []string{p.line(w, p.s("dim").Render("No task selected"), "")}, -1
	}
	keys := detailKeys(t)
	cw := w - 4
	var lines []string
	sel := -1
	pick := func(k string) pen {
		if focused && a.df < len(keys) && keys[a.df] == k {
			sel = len(lines)
			return p.on(a.selBg(true))
		}
		return p
	}
	done := store.Done(t)
	editing := func(k string) bool { return a.edit == k && a.editID == t.ID }

	// title
	tp := pick("title")
	dot := tp.s(store.PrioRole(t.Priority)).Render("●")
	if done {
		dot = tp.s("ok").Render("✓")
	}
	if editing("title") {
		lines = append(lines, tp.line(w, dot+tp.sp(1)+a.in.view(tp, "text", cw-2), ""))
	}
	for i, l := range parse.Wrap([]parse.Span{{Text: t.Title}}, cw-2) {
		if editing("title") {
			break
		}
		lead := dot + tp.sp(1)
		if i > 0 {
			lead = tp.sp(2)
		}
		lines = append(lines, tp.line(w, lead+tp.s("text").Bold(true).Strikethrough(done).Render(l[0].Text), ""))
	}
	lines = append(lines, "")

	// fields
	placeholder := map[string]string{"due": "e.g. tomorrow 17:00", "tags": "#tag #another"}
	field := func(k, label, value, role string) {
		fp := pick(k)
		v := fp.s(role).Render(trunc(value, cw-9))
		if editing(k) {
			v = a.in.view(fp, "text", cw-9)
			if a.in.value() == "" {
				v += fp.s("dim").Render(placeholder[k])
			}
		}
		lines = append(lines, fp.line(w, fp.s("muted").Render(fmt.Sprintf("%-9s", label))+v, ""))
	}
	if d := store.DueLabel(t, a.now); d != nil {
		field("due", "Due", a.icon(" ", "")+d.Long, d.Role)
	} else {
		field("due", "Due", "—", "dim")
	}
	if r := store.RepeatLabel(t.RepeatFlag); r != "" {
		field("repeat", "Repeat", "↻ "+r, "text")
	} else {
		field("repeat", "Repeat", "never", "dim")
	}
	field("list", "List", a.st.ListPath(t.ProjectID), "text")
	if len(t.Tags) > 0 {
		field("tags", "Tags", "#"+strings.Join(t.Tags, "  #"), "secondary")
	} else {
		field("tags", "Tags", "—", "dim")
	}
	field("priority", "Priority", "● "+store.PrioName(t.Priority), store.PrioRole(t.Priority))

	// checklist
	if n := len(t.Items); n > 0 {
		d := 0
		for _, it := range t.Items {
			if it.Status != 0 {
				d++
			}
		}
		const barW = 14
		fill := d * barW / n
		lines = append(lines, "", p.line(w, p.s("sub").Render("Checklist ")+p.s("ok").Render(strings.Repeat("━", fill))+
			p.s("line").Render(strings.Repeat("─", barW-fill))+p.sp(1)+p.s("sub").Render(fmt.Sprintf("%d/%d", d, n)), ""))
		for i, it := range t.Items {
			ip := pick(fmt.Sprintf("c%d", i))
			mark, text := ip.s("muted").Render("○"), ip.s("text").Render(trunc(it.Title, cw-2))
			if it.Status != 0 {
				mark, text = ip.s("ok").Render("✓"), ip.s("dim").Strikethrough(true).Render(trunc(it.Title, cw-2))
			}
			lines = append(lines, ip.line(w, mark+ip.sp(1)+text, ""))
		}
	} else {
		lines = append(lines, "")
	}
	ap := pick("additem")
	if editing("additem") {
		v := a.in.view(ap, "text", cw-2)
		if a.in.value() == "" {
			v += ap.s("dim").Render("new item · ⏎ add · esc done")
		}
		lines = append(lines, ap.line(w, ap.s("ok").Render("+")+ap.sp(1)+v, ""))
	} else {
		lines = append(lines, ap.line(w, ap.s("dim").Render("+ add checklist item"), ""))
	}

	// notes
	hint := p.s("dim").Render(" markdown")
	if editing("notes") {
		hint = p.s("dim").Render(" esc save")
	}
	lines = append(lines, "", p.line(w, p.s("sub").Render("Notes ")+p.s("line").Render(strings.Repeat("─", max(cw-6-9, 0))), hint))
	a.notesTop = len(lines) - 1
	np := pick("notes")
	if sel >= 0 && keys[a.df] == "notes" && sel < a.offDetail { // scrolled into the notes: don't snap back to their top
		sel = -1
	}
	if editing("notes") {
		np = p.on(a.selBg(true))
		ls, row := a.in.lines(np, "text", cw)
		sel = len(lines) + row
		for len(ls) < 4 {
			ls = append(ls, "")
		}
		for _, l := range ls {
			lines = append(lines, np.line(w, l, ""))
		}
	} else if _, notes := t.Notes(); strings.TrimSpace(*notes) == "" {
		lines = append(lines, np.line(w, np.s("dim").Render("no notes · i to write"), ""))
	} else {
		lines = append(lines, a.markdownLines(np, *notes, w)...)
	}
	return lines, sel
}

func (a *App) markdownLines(p pen, src string, w int) []string {
	cw := w - 4
	var out []string
	for _, ml := range parse.Markdown(src) {
		switch ml.Kind {
		case parse.Blank:
			out = append(out, p.line(w, "", ""))
		case parse.Bullet:
			for i, l := range parse.Wrap(ml.Spans, cw-2) {
				lead := p.s("accent").Render("•") + p.sp(1)
				if i > 0 {
					lead = p.sp(2)
				}
				out = append(out, p.line(w, lead+a.spans(p, l, "sub"), ""))
			}
		default:
			role := "sub"
			if ml.Kind == parse.Heading {
				role = "accent"
			}
			for _, l := range parse.Wrap(ml.Spans, cw) {
				out = append(out, p.line(w, a.spans(p, l, role), ""))
			}
		}
	}
	return out
}

func (a *App) spans(p pen, ss []parse.Span, role string) string {
	var b strings.Builder
	for _, s := range ss {
		switch s.Kind {
		case parse.Bold:
			b.WriteString(p.s("text").Bold(true).Render(s.Text))
		case parse.Code:
			b.WriteString(p.s("secondary").Background(a.th.C("surface2")).Render(s.Text))
		default:
			b.WriteString(p.s(role).Bold(role == "accent").Render(s.Text))
		}
	}
	return b.String()
}

// ---- composition ----

// selBg is the selected-row background: accent@15% in the focused pane, surface2 elsewhere.
func (a *App) selBg(focused bool) color.Color {
	if focused {
		return tint(a.th, "accent", 0.15)
	}
	return a.th.C("surface2")
}

func (a *App) effFocus(n int) string {
	switch {
	case a.focus == "lists" && n < 3:
		return "tasks"
	case a.focus == "detail" && n == 1 && !a.sheet:
		return "tasks"
	}
	return a.focus
}

func (a *App) viewMain() (string, [][2]string) {
	n, lw, tw, dw := layout(a.w, a.cfg.Layout.Columns)
	ef := a.effFocus(n)
	paneH := a.h - 1
	overlay := a.settings || a.help || a.cmd != nil || (n == 1 && a.sheet)
	p := a.pen()
	p.faint = overlay

	var panes []string
	if n == 3 {
		lines, sel := a.listsLines(p, lw, ef == "lists")
		panes = append(panes, a.frame(p, frameOpts{w: lw, h: paneH, title: "Lists", focused: ef == "lists",
			body: window(lines, sel, paneH-2, &a.offLists)}))
	}

	name, icon, _ := a.listMeta(a.list)
	title := icon + " " + name
	if n < 3 {
		title += " ▾"
	}
	lines, sel, pos, total := a.tasksLines(p, tw, ef == "tasks")
	br := ""
	if total > 0 {
		br = fmt.Sprintf("%d/%d", pos, total)
	}
	panes = append(panes, a.frame(p, frameOpts{w: tw, h: paneH, title: title, focused: ef == "tasks",
		topRight: fmt.Sprintf("%d open", a.st.OpenCount(a.list, a.now)), bottomRight: br,
		body: window(lines, sel, paneH-2, &a.offTasks)}))

	detail := func(p pen, w, h int) string {
		focused := ef == "detail"
		lines, sel := a.detailLines(p, a.selTask(), w, focused)
		br := "⏎ open"
		if focused {
			br = "i edit · esc back"
		}
		return a.frame(p, frameOpts{w: w, h: h, title: "Details", focused: focused, bottomRight: br,
			body: window(lines, sel, h-2, &a.offDetail)})
	}
	if n >= 2 {
		panes = append(panes, detail(p, dw, paneH))
	}

	parts := []string{" "}
	for i, pn := range panes {
		if i > 0 {
			parts = append(parts, " ")
		}
		parts = append(parts, pn)
	}
	base := lipgloss.JoinHorizontal(lipgloss.Top, append(parts, " ")...)

	var hints [][2]string
	switch ef {
	case "lists":
		open := [2]string{"l", "open"}
		if r := a.sideRow(); r != nil && r.folder != "" {
			open = [2]string{"␣ ⏎", "open folder"}
			if a.folderOpen(r.folder) {
				open[1] = "close folder"
			}
		}
		hints = [][2]string{{"j/k", "move"}, open, {"/", "search"}, {"?", "keys"}, {":", "command"}, {",", "settings"}, {"a", "add"}}
	case "tasks":
		done := "done"
		if t := a.selTask(); t != nil && store.Done(t) {
			done = "reopen"
		}
		hints = [][2]string{{"j/k", "move"}, {"⏎", "open"}, {"x", done}, {"a", "add"}, {"?", "keys"}, {"d", "due"}, {"m", "move to"}, {"p", "priority"}, {"/", "search"}, {":", "command"}, {",", "settings"}}
	default:
		hints = [][2]string{{"j/k", "field"}, {"i", "edit"}, {"␣", "toggle"}, {"?", "keys"}, {"d", "due"}, {"m", "move to"}, {"c", "checklist"}, {"h", "back"}}
	}

	switch {
	case a.cmd != nil:
		if n == 1 && a.sheet { // keep the sheet under the bar
			sh := max(paneH*68/100, 8)
			base = lipgloss.NewCompositor(lipgloss.NewLayer(base), lipgloss.NewLayer(detail(p, tw, sh)).X(1).Y(paneH-sh).Z(1)).Render()
		}
		box, bw, _ := a.viewCmd()
		base = lipgloss.NewCompositor(lipgloss.NewLayer(base), lipgloss.NewLayer(box).X((a.w-bw)/2).Y(a.h*14/100).Z(2)).Render()
		hints = [][2]string{{"↑↓", "select"}, {"⏎", "run"}, {"esc", "close"}}
	case a.edit != "":
		enter := "save"
		switch a.edit {
		case "notes":
			enter = "newline"
		case "additem":
			enter = "add"
		}
		hints = [][2]string{{"esc", "save"}, {"⏎", enter}}
		if n == 1 && a.sheet {
			sh := max(paneH*68/100, 8)
			base = lipgloss.NewCompositor(lipgloss.NewLayer(base), lipgloss.NewLayer(detail(a.overlayPen(), tw, sh)).X(1).Y(paneH-sh).Z(1)).Render()
		}
	case a.settings:
		box, bw, bh := a.viewSettings()
		base = lipgloss.NewCompositor(lipgloss.NewLayer(base),
			lipgloss.NewLayer(box).X((a.w-bw)/2).Y(max((paneH-bh)/2, 0)).Z(1)).Render()
		hints = [][2]string{{"j/k", "move"}, {"h/l", "change"}, {"esc", "close"}}
	case a.help:
		box, bw, bh := a.viewHelp()
		base = lipgloss.NewCompositor(lipgloss.NewLayer(base),
			lipgloss.NewLayer(box).X((a.w-bw)/2).Y(max((paneH-bh)/2, 0)).Z(1)).Render()
		hints = [][2]string{{"j/k", "scroll"}, {"esc", "close"}}
	case n == 1 && a.sheet:
		sh := max(paneH*68/100, 8)
		sheet := detail(a.overlayPen(), tw, sh)
		base = lipgloss.NewCompositor(lipgloss.NewLayer(base), lipgloss.NewLayer(sheet).X(1).Y(paneH-sh).Z(1)).Render()
	}
	return base, hints
}

// ---- keys ----

func (a *App) mainKey(k tea.KeyPressMsg) tea.Cmd {
	n, _, _, _ := layout(a.w, a.cfg.Layout.Columns)
	ef := a.effFocus(n)
	key := k.String()
	if a.cfg.Keys.Keymap != "arrows" {
		if alias, ok := map[string]string{"j": "down", "k": "up", "h": "left", "l": "right", "g": "home", "G": "end"}[key]; ok {
			key = alias
		}
	}
	switch key {
	case "q":
		return tea.Quit
	case "down":
		a.move(ef, 1)
	case "up":
		a.move(ef, -1)
	case "home":
		a.move(ef, -1<<20)
	case "end":
		a.move(ef, 1<<20)
	case "pgup":
		a.move(ef, -a.page())
	case "pgdown":
		a.move(ef, a.page())
	case "left":
		switch {
		case ef == "detail":
			a.focus, a.sheet = "tasks", false
		case ef == "tasks" && n == 3:
			a.focus = "lists"
		case ef == "tasks":
			a.openCmd("@")
		}
	case "right", "enter":
		switch ef {
		case "lists":
			a.openSide()
		case "tasks":
			a.openDetail(n)
		default:
			if key == "enter" {
				return a.activate()
			}
		}
	case "esc":
		if ef == "detail" {
			a.focus, a.sheet = "tasks", false
		}
	case "tab":
		switch {
		case n == 1 && ef == "detail":
			a.focus, a.sheet = "tasks", false
		case n == 1:
			a.openDetail(n)
		default:
			order := []string{"tasks", "detail"}
			if n == 3 {
				order = []string{"lists", "tasks", "detail"}
			}
			a.focus = order[(slices.Index(order, ef)+1)%len(order)]
		}
	case "1":
		if n == 3 {
			a.focus = "lists"
		} else {
			a.openCmd("@")
		}
	case "2":
		a.focus, a.sheet = "tasks", false
	case "3":
		a.openDetail(n)
	case "space", "x":
		if ef == "lists" {
			if r := a.sideRow(); r != nil && r.folder != "" && key == "space" {
				a.toggleFolder(r.folder)
			}
			return nil
		}
		if key == "x" {
			if cmd, ok := a.undoDone(); ok {
				return cmd
			}
		}
		t := a.selTask()
		if t == nil {
			return nil
		}
		if k := detailKeys(t)[min(a.df, len(detailKeys(t))-1)]; ef == "detail" && strings.HasPrefix(k, "c") {
			return a.activate()
		}
		return a.toggleDone(t)
	case "p":
		if t := a.selTask(); t != nil && ef != "lists" {
			return a.cyclePrio(t)
		}
	case "i", "e":
		switch ef {
		case "tasks":
			a.startEdit("title")
		case "detail":
			if t := a.selTask(); t != nil {
				if k := detailKeys(t)[a.df]; k == "title" || k == "due" || k == "tags" || k == "notes" || k == "additem" {
					a.startEdit(k)
				} else {
					return a.activate()
				}
			}
		}
	case "a", "n":
		a.openCmd("+ ")
	case "/", "ctrl+k":
		a.openCmd("")
	case ":":
		a.openCmd(">")
	case "@":
		a.openCmd("@")
	case "#":
		a.openCmd("#")
	case "ctrl+r":
		a.setFlash("⟳ syncing…")
		return a.syncNow()
	case "t":
		a.toggleDueLabels()
	case "s":
		if ef != "lists" {
			a.sortPicker(0)
		}
	case "U":
		a.askUpdate()
	case ",":
		a.settings, a.sIdx = true, 1
	case "?":
		a.help, a.offHelp = true, 0
	case "m", "d", "c":
		t := a.selTask()
		if t == nil || ef == "lists" {
			return nil
		}
		switch key {
		case "m":
			a.movePicker(t)
		case "d":
			a.duePicker(t)
		default:
			a.startEdit("additem")
		}
	}
	return nil
}

func (a *App) sideRow() *sideItem {
	for _, r := range a.sideItems() {
		if !r.header && r.key == a.sideKey {
			return &r
		}
	}
	return nil
}

func (a *App) move(ef string, d int) {
	clamp := func(i, n int) int { return max(0, min(i, n-1)) }
	switch ef {
	case "lists":
		var nav []sideItem
		for _, r := range a.sideItems() {
			if !r.header {
				nav = append(nav, r)
			}
		}
		i := slices.IndexFunc(nav, func(r sideItem) bool { return r.key == a.sideKey })
		r := nav[clamp(i+d, len(nav))]
		a.sideKey = r.key
		if r.list != "" && r.list != a.list {
			a.list, a.taskID, a.offTasks = r.list, "", 0
		}
	case "tasks":
		nav := navTasks(a.groups())
		if len(nav) == 0 {
			return
		}
		i := 0
		if t := a.selTask(); t != nil {
			i = slices.Index(nav, t)
		}
		a.taskID, a.df, a.offDetail = nav[clamp(i+d, len(nav))].ID, 0, 0
	case "detail":
		t := a.selTask()
		if t == nil {
			return
		}
		// On the notes (the last field) ↑↓ and pgup/pgdown scroll them; going up leaves once
		// their header is back on top. home/end keep jumping between fields.
		if keys := detailKeys(t); a.df == len(keys)-1 && d != -1<<20 && (d > 0 || a.offDetail > a.notesTop) {
			if a.offDetail += d; d < 0 {
				a.offDetail = max(a.offDetail, a.notesTop)
			}
			return
		}
		a.df = clamp(a.df+d, len(detailKeys(t)))
	}
}

func (a *App) openSide() {
	r := a.sideRow()
	if r == nil {
		return
	}
	if r.folder != "" {
		a.toggleFolder(r.folder)
		return
	}
	a.list, a.focus = r.list, "tasks"
}

// Folders start collapsed, as in the web app; the open ones are remembered in config.toml.
func (a *App) folderOpen(id string) bool { return slices.Contains(a.cfg.Layout.OpenFolders, id) }

func (a *App) toggleFolder(id string) {
	if i := slices.Index(a.cfg.Layout.OpenFolders, id); i >= 0 {
		a.cfg.Layout.OpenFolders = slices.Delete(a.cfg.Layout.OpenFolders, i, i+1)
	} else {
		a.cfg.Layout.OpenFolders = append(a.cfg.Layout.OpenFolders, id)
	}
	a.save()
}

func (a *App) toggleDueLabels() {
	a.cfg.Tasks.DueLabel = !a.cfg.Tasks.DueLabel
	a.save()
	a.setFlash("due-date labels " + onOff(a.cfg.Tasks.DueLabel))
}

func (a *App) openDetail(n int) {
	t := a.selTask()
	if t == nil {
		return
	}
	a.remember(t.ID)
	a.focus, a.df, a.offDetail, a.sheet = "detail", 0, 0, n == 1
}

func (a *App) save() {
	if err := config.Save(a.cfg); err != nil {
		a.setFlash("couldn't save config: " + err.Error())
	}
}

func onOff(b bool) string {
	if b {
		return "on"
	}
	return "off"
}
