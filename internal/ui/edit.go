package ui

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/raccoon-overlord-dev/ticktick-tui/internal/api"
	"github.com/raccoon-overlord-dev/ticktick-tui/internal/config"
	"github.com/raccoon-overlord-dev/ticktick-tui/internal/parse"
	"github.com/raccoon-overlord-dev/ticktick-tui/internal/store"
)

// op is one queued write. Edits apply to the store at once; ops are sent one at a time
// (writes are serialized) and reverted if the API rejects them.
type op struct {
	kind   string // create | update | complete | move | delete | skip
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
	} else if o.before != nil { // deleted locally already
		pid = o.before.ProjectID
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
		case "delete":
			err = c.DeleteTask(ctx, pid, id)
		case "skip":
			t, err = skipOccurrence(ctx, c, pid, id, o.before)
		}
		return opDoneMsg{o, t, err}
	}
}

// skipOccurrence deletes only the current occurrence of a repeating task. The API has no
// call for it, so it completes the task (the server moves it to the next occurrence and
// files a completed copy) and deletes that copy. On the last occurrence the task itself
// ends up completed and is deleted. Returns the task at its next occurrence, or nil.
func skipOccurrence(ctx context.Context, c *api.Client, pid, id string, before *api.Task) (*api.Task, error) {
	start := time.Now().Add(-time.Minute)
	if err := c.CompleteTask(ctx, pid, id); err != nil {
		return nil, err
	}
	t, err := c.GetTask(ctx, pid, id)
	if err != nil {
		return nil, err
	}
	if store.Done(t) { // no next occurrence
		return nil, c.DeleteTask(ctx, pid, id)
	}
	done, err := c.CompletedTasks(ctx, start, time.Time{})
	if err != nil {
		return t, err
	}
	// The copy has a new id and no link back: match it by list, title and due date.
	for _, d := range done {
		if d.ProjectID == pid && d.Title == before.Title && d.RepeatFlag == "" && sameTime(d.DueDate, before.DueDate) {
			return t, c.DeleteTask(ctx, pid, d.ID)
		}
	}
	return t, errors.New("skipped, but its completed copy wasn't found")
}

// sameTime compares API dates, which come back with milliseconds.
func sameTime(a, b string) bool {
	ta, err1 := time.Parse(api.DateLayout, a)
	tb, err2 := time.Parse(api.DateLayout, b)
	return err1 == nil && err2 == nil && ta.Equal(tb)
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
		} else if m.op.kind == "skip" && m.t != nil { // completed on the server already
			a.st.Upsert(*m.t)
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
		return tea.Batch(a.pump(), a.restartIfIdle())
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
	if m.op.kind == "skip" && m.t == nil { // that was the last occurrence
		a.st.Remove(m.op.taskID)
	}
	a.st.Save()
	return tea.Batch(a.pump(), a.restartIfIdle())
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
	return a.fetchStore(a.signed.AccessToken, nil)
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
	a.selectNeighbor(t)
	before := store.Clone(*t)
	t.Status, t.CompletedTime = 2, time.Now().UTC().Format(api.DateLayout)
	a.lastDone = t.ID
	a.setFlash(fmt.Sprintf("✓ completed “%s” · x to undo", t.Title))
	return a.enqueue(op{kind: "complete", taskID: t.ID, before: &before})
}

// selectNeighbor moves the cursor off t, to the next task (or the previous one at the end).
func (a *App) selectNeighbor(t *api.Task) {
	nav := navTasks(a.groups())
	if i := slices.Index(nav, t); i >= 0 {
		switch {
		case i+1 < len(nav):
			a.taskID = nav[i+1].ID
		case i > 0:
			a.taskID = nav[i-1].ID
		}
	}
}

// askDelete asks before deleting t (D). An open repeating task offers, like the web app,
// this occurrence only or the whole series; anything else cancels.
func (a *App) askDelete(t *api.Task) {
	a.delID = t.ID
	q := fmt.Sprintf("Delete “%s”? y / N", t.Title)
	if t.RepeatFlag != "" && !store.Done(t) {
		q = fmt.Sprintf("Delete “%s”? o this occurrence · a all occurrences · N cancel", t.Title)
	}
	a.flash, a.flashRole, a.flashUntil = q, "warn", time.Now().Add(time.Hour)
}

