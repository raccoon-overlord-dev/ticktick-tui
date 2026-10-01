package ui

import (
	"context"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"ttui/internal/api"
	"ttui/internal/parse"
	"ttui/internal/store"
)

// op is one queued write. Edits apply to the store at once; ops are sent one at a time
// (writes are serialized) and reverted if the API rejects them.
type op struct {
	kind   string // create | update | complete | move
	taskID string
	before *api.Task      // state to restore on failure; nil for create
	fields map[string]any // update fields, the create body, or the move's "from" and "to"
}

type (
	opDoneMsg struct {
		op  op
		t   *api.Task
		err error
	}
	syncTickMsg int // generation; stale ticks are ignored
)

func (a *App) enqueue(o op) tea.Cmd {
	a.queue = append(a.queue, o)
	return a.pump()
}

// pump sends the head of the queue if nothing is in flight.
func (a *App) pump() tea.Cmd {
	if a.busy || len(a.queue) == 0 {
		return nil
	}
	if a.demo { // no API in demo mode: keep the local change
		a.queue = nil
		return nil
	}
	a.busy = true
	o := a.queue[0]
	id := o.taskID
	if real, ok := a.idMap[id]; ok {
		id = real
	}
	pid := ""
	if t := a.st.Task(id); t != nil {
		pid = t.ProjectID
	}
	c := api.New(a.signed.AccessToken)
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		var t *api.Task
		var err error
		switch o.kind {
		case "create":
			t, err = c.CreateTask(ctx, o.fields)
		case "update":
			t, err = c.UpdateTask(ctx, id, pid, o.fields)
		case "complete":
			if err = c.CompleteTask(ctx, pid, id); err == nil {
				// re-read: a repeating task stays open with its next due date
				t, err = c.GetTask(ctx, pid, id)
			}
		case "move":
			to := o.fields["to"].(string)
			if err = c.MoveTask(ctx, o.fields["from"].(string), to, id); err == nil {
				t, err = c.GetTask(ctx, to, id)
			}
		}
		return opDoneMsg{o, t, err}
	}
}

func (a *App) onOpDone(m opDoneMsg) tea.Cmd {
	a.busy = false
	a.queue = a.queue[1:]
	if a.st == nil { // signed out meanwhile
		return nil
	}
	if m.err != nil {
		if m.op.kind == "create" {
			a.st.Remove(m.op.taskID)
		} else if m.op.before != nil {
			b := *m.op.before
			if real, ok := a.idMap[b.ID]; ok { // edited before its create returned
				b.ID = real
			}
			a.st.Upsert(b)
		}
		a.flashError("✗ couldn't save: " + shortErr(m.err))
		if unauthorized(m.err) {
			return a.expired()
		}
		return a.pump()
	}
	// Take the server's copy unless more local edits to this task are still queued.
	pending := slices.ContainsFunc(a.queue, func(o op) bool { return o.taskID == m.op.taskID })
	if m.op.kind == "create" {
		a.idMap[m.op.taskID] = m.t.ID
		if local := a.st.Task(m.op.taskID); local != nil && pending {
			l := *local // keep the queued local edits, under the server's id
			l.ID, l.ProjectID = m.t.ID, m.t.ProjectID
			a.st.Upsert(l)
		}
		a.st.Remove(m.op.taskID)
		if a.taskID == m.op.taskID {
			a.taskID = m.t.ID
		}
		if a.editID == m.op.taskID {
			a.editID = m.t.ID
		}
	}
	if m.t != nil && m.t.ID != "" && !pending {
		a.st.Upsert(*m.t)
	}
	a.st.Save()
	return a.pump()
}

func unauthorized(err error) bool {
	var e *api.Error
	return errors.As(err, &e) && e.Status == 401
}

func shortErr(err error) string {
	var e *api.Error
	if errors.As(err, &e) {
		return fmt.Sprintf("HTTP %d", e.Status)
	}
	return err.Error()
}

// expired returns to the auth screen after a 401.
func (a *App) expired() tea.Cmd {
	a.signed, a.screen, a.queue, a.busy = nil, screenAuth, nil, false
	a.auth = authScreen{notice: "Session expired · sign in again"}
	return a.auth.start()
}

// ---- sync ----

func syncEvery(s string) time.Duration {
	d, err := time.ParseDuration(s)
	if err != nil { // "manual"
		return 0
	}
	return d
}

// scheduleSync arms the next background sync; any earlier timer becomes stale.
func (a *App) scheduleSync() tea.Cmd {
	a.syncGen++
	d := syncEvery(a.cfg.Account.SyncEvery)
	if d == 0 || a.demo {
		return nil
	}
	gen := a.syncGen
	return tea.Tick(d, func(time.Time) tea.Msg { return syncTickMsg(gen) })
}

