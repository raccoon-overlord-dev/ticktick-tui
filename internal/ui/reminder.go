package ui

import (
	"os/exec"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/raccoon-overlord-dev/ticktick-tui/internal/api"
	"github.com/raccoon-overlord-dev/ticktick-tui/internal/parse"
	"github.com/raccoon-overlord-dev/ticktick-tui/internal/store"
)

// reminderPicker opens the Reminder menu (⏎ on the Remind field), like the web app's:
// presets toggle on and off (a task can have several), Custom opens the dialog.
func (a *App) reminderPicker(t *api.Task) {
	if _, ok := store.DueTime(t); !ok {
		a.setFlash("set a due date first · reminders need one")
		return
	}
	var presets []time.Duration
	if t.IsAllDay {
		h, m, _ := parse.ParseTime(a.cfg.Reminders.DefaultTime)
		for _, days := range []int{0, 1, 2, 3, 7} {
			presets = append(presets, parse.AllDayTrigger(days, h, m))
		}
	} else {
		presets = []time.Duration{0, -5 * time.Minute, -30 * time.Minute, -time.Hour, -24 * time.Hour}
	}
	trigs := make([]string, 0, len(presets))
	for _, d := range presets {
		trigs = append(trigs, parse.Trigger(d, t.IsAllDay))
	}
	for _, r := range t.Reminders { // set elsewhere: listed so they can be turned off
		if !slices.Contains(trigs, r) {
			trigs = append(trigs, r)
		}
	}
	var items []cmdItem
	for i, trig := range trigs {
		it := cmdItem{icon: a.icon("", "!"), iconRole: "dim", label: parse.ReminderText(trig, t.IsAllDay),
			run: func() tea.Cmd {
				cmd := a.toggleReminder(t, trig)
				a.reminderPicker(t) // stay open to pick more
				a.cmd.idx = i
				return cmd
			}}
		if slices.Contains(t.Reminders, trig) {
			it.iconRole, it.hint = "ok", "✓ on · ⏎ off"
		}
		items = append(items, it)
	}
	items = append(items, cmdItem{icon: "…", iconRole: "dim", label: "Custom", hint: "days or weeks early, at a time",
		run: func() tea.Cmd { a.openReminderForm(t); return nil }})
	a.openPick("remind", items)
}

func (a *App) toggleReminder(t *api.Task, trig string) tea.Cmd {
	return a.setReminders(t, toggle(slices.Clone(t.Reminders), trig))
}

// setReminders sends the task's whole reminder list. The API can't remove the last one
// (an empty list only drops the first; see docs/api-notes.md), so that is refused.
func (a *App) setReminders(t *api.Task, rs []string) tea.Cmd {
	if len(rs) == 0 && t != a.draft {
		a.flashError("✗ the API can't remove a task's last reminder · remove it in the TickTick app")
		return nil
	}
	return a.update(t, map[string]any{"reminders": rs}, func(t *api.Task) {
		t.Reminders = nil
		if len(rs) > 0 {
			t.Reminders = rs
		}
	})
}

// remForm is the Custom reminder dialog: N days or weeks early at a time (all-day tasks),
// or N minutes / hours / days early (timed tasks), with a preview of when it goes off.
type remForm struct {
	task   *api.Task
	unit   string
	n      int
	at     int    // minutes after midnight (all-day)
	atBuf  string // digits typed into the time row
	row    int
	allDay bool
}

func (a *App) openReminderForm(t *api.Task) {
	h, m, _ := parse.ParseTime(a.cfg.Reminders.DefaultTime)
	f := &remForm{task: t, allDay: t.IsAllDay, unit: "day", n: 1, at: h*60 + m}
	if !t.IsAllDay {
		f.unit, f.n = "minute", 30
	}
	a.rem = f
}

func (f *remForm) units() []string {
	if f.allDay {
		return []string{"day", "week"}
	}
	return []string{"minute", "hour", "day"}
}