func (a *App) deleteKey(k tea.KeyPressMsg) tea.Cmd {
	t := a.st.Task(a.delID)
	a.delID, a.flash = "", ""
	if t == nil {
		return nil
	}
	repeat := t.RepeatFlag != "" && !store.Done(t)
	defer a.keepPlace(t)()
	switch key := k.String(); {
	case !repeat && key == "y", repeat && key == "a":
		before := store.Clone(*t)
		a.st.Remove(t.ID)
		a.setFlash(fmt.Sprintf("deleted “%s”", before.Title))
		return a.enqueue(op{kind: "delete", taskID: before.ID, before: &before})
	case repeat && key == "o":
		before := store.Clone(*t)
		t.Status = 2 // hidden until the server returns the next occurrence
		a.setFlash(fmt.Sprintf("deleted this occurrence of “%s”", t.Title))
		return a.enqueue(op{kind: "skip", taskID: t.ID, before: &before})
	}
	return nil
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
	// as in the web app: unticking an item reopens a completed task, and ticking the last
	// open one completes it
	if store.Done(t) && items[i].Status == 0 {
		a.setFlash(fmt.Sprintf("reopened “%s”", t.Title))
		return a.update(t, map[string]any{"items": items, "status": 0}, func(t *api.Task) { t.Items, t.Status, t.CompletedTime = items, 0, "" })
	}
	cmd := a.update(t, map[string]any{"items": items}, func(t *api.Task) { t.Items = items })
	if !store.Done(t) && !slices.ContainsFunc(items, func(it api.Item) bool { return it.Status == 0 }) {
		return tea.Sequence(cmd, a.toggleDone(t))
	}
	return cmd
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
	defer a.keepPlace(t)()
	before := store.Clone(*t)
	from := t.ProjectID
	t.ProjectID = to
	name, _, _ := a.listMeta(list)
	a.setFlash("moved to " + name)
	return a.enqueue(op{kind: "move", taskID: t.ID, before: &before, fields: map[string]any{"from": from, "to": to}})
}

var (
	groupOpts = []string{"list", "date", "created", "tag", "priority", "none"}
	sortOpts  = []string{"date", "created", "modified", "title", "tag", "priority"}
	orderOpts = []string{"oldest", "newest"}
	sortLabel = map[string]string{"list": "List", "date": "Date", "created": "Created Time", "modified": "Modified Time",
		"title": "Title", "tag": "Tag", "priority": "Priority", "none": "None", "oldest": "Oldest First", "newest": "Newest First"}
)