func (a *App) syncNow() tea.Cmd {
	if a.demo {
		a.setFlash("⟳ synced · demo data")
		return nil
	}
	if a.signed == nil || a.syncing {
		return nil
	}
	a.syncing = true
	return fetchStore(a.signed.AccessToken, nil)
}

// ---- actions on the selected task ----

func (a *App) update(t *api.Task, fields map[string]any, apply func(*api.Task)) tea.Cmd {
	before := store.Clone(*t)
	apply(t)
	return a.enqueue(op{kind: "update", taskID: t.ID, before: &before, fields: fields})
}

func (a *App) toggleDone(t *api.Task) tea.Cmd {
	if store.Done(t) {
		a.setFlash(fmt.Sprintf("reopened “%s”", t.Title))
		return a.update(t, map[string]any{"status": 0}, func(t *api.Task) { t.Status, t.CompletedTime = 0, "" })
	}
	nav := navTasks(a.groups())
	if i := slices.Index(nav, t); i >= 0 {
		switch {
		case i+1 < len(nav):
			a.taskID = nav[i+1].ID
		case i > 0:
			a.taskID = nav[i-1].ID
		}
	}
	before := store.Clone(*t)
	t.Status, t.CompletedTime = 2, time.Now().UTC().Format(api.DateLayout)
	a.lastDone = t.ID
	a.setFlash(fmt.Sprintf("✓ completed “%s” · x to undo", t.Title))
	return a.enqueue(op{kind: "complete", taskID: t.ID, before: &before})
}

// undoDone reopens the task completed last, while its "x to undo" message is showing.
func (a *App) undoDone() (tea.Cmd, bool) {
	if a.lastDone == "" || !strings.HasPrefix(a.flash, "✓ completed") {
		return nil, false
	}
	t := a.st.Task(a.lastDone)
	a.lastDone = ""
	if t == nil || !store.Done(t) {
		return nil, false
	}
	a.taskID = t.ID
	return a.toggleDone(t), true
}

var prioNext = map[int]int{store.PrioHigh: store.PrioMed, store.PrioMed: store.PrioLow, store.PrioLow: store.PrioNone, store.PrioNone: store.PrioHigh}

func (a *App) cyclePrio(t *api.Task) tea.Cmd {
	p, ok := prioNext[t.Priority]
	if !ok {
		p = store.PrioHigh
	}
	a.taskID = t.ID
	a.setFlash("priority → " + strings.ToLower(store.PrioName(p)))
	return a.update(t, map[string]any{"priority": p}, func(t *api.Task) { t.Priority = p })
}

func (a *App) toggleCheck(t *api.Task, i int) tea.Cmd {
	items := slices.Clone(t.Items)
	if items[i].Status == 0 {
		items[i].Status = 1
	} else {
		items[i].Status = 0
	}
	return a.update(t, map[string]any{"items": items}, func(t *api.Task) { t.Items = items })
}

// addItem appends a checklist item; the server assigns its id.
func (a *App) addItem(t *api.Task, title string) tea.Cmd {
	items := append(slices.Clone(t.Items), api.Item{Title: title})
	return a.update(t, map[string]any{"items": items}, func(t *api.Task) { t.Items = items })
}

// movePicker opens the list menu for t (m, or ⏎ on the List field).
func (a *App) movePicker(t *api.Task) {
	var items []cmdItem
	for _, id := range a.allLists() {
		if id != "inbox" && !strings.HasPrefix(id, "p:") {
			continue
		}
		it := a.listItem(id, nil)
		if (id == "inbox" && store.IsInbox(t.ProjectID)) || id == "p:"+t.ProjectID {
			it.hint = "current"
		}
		it.run = func() tea.Cmd { return a.moveTask(t, id) }
		items = append(items, it)
	}
	a.openPick("move to", items)
}

func (a *App) moveTask(t *api.Task, list string) tea.Cmd {
	to := strings.TrimPrefix(list, "p:")
	if list == "inbox" {
		if to = a.st.InboxID(); to == "" {
			a.flashError("✗ Inbox id unknown until a task in the Inbox has synced")
			return nil
		}
	}
	if to == t.ProjectID {
		return nil
	}
	if _, ok := a.idMap[t.ID]; strings.HasPrefix(t.ID, "tmp-") && !ok {
		a.setFlash("still saving this task · try again in a moment")
		return nil
	}
	before := store.Clone(*t)
	from := t.ProjectID
	t.ProjectID = to
	name, _, _ := a.listMeta(list)
	a.setFlash("moved to " + name)
	return a.enqueue(op{kind: "move", taskID: t.ID, before: &before, fields: map[string]any{"from": from, "to": to}})
}

