package ui

import (
	"os"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/raccoon-overlord-dev/ticktick-tui/internal/config"
	"github.com/raccoon-overlord-dev/ticktick-tui/internal/theme"
	"github.com/raccoon-overlord-dev/ticktick-tui/internal/update"
)

type setRow struct {
	sec   string // section header row when set
	key   string
	label string
	opts  []string
	desc  string
}

var setRows = []setRow{
	{sec: "APPEARANCE"},
	{key: "theme", label: "Theme", opts: []string{"terminal", "colorful", "lotr"}, desc: "terminal inherits your emulator’s 16 ANSI colors"},
	{key: "prio", label: "Group headers", opts: []string{"rule", "label", "tab"}, desc: "How task groups are separated in the task list"},
	{key: "focus", label: "Focused panel", opts: []string{"border", "title"}, desc: "Accent border, or an inverted title chip"},
	{key: "bg", label: "Background", opts: []string{"solid", "transparent", "blur"}, desc: "transparent/blur: ttui paints no background, so your terminal’s opacity + blur show through"},
	{key: "icons", label: "Nerd Font icons", opts: []string{"on", "off"}, desc: "Off falls back to plain ASCII glyphs"},
	{sec: "LAYOUT"},
	{key: "columns", label: "Columns", opts: []string{"auto", "3", "2", "1"}, desc: "auto: 3 panes ≥ 120 cols · 2 panes ≥ 80 · 1 pane below"},
	{key: "completed", label: "Show completed", opts: []string{"on", "off"}, desc: "Completed tasks collapse into a group at the end"},
	{sec: "TASKS"},
	{key: "due", label: "Due date on task row", opts: []string{"on", "off"}, desc: "Right-aligned due label on each task (t toggles)"},
	{key: "group", label: "Group by", opts: groupOpts, desc: "Default for all lists · s in the task list sets one list's own"},
	{key: "sort", label: "Sort by", opts: sortOpts, desc: "Default for all lists · s in the task list sets one list's own"},
	{key: "order", label: "Order", opts: orderOpts, desc: "Default for all lists · s in the task list sets one list's own"},
	{key: "smartdates", label: "Smart dates", opts: []string{"on", "off"}, desc: "Quick add (a) reads tomorrow, fri, 17:00… as the due date; off keeps them in the title"},
	{key: "history", label: "Completed history", opts: []string{"7", "30", "90", "365"}, desc: "Days of completed tasks to download (Completed list and groups); changing it syncs"},
	{key: "week", label: "Week starts on", opts: []string{"mon", "sun"}, desc: ""},
	{sec: "KEYS"},
	{key: "keymap", label: "Keymap", opts: []string{"vim + arrows", "arrows only"}, desc: "vim: hjkl, / search, : command · arrows + letters always work"},
	{sec: "ACCOUNT"},
	{key: "account", label: "Signed in", desc: "Enter signs out and returns to the login screen"},
	{key: "sync", label: "Sync every", opts: []string{"1m", "5m", "15m", "manual"}, desc: "Background two-way sync with TickTick"},
	{key: "updates", label: "Check for updates", opts: []string{"on", "off"}, desc: "Once a day; U installs a new release and restarts"},
	{key: "checknow", label: "Check now", desc: "Look for a new release on GitHub now"},
	{sec: "INFO"},
	{key: "about", label: "About", desc: "Version and project page"},
}

// field returns a pointer to the config string behind key, or nil for bool rows.
func (a *App) field(key string) *string {
	c := a.cfg
	return map[string]*string{
		"theme": &c.Appearance.Theme, "prio": &c.Appearance.PriorityHeaders, "focus": &c.Appearance.FocusedPanel,
		"bg": &c.Appearance.Background, "columns": &c.Layout.Columns, "group": &c.Tasks.GroupBy,
		"sort": &c.Tasks.SortBy, "order": &c.Tasks.Order,
		"week": &c.Tasks.WeekStart, "history": &c.Tasks.CompletedDays, "keymap": &c.Keys.Keymap, "sync": &c.Account.SyncEvery,
	}[key]
}

func (a *App) flag(key string) *bool {
	c := a.cfg
	return map[string]*bool{"icons": &c.Appearance.NerdFontIcons, "completed": &c.Layout.ShowCompleted, "due": &c.Tasks.DueLabel, "smartdates": &c.Tasks.SmartDates, "updates": &c.Account.UpdateCheck}[key]
}

// keymap values in config.toml differ from their labels.
var keymapValue = map[string]string{"vim + arrows": "vim+arrows", "arrows only": "arrows"}

func (a *App) settingValue(r setRow) string {
	if b := a.flag(r.key); b != nil {
		return onOff(*b)
	}
	v := *a.field(r.key)
	for label, val := range keymapValue {
		if r.key == "keymap" && val == v {
			return label
		}
	}
	return v
}