func (f *remForm) rows() []string {
	if f.allDay {
		return []string{"unit", "n", "at", "ok"}
	}
	return []string{"unit", "n", "ok"}
}

func (f *remForm) offset() time.Duration {
	if f.allDay {
		days := f.n
		if f.unit == "week" {
			days *= 7
		}
		return parse.AllDayTrigger(days, f.at/60, f.at%60)
	}
	return -time.Duration(f.n) * map[string]time.Duration{"minute": time.Minute, "hour": time.Hour, "day": 24 * time.Hour}[f.unit]
}

func (a *App) remKey(k tea.KeyPressMsg) tea.Cmd {
	f := a.rem
	rows := f.rows()
	row := rows[min(f.row, len(rows)-1)]
	key := k.String()
	if a.cfg.Keys.Keymap != "arrows" {
		if alias, ok := map[string]string{"j": "down", "k": "up", "h": "left", "l": "right"}[key]; ok {
			key = alias
		}
	}
	switch key {
	case "esc", "q":
		a.rem = nil
	case "ctrl+s":
		return a.saveReminder()
	case "up", "down", "tab", "shift+tab":
		f.row, f.atBuf = step(f.row, map[bool]int{true: -1, false: 1}[key == "up" || key == "shift+tab"], len(rows)), ""
	case "left", "right":
		dir := map[bool]int{true: -1, false: 1}[key == "left"]
		switch row {
		case "unit":
			cycle(&f.unit, f.units(), dir)
		case "n":
			f.n = max(0, f.n+dir)
		case "at":
			f.at, f.atBuf = ((f.at+15*dir)%1440+1440)%1440, ""
		}
	case "enter", "space":
		if row == "ok" {
			return a.saveReminder()
		}
		if row == "unit" {
			cycle(&f.unit, f.units(), 1)
		}
	case "backspace":
		switch row {
		case "n":
			f.n /= 10
		case "at":
			f.atBuf = ""
		}
	default:
		d := int(k.Code - '0')
		if len(key) != 1 || d < 0 || d > 9 {
			return nil
		}
		switch row {
		case "n":
			if f.n = f.n*10 + d; f.n > 999 {
				f.n = d
			}
		case "at": // HHMM, applied once 4 digits are in
			if f.atBuf += key; len(f.atBuf) == 4 {
				h, _ := strconv.Atoi(f.atBuf[:2])
				m, _ := strconv.Atoi(f.atBuf[2:])
				if h < 24 && m < 60 {
					f.at = h*60 + m
				}
				f.atBuf = ""
			}
		}
	}
	return nil
}

func (a *App) saveReminder() tea.Cmd {
	f := a.rem
	a.rem = nil
	trig := parse.Trigger(f.offset(), f.allDay)
	if slices.Contains(f.task.Reminders, trig) {
		return nil
	}
	a.setFlash("reminder → " + parse.ReminderText(trig, f.allDay))
	return a.setReminders(f.task, append(slices.Clone(f.task.Reminders), trig))
}