// duePicker opens the due date menu for t (d, or ⏎ on the Due field): the dates from
// config due_menu, No date, and Custom… for the text editor.
func (a *App) duePicker(t *api.Task) {
	now := time.Now()
	var items []cmdItem
	for _, s := range a.cfg.Tasks.DueMenu {
		d, err := parse.ParseDue(s, now)
		if err != nil || d == nil {
			continue
		}
		hint := d.At(now).Format("Mon 2 Jan")
		if d.HasTime {
			hint += d.At(now).Format(" 15:04")
		}
		items = append(items, cmdItem{icon: a.icon("\uf017", "~"), iconRole: "secondary", label: s, hint: hint,
			run: func() tea.Cmd {
				// a date without a time keeps the task's time, as in the web app
				if at, ok := store.DueTime(t); ok && !d.HasTime && !t.IsAllDay {
					at = at.In(now.Location())
					d.H, d.M, d.HasTime = at.Hour(), at.Minute(), true
				}
				a.setFlash("due → " + s)
				return a.setDue(t, d)
			}})
	}
	items = append(items,
		cmdItem{icon: "×", iconRole: "dim", label: "No date", run: func() tea.Cmd { a.setFlash("due date cleared"); return a.setDue(t, nil) }},
		cmdItem{icon: "…", iconRole: "dim", label: "Custom", hint: "tomorrow 17:00, fri, +3d", run: func() tea.Cmd { a.startEdit("due"); return nil }})
	a.openPick("due", items)
}

// setDue sets or clears (d nil) t's due date; no time means all-day.
func (a *App) setDue(t *api.Task, d *parse.Due) tea.Cmd {
	f := dueFields(d, time.Now())
	return a.update(t, f, func(t *api.Task) {
		t.DueDate, _ = f["dueDate"].(string)
		t.StartDate = t.DueDate
		if d != nil {
			t.IsAllDay, t.TimeZone = !d.HasTime, localZone()
		} else {
			t.RepeatFlag = "" // the server drops the repeat rule with the date
		}
	})
}

var repeats = []struct{ label, rule string }{
	{"never", ""},
	{"Daily", "RRULE:FREQ=DAILY;INTERVAL=1"},
	{"Weekdays", "RRULE:FREQ=WEEKLY;INTERVAL=1;BYDAY=MO,TU,WE,TH,FR"},
	{"Weekly", "RRULE:FREQ=WEEKLY;INTERVAL=1"},
	{"Monthly", "RRULE:FREQ=MONTHLY;INTERVAL=1"},
}

func (a *App) cycleRepeat(t *api.Task) tea.Cmd {
	if t.DueDate == "" { // the API drops a repeat rule on a task without a due date
		a.setFlash("set a due date first · repeat needs one")
		return nil
	}
	cur := store.RepeatLabel(t.RepeatFlag)
	if cur == "" {
		cur = "never"
	}
	i := slices.IndexFunc(repeats, func(r struct{ label, rule string }) bool { return r.label == cur })
	next := repeats[(i+1)%len(repeats)] // Custom (-1) goes to never
	a.setFlash("repeat → " + next.label)
	return a.update(t, map[string]any{"repeatFlag": next.rule}, func(t *api.Task) { t.RepeatFlag = next.rule })
}

// dueFields turns a parsed due date into API fields (all-day when no time is given).
func dueFields(d *parse.Due, now time.Time) map[string]any {
	if d == nil {
		return map[string]any{"dueDate": nil, "startDate": nil} // null clears; "" is ignored
	}
	at := d.At(now).Format(api.DateLayout)
	return map[string]any{"dueDate": at, "startDate": at, "isAllDay": !d.HasTime, "timeZone": localZone()}
}

// localZone is the IANA name of the local time zone (TickTick needs it for all-day dates).
func localZone() string {
	if tz := os.Getenv("TZ"); tz != "" {
		return tz
	}
	if p, err := os.Readlink("/etc/localtime"); err == nil {
		if _, name, ok := strings.Cut(p, "zoneinfo/"); ok {
			return name
		}
	}
	return "UTC"
}