func (a *App) changeSetting(r setRow, dir int) tea.Cmd {
	if r.key == "account" {
		if dir > 0 {
			return a.signOut()
		}
		return nil
	}
	if r.key == "about" {
		a.about = dir > 0
		return nil
	}
	if r.key == "checknow" {
		switch {
		case dir < 0:
		case a.newVersion != "":
			a.askUpdate()
		default:
			return a.checkUpdate(true)
		}
		return nil
	}
	i := slices.Index(r.opts, a.settingValue(r))
	return a.setOption(r.key, r.opts[((i+dir)%len(r.opts)+len(r.opts))%len(r.opts)])
}

// setOption applies a setting (by settings-row key and option label) live and saves config.toml.
func (a *App) setOption(key, opt string) tea.Cmd {
	r := setRow{key: key}
	if b := a.flag(r.key); b != nil {
		*b = opt == "on"
	} else if r.key == "keymap" {
		*a.field(r.key) = keymapValue[opt]
	} else {
		*a.field(r.key) = opt
	}
	if r.key == "theme" {
		th, err := theme.Load(opt)
		if err != nil {
			a.setFlash(err.Error())
			return nil
		}
		a.th = th
	}
	a.save()
	switch r.key {
	case "sync":
		return a.scheduleSync()
	case "history":
		a.setFlash("⟳ syncing " + opt + " days of completed tasks…")
		return a.syncNow()
	case "updates":
		a.newVersion = ""
		return a.checkUpdate(false)
	}
	return nil
}

func (a *App) settingsKey(k tea.KeyPressMsg) tea.Cmd {
	if a.about {
		if key := k.String(); key == "esc" || key == "enter" || key == "q" || key == "," {
			a.about = false
		}
		return nil
	}
	move := func(d int) { // wraps around, skipping section headers
		for i := (a.sIdx + d + len(setRows)) % len(setRows); i != a.sIdx; i = (i + d + len(setRows)) % len(setRows) {
			if setRows[i].sec == "" {
				a.sIdx = i
				return
			}
		}
	}
	vim := a.cfg.Keys.Keymap != "arrows"
	switch key := k.String(); {
	case key == "down" || (vim && key == "j"):
		move(1)
	case key == "up" || (vim && key == "k"):
		move(-1)
	case key == "right" || (vim && key == "l") || key == "enter" || key == "space":
		return a.changeSetting(setRows[a.sIdx], 1)
	case key == "left" || (vim && key == "h"):
		return a.changeSetting(setRows[a.sIdx], -1)
	case key == "esc" || key == "," || key == "q":
		a.settings = false
	}
	return nil
}

// viewSettings renders the settings modal and returns it with its size.
func (a *App) viewSettings() (string, int, int) {
	p := a.overlayPen()
	w := min(72, a.w-4)
	var lines []string
	sel := 0
	for i, r := range setRows {
		if r.sec != "" {
			if i > 0 {
				lines = append(lines, "")
			}
			lines = append(lines, p.line(w, p.s("dim").Render(r.sec), ""))
			continue
		}
		rp := p
		if i == a.sIdx {
			sel = len(lines)
			rp = p.on(a.selBg(true))
		}
		label := rp.s("text").Render(padRight(r.label, 22))
		var val string
		if r.key == "account" {
			email := "signed in"
			if a.st != nil && a.st.Email != "" {
				email = a.st.Email
			}
			val = rp.s("text").Render(email) + rp.sp(2) + rp.s("error").Render("⏎ sign out")
		} else if r.key == "about" {
			val = rp.s("dim").Render("⏎ open")
		} else if r.key == "checknow" && a.newVersion != "" {
			val = rp.s("info").Render("⏎ update to " + a.newVersion)
		} else if r.key == "checknow" {
			val = rp.s("dim").Render("⏎ check · ttui " + a.version)
		} else {
			cur := a.settingValue(r)
			for j, o := range r.opts {
				if j > 0 {
					val += rp.sp(2)
				}
				if o == cur {
					val += rp.s("accent").Bold(true).Underline(true).Render(o)
				} else {
					val += rp.s("dim").Render(o)
				}
			}
		}
		lines = append(lines, rp.line(w, label+val, ""))
	}
	desc := setRows[a.sIdx].desc
	footer := []string{
		p.s("line").Render(strings.Repeat("─", w-2)),
		p.line(w, p.s("muted").Render(trunc(desc, w-4)), ""),
		p.line(w, p.s("dim").Render("j/k move · h/l change · esc close"), ""),
	}
	maxBody := max(a.h-1-2-len(footer), 1)
	body := append(window(lines, sel, maxBody, new(int)), footer...)
	h := len(body) + 2

	path := config.Path
	if home, err := os.UserHomeDir(); err == nil {
		path = strings.Replace(path, home, "~", 1)
	}
	return a.frame(p, frameOpts{w: w, h: h, title: "Settings", modal: true, topRight: path, body: body}), w, h
}