// sortPicker opens the Group by / Sort by / Order menu for the current list (s). Picking an
// option saves it as the list's own sort and reopens the menu at idx, so several can be set.
func (a *App) sortPicker(idx int) {
	if a.list == "completed" {
		a.setFlash("completed tasks are sorted by completion time")
		return
	}
	cur := a.sortFor(a.list)
	_, own := a.cfg.Tasks.ListSort[a.list]
	var items []cmdItem
	add := func(group string, opts []string, field *string) {
		for _, v := range opts {
			it := cmdItem{icon: "·", iconRole: "dim", label: sortLabel[v], group: group}
			if *field == v {
				it.icon, it.iconRole, it.hint = "●", "accent", "current"
			}
			i := len(items)
			it.run = func() tea.Cmd {
				so := a.sortFor(a.list)
				*map[string]*string{"group by": &so.GroupBy, "sort by": &so.SortBy, "order": &so.Order}[group] = v
				if a.cfg.Tasks.ListSort == nil {
					a.cfg.Tasks.ListSort = map[string]config.Sort{}
				}
				a.cfg.Tasks.ListSort[a.list] = so
				a.save()
				a.sortPicker(i)
				return nil
			}
			items = append(items, it)
		}
	}
	add("group by", groupOpts, &cur.GroupBy)
	add("sort by", sortOpts, &cur.SortBy)
	add("order", orderOpts, &cur.Order)
	if own {
		items = append(items, cmdItem{icon: "×", iconRole: "dim", label: "Use default", group: "this list", hint: "from Settings",
			run: func() tea.Cmd {
				delete(a.cfg.Tasks.ListSort, a.list)
				a.save()
				a.setFlash("sort → default")
				return nil
			}})
	}
	a.openPick("sort", items)
	a.cmd.idx = idx
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

// keepPlace is called before a change to t (due date, move, delete) and returns a func to
// call after it: if t has left the current list, the task above it gets the cursor (the
// new top one if t was first). Works on ids, since deleting shifts the store's tasks.
func (a *App) keepPlace(t *api.Task) func() {
	var ids []string
	for _, x := range navTasks(a.groups()) {
		ids = append(ids, x.ID)
	}
	id := t.ID
	return func() {
		i := slices.Index(ids, id)
		if i < 0 || slices.ContainsFunc(navTasks(a.groups()), func(x *api.Task) bool { return x.ID == id }) {
			return
		}
		switch {
		case i > 0:
			a.taskID = ids[i-1]
		case len(ids) > 1:
			a.taskID = ids[1]
		}
	}
}

// setDue sets or clears (d nil) t's due date; no time means all-day.
func (a *App) setDue(t *api.Task, d *parse.Due) tea.Cmd {
	f := dueFields(d, time.Now())
	defer a.keepPlace(t)()
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

// repeatPicker opens the repeat menu for t (⏎ on the Repeat field), like the web app's:
// Weekly, Monthly and Yearly repeat on the due date's weekday, day and date.
func (a *App) repeatPicker(t *api.Task) {
	due, ok := store.DueTime(t)
	if !ok { // the API drops a repeat rule on a task without a due date
		a.setFlash("set a due date first · repeat needs one")
		return
	}
	due = due.In(time.Now().Location())
	opts := []struct{ label, hint, rule string }{
		{"Daily", "", "RRULE:FREQ=DAILY;INTERVAL=1"},
		{"Weekdays", "Mon–Fri", "RRULE:FREQ=WEEKLY;INTERVAL=1;BYDAY=MO,TU,WE,TH,FR"},
		{"Weekly", due.Format("on Mon"), "RRULE:FREQ=WEEKLY;INTERVAL=1"},
		{"Monthly", "on the " + parse.Ordinal(due.Day()), "RRULE:FREQ=MONTHLY;INTERVAL=1"},
		{"Yearly", due.Format("on 2 Jan"), "RRULE:FREQ=YEARLY;INTERVAL=1"},
		{"Never", "", ""},
	}
	cur := cmp.Or(store.RepeatLabel(t.RepeatFlag), "Never")
	var items []cmdItem
	for _, o := range opts {
		it := cmdItem{icon: a.icon("\uf01e", "~"), iconRole: "secondary", label: o.label, hint: o.hint,
			run: func() tea.Cmd {
				a.setFlash("repeat → " + strings.ToLower(o.label))
				return a.setRepeat(t, o.rule, false)
			}}
		if o.rule == "" {
			it.icon, it.iconRole = "×", "dim"
		}
		if o.label == cur {
			it.hint = strings.TrimSpace(o.hint + " · current")
		}
		items = append(items, it)
	}
	custom := cmdItem{icon: "…", iconRole: "dim", label: "Custom", hint: "3rd wed, last workday, every 2 weeks",
		run: func() tea.Cmd { a.startEdit("repeat"); return nil }}
	if cur == "Custom" {
		custom.hint = parse.RepeatText(t.RepeatFlag, t.RepeatFrom == "1") + " · current"
	}
	a.openPick("repeat", append(items, custom))
}

func (a *App) setRepeat(t *api.Task, rule string, fromCompletion bool) tea.Cmd {
	from := map[bool]string{false: "0", true: "1"}[fromCompletion]
	return a.update(t, map[string]any{"repeatFlag": rule, "repeatFrom": from}, func(t *api.Task) { t.RepeatFlag, t.RepeatFrom = rule, from })
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
		if key, notes := t.Notes(); v != *notes {
			cmd = a.update(t, map[string]any{key: v}, func(t *api.Task) { _, n := t.Notes(); *n = v })
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
	case "repeat":
		rule, from := "", false
		if strings.TrimSpace(v) != "" { // empty: never
			var err error
			if rule, from, err = parse.ParseRepeat(v, time.Now()); err != nil {
				a.setFlash(err.Error() + " · try: " + parse.RepeatHelp)
				return nil, false
			}
		}
		cmd = a.setRepeat(t, rule, from)
	case "additem":
		if v = strings.TrimSpace(v); v != "" {
			cmd = a.addItem(t, v)
		}
	default: // checklist item "c<i>"
		var i int
		fmt.Sscanf(field, "c%d", &i)
		if v = strings.TrimSpace(v); v == "" {
			a.setFlash("item can't be empty")
			return nil, false
		}
		if i < len(t.Items) && v != t.Items[i].Title {
			items := slices.Clone(t.Items)
			items[i].Title = v
			cmd = a.update(t, map[string]any{"items": items}, func(t *api.Task) { t.Items = items })
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
	case "repeat":
		if t.DueDate == "" {
			a.setFlash("set a due date first · repeat needs one")
			return
		}
		if t.RepeatFlag != "" {
			v = parse.RepeatText(t.RepeatFlag, t.RepeatFrom == "1")
		}
	case "tags":
		if len(t.Tags) > 0 {
			v = "#" + strings.Join(t.Tags, " #")
		}
	case "notes":
		_, notes := t.Notes()
		v = *notes
	case "additem":
	default:
		var i int
		if _, err := fmt.Sscanf(field, "c%d", &i); err != nil || i >= len(t.Items) {
			return
		}
		v = t.Items[i].Title
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
	case k == "title" || k == "tags" || k == "notes" || k == "additem" || strings.HasPrefix(k, "c"):
		a.startEdit(k)
	case k == "due":
		a.duePicker(t)
	case k == "repeat":
		a.repeatPicker(t)
	case k == "priority":
		return a.cyclePrio(t)
	case k == "list":
		a.movePicker(t)
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