// commitEdit saves the inline edit. It returns false (and keeps editing) if the value is invalid.
func (a *App) commitEdit() (tea.Cmd, bool) {
	t := a.st.Task(a.editID)
	field, v := a.edit, a.in.value()
	if t == nil {
		a.edit = ""
		return nil, true
	}
	var cmd tea.Cmd
	switch field {
	case "title":
		v = strings.TrimSpace(v)
		if v == "" {
			a.setFlash("title can't be empty")
			return nil, false
		}
		if v != t.Title {
			cmd = a.update(t, map[string]any{"title": v}, func(t *api.Task) { t.Title = v })
		}
	case "notes":
		if v != t.Content {
			cmd = a.update(t, map[string]any{"content": v}, func(t *api.Task) { t.Content = v })
		}
	case "tags":
		var tags []string
		for _, w := range strings.FieldsFunc(v, func(r rune) bool { return r == ' ' || r == ',' }) {
			if w = strings.TrimPrefix(w, "#"); w != "" {
				tags = append(tags, strings.ToLower(w))
			}
		}
		if !slices.Equal(tags, t.Tags) {
			cmd = a.update(t, map[string]any{"tags": tags}, func(t *api.Task) { t.Tags = tags })
		}
	case "due":
		d, err := parse.ParseDue(v, time.Now())
		if err != nil {
			a.setFlash(err.Error() + " · try: tomorrow 17:00, fri, 2026-10-08")
			return nil, false
		}
		cmd = a.setDue(t, d)
	case "additem":
		if v = strings.TrimSpace(v); v != "" {
			cmd = a.addItem(t, v)
		}
	}
	a.edit = ""
	a.setFlash("saved")
	return cmd, true
}

// dueText is the due date as the Due field editor shows it.
func dueText(t *api.Task, now time.Time) string {
	d, ok := store.DueTime(t)
	if !ok {
		return ""
	}
	day, _ := store.DayDiff(t, now)
	var s string
	switch {
	case day == 0:
		s = "today"
	case day == 1:
		s = "tomorrow"
	case day == -1:
		s = "yesterday"
	case day > 1 && day < 7:
		s = strings.ToLower(d.Format("Mon"))
	default:
		s = d.Format("2006-01-02")
	}
	if !t.IsAllDay {
		s += " " + d.Format("15:04")
	}
	return s
}

func (a *App) startEdit(field string) {
	t := a.selTask()
	if t == nil {
		return
	}
	n, _, _, _ := layout(a.w, a.cfg.Layout.Columns)
	var v string
	switch field {
	case "title":
		v = t.Title
	case "due":
		v = dueText(t, a.now)
	case "tags":
		if len(t.Tags) > 0 {
			v = "#" + strings.Join(t.Tags, " #")
		}
	case "notes":
		v = t.Content
	case "additem":
	default:
		return
	}
	a.focus, a.edit, a.editID, a.in = "detail", field, t.ID, newInput(v)
	a.df = slices.Index(detailKeys(t), field)
	if n == 1 {
		a.sheet = true
	}
	a.remember(t.ID)
}

// activate is ⏎ on a detail field.
func (a *App) activate() tea.Cmd {
	t := a.selTask()
	if t == nil {
		return nil
	}
	switch k := detailKeys(t)[a.df]; {
	case k == "title" || k == "tags" || k == "notes" || k == "additem":
		a.startEdit(k)
	case k == "due":
		a.duePicker(t)
	case k == "repeat":
		return a.cycleRepeat(t)
	case k == "priority":
		return a.cyclePrio(t)
	case k == "list":
		a.movePicker(t)
	case strings.HasPrefix(k, "c"):
		var i int
		fmt.Sscanf(k, "c%d", &i)
		return a.toggleCheck(t, i)
	}
	return nil
}

// createTask adds the quick-add result optimistically.
func (a *App) createTask(add parse.Add) tea.Cmd {
	if add.Title == "" {
		a.setFlash("type a title first")
		return nil
	}
	a.tmpN++
	tmp := fmt.Sprintf("tmp-%d", a.tmpN)
	now := time.Now()
	t := api.Task{ID: tmp, Title: add.Title, Priority: add.Prio, Tags: add.Tags, CreatedTime: now.UTC().Format(api.DateLayout), TimeZone: localZone()}
	body := map[string]any{"title": add.Title, "priority": add.Prio}
	if len(add.Tags) > 0 {
		body["tags"] = add.Tags
	}
	t.ProjectID = strings.TrimPrefix(add.List, "p:")
	if add.List != "inbox" {
		body["projectId"] = t.ProjectID // Inbox: omit and the server picks it
	}
	if add.Due != nil {
		for k, v := range dueFields(add.Due, now) {
			body[k] = v
		}
		t.DueDate, t.IsAllDay = body["dueDate"].(string), !add.Due.HasTime
		t.StartDate = t.DueDate
	}
	a.st.Upsert(t)
	a.taskID, a.focus = tmp, "tasks"
	name, _, _ := a.listMeta(add.List)
	a.setFlash("+ added to " + name)
	return a.enqueue(op{kind: "create", taskID: tmp, fields: body})
}

// remember keeps the 3 most recently opened tasks for the command bar's RECENT group.
func (a *App) remember(id string) {
	a.recent = slices.DeleteFunc(a.recent, func(x string) bool { return x == id })
	a.recent = append([]string{id}, a.recent...)
	if len(a.recent) > 3 {
		a.recent = a.recent[:3]
	}
}

func (a *App) flashError(s string) {
	a.setFlash(s)
	a.flashRole = "error"
}