func padRight(s string, n int) string {
	if w := len([]rune(s)); w < n {
		return s + strings.Repeat(" ", n-w)
	}
	return s
}

// helpKeys is the ? panel. Keep it in sync with the Keys table in docs/usage.md.
var helpKeys = []struct{ sec, key, desc string }{
	{sec: "MOVE"},
	{key: "j k  ↓ ↑", desc: "Move; in lists, moving selects the list"},
	{key: "g G", desc: "Top / bottom"},
	{key: "pgup pgdn", desc: "Page up / down"},
	{key: "h l  ← →", desc: "Previous / next pane; l opens"},
	{key: "1 2 3", desc: "Focus lists / tasks / details (1 = @ if narrow)"},
	{key: "tab", desc: "Next pane"},
	{key: "⏎", desc: "Open; on a field: edit, pick or cycle"},
	{key: "esc", desc: "Back"},
	{key: "␣ ⏎ l", desc: "On a folder: open / close it (remembered)"},
	{key: "H", desc: "On a list: hide from / show in smart lists"},
	{sec: "TASKS"},
	{key: "x  ␣", desc: "Complete / reopen (x right after: undo); tick an item"},
	{key: "p", desc: "Cycle priority"},
	{key: "i e", desc: "Edit the title or the field under the cursor"},
	{key: "d", desc: "Due date menu (Pick a date: calendar)"},
	{key: "m", desc: "Move to another list"},
	{key: "c", desc: "Add checklist items"},
	{key: "D  del", desc: "Delete (asks first; repeating: this one or all)"},
	{key: "t", desc: "Toggle due-date labels"},
	{key: "s", desc: "Group / sort this list"},
	{sec: "EVERYWHERE"},
	{key: "a", desc: "Quick add (one line)"},
	{key: "n", desc: "New task panel (all fields)"},
	{key: "/  ctrl+k", desc: "Search"},
	{key: ":", desc: "Commands"},
	{key: "@ #", desc: "Jump to a list / tag"},
	{key: "ctrl+r", desc: "Sync now"},
	{key: "U", desc: "Update ttui (when the status bar shows ↑)"},
	{key: ",", desc: "Settings"},
	{key: "?", desc: "This panel"},
	{key: "q", desc: "Quit"},
	{sec: "EDITING"},
	{key: "⏎  esc", desc: "Save"},
	{key: "notes", desc: "⏎ newline · ↑↓ line · esc or ctrl+s save"},
	{key: "checklist", desc: "⏎ add and type the next · esc done"},
	{key: "ctrl+a", desc: "Select all (outside editing: edit with all selected)"},
	{key: "shift+←→", desc: "Select (shift+↑↓ in notes)"},
	{key: "ctrl+c x", desc: "Copy / cut the selection"},
	{key: "ctrl+z y", desc: "Undo / redo (until the field is saved)"},
}

func (a *App) helpKey(k tea.KeyPressMsg) tea.Cmd {
	vim := a.cfg.Keys.Keymap != "arrows"
	switch key := k.String(); {
	case key == "down" || (vim && key == "j"):
		a.offHelp++
	case key == "up" || (vim && key == "k"):
		a.offHelp = max(a.offHelp-1, 0)
	case key == "pgdown":
		a.offHelp += a.page()
	case key == "pgup":
		a.offHelp = max(a.offHelp-a.page(), 0)
	case key == "esc" || key == "?" || key == "q":
		a.help = false
	}
	return nil
}

// viewHelp renders the shortcuts panel and returns it with its size.
func (a *App) viewHelp() (string, int, int) {
	p := a.overlayPen()
	w := min(64, a.w-4)
	var lines []string
	for i, r := range helpKeys {
		if r.sec != "" {
			if i > 0 {
				lines = append(lines, "")
			}
			lines = append(lines, p.line(w, p.s("dim").Render(r.sec), ""))
			continue
		}
		lines = append(lines, p.line(w, p.s("accent").Render(padRight(r.key, 12))+p.s("text").Render(trunc(r.desc, w-4-12)), ""))
	}
	body := window(lines, -1, max(a.h-1-2, 1), &a.offHelp)
	h := len(body) + 2
	return a.frame(p, frameOpts{w: w, h: h, title: "Keys", modal: true, topRight: "esc close", body: body}), w, h
}

// viewAbout renders the About box (Settings → About): version and project link.
func (a *App) viewAbout() (string, int, int) {
	p := a.overlayPen()
	w := min(64, a.w-4)
	row := func(label, val string) string {
		return p.line(w, p.s("dim").Render(padRight(label, 10))+val, "")
	}
	body := []string{
		"",
		row("Version", p.s("text").Render(a.version)),
		row("GitHub", linkify(p.s("text"), p.s("info"), update.Repo, w-4-10)),
		"",
	}
	h := len(body) + 2
	return a.frame(p, frameOpts{w: w, h: h, title: "About ttui", modal: true, topRight: "esc close", body: body}), w, h
}