// viewReminder renders the Custom reminder dialog and returns it with its size.
func (a *App) viewReminder() (string, int, int) {
	f := a.rem
	p := a.overlayPen()
	w := min(52, a.w-4)
	var lines []string
	sel := 0
	for i, row := range f.rows() {
		on := i == f.row
		rp := p
		if on {
			sel, rp = len(lines), p.on(a.selBg(true))
		}
		label := func(s string) string { return rp.s("muted").Render(padRight(s, 15)) }
		switch row {
		case "unit":
			u := strings.ToUpper(f.unit[:1]) + f.unit[1:]
			v := rp.s("text").Render(u)
			if on {
				v = rp.s("accent").Render("‹ ") + rp.s("text").Bold(true).Render(u) + rp.s("accent").Render(" ›")
			}
			lines = append(lines, rp.line(w, label("Remind by")+v, ""))
		case "n":
			lines = append(lines, rp.line(w, label(f.unit+"s early")+rp.s("text").Bold(on).Render(strconv.Itoa(f.n)), hintIf(rp, on, "←→ or type")))
		case "at":
			v := parse.FmtTime(time.Date(2000, 1, 1, f.at/60, f.at%60, 0, 0, time.UTC))
			if f.atBuf != "" {
				v = f.atBuf + strings.Repeat("_", 4-len(f.atBuf))
			}
			lines = append(lines, rp.line(w, label("Remind me at")+rp.s("text").Bold(on).Render(v), hintIf(rp, on, "←→ 15 min or type HHMM")))
		case "ok":
			lines = append(lines, "", rp.line(w, rp.s("accent").Bold(true).Render("[ OK ]"), ""))
			if on {
				sel = len(lines) - 1
			}
		}
	}
	summary := ""
	if due, ok := store.DueTime(f.task); ok {
		at := parse.FireTime(due, f.allDay, f.offset())
		summary = "Remind at " + parse.FmtTime(at) + " on " + parse.FmtLongDate(at)
	}
	lines = append(lines, "", p.line(w, p.s("secondary").Render(summary), ""), p.s("line").Render(strings.Repeat("─", w-2)),
		p.line(w, p.s("dim").Render("↑↓ field · ←→ change · ctrl+s add"), ""))
	body := window(lines, sel, max(a.h-3, 3), new(int))
	h := len(body) + 2
	return a.frame(p, frameOpts{w: w, h: h, title: "Custom reminder", modal: true, body: body}), w, h
}

// checkReminders notifies the reminders that came due since the last check (Settings →
// Notifications). Only while ttui is open: reminders missed while it was closed are skipped.
func (a *App) checkReminders() tea.Cmd {
	mode := a.cfg.Reminders.Notify
	if mode == "" || mode == "off" || a.st == nil {
		a.remChecked = time.Time{}
		return nil
	}
	now := time.Now()
	from := a.remChecked
	a.remChecked = now
	if from.IsZero() { // just turned on or started: nothing is "missed"
		return nil
	}
	var cmds []tea.Cmd
	for i := range a.st.Tasks {
		t := &a.st.Tasks[i]
		due, ok := store.DueTime(t)
		if !ok || store.Done(t) {
			continue
		}
		for _, r := range t.Reminders {
			d, ok := parse.ParseTrigger(r)
			if at := parse.FireTime(due, t.IsAllDay, d); ok && at.After(from) && !at.After(now) {
				body := "due"
				if l := store.DueLabel(t, now); l != nil {
					body = "due " + strings.ToLower(l.Long[:1]) + l.Long[1:]
				}
				a.setFlash("🔔 " + t.Title + " · " + body)
				cmds = append(cmds, notify(mode, t.Title, body))
			}
		}
	}
	return tea.Batch(cmds...)
}

// notify shows a notification through the terminal (OSC 9: Ghostty, iTerm2, WezTerm, kitty…)
// or the OS (notify-send on Linux, osascript on macOS).
func notify(mode, title, body string) tea.Cmd {
	title, body = plain(title), plain(body)
	if mode == "terminal" {
		return tea.Raw(ansi.Notify(title + " · " + body))
	}
	return func() tea.Msg {
		var err error
		switch runtime.GOOS {
		case "darwin": // text passed as arguments, never inside the script
			err = exec.Command("osascript", "-e", "on run argv", "-e", "display notification (item 2 of argv) with title (item 1 of argv)",
				"-e", "end run", title, body).Run()
		default:
			err = exec.Command("notify-send", "-a", "ttui", title, body).Run()
		}
		if err != nil {
			return flashMsg("✗ notification failed: " + err.Error())
		}
		return nil
	}
}

// plain drops control characters, so a task title can't end the escape sequence early.
func plain(s string) string {
	return strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f || (r >= 0x80 && r < 0xa0) {
			return -1
		}
		return r
	}, s)
}
