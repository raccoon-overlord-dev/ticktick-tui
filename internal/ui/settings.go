package ui

import (
	"os"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"

	"ttui/internal/config"
	"ttui/internal/theme"
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
	{key: "prio", label: "Priority headers", opts: []string{"rule", "label", "tab"}, desc: "How priority groups are separated in the task list"},
	{key: "focus", label: "Focused panel", opts: []string{"border", "title"}, desc: "Accent border, or an inverted title chip"},
	{key: "bg", label: "Background", opts: []string{"solid", "transparent", "blur"}, desc: "transparent/blur: ttui paints no background, so your terminal’s opacity + blur show through"},
	{key: "icons", label: "Nerd Font icons", opts: []string{"on", "off"}, desc: "Off falls back to plain ASCII glyphs"},
	{sec: "LAYOUT"},
	{key: "columns", label: "Columns", opts: []string{"auto", "3", "2", "1"}, desc: "auto: 3 panes ≥ 120 cols · 2 panes ≥ 80 · 1 pane below"},
	{key: "completed", label: "Show completed", opts: []string{"on", "off"}, desc: "Completed tasks collapse into a group at the end"},
	{sec: "TASKS"},
	{key: "due", label: "Due date on task row", opts: []string{"on", "off"}, desc: "Right-aligned due label on each task (t toggles)"},
	{key: "sort", label: "Sort inside priority", opts: []string{"due", "title", "created"}, desc: "Order of tasks within each priority group"},
	{key: "week", label: "Week starts on", opts: []string{"mon", "sun"}, desc: ""},
	{sec: "KEYS"},
	{key: "keymap", label: "Keymap", opts: []string{"vim + arrows", "arrows only"}, desc: "vim: hjkl, / search, : command · arrows + letters always work"},
	{sec: "ACCOUNT"},
	{key: "account", label: "Signed in", desc: "Enter signs out and returns to the login screen"},
	{key: "sync", label: "Sync every", opts: []string{"1m", "5m", "15m", "manual"}, desc: "Background two-way sync with TickTick"},
}

// field returns a pointer to the config string behind key, or nil for bool rows.
func (a *App) field(key string) *string {
	c := a.cfg
	return map[string]*string{
		"theme": &c.Appearance.Theme, "prio": &c.Appearance.PriorityHeaders, "focus": &c.Appearance.FocusedPanel,
		"bg": &c.Appearance.Background, "columns": &c.Layout.Columns, "sort": &c.Tasks.SortInPriority,
		"week": &c.Tasks.WeekStart, "keymap": &c.Keys.Keymap, "sync": &c.Account.SyncEvery,
	}[key]
}

func (a *App) flag(key string) *bool {
	c := a.cfg
	return map[string]*bool{"icons": &c.Appearance.NerdFontIcons, "completed": &c.Layout.ShowCompleted, "due": &c.Tasks.DueLabel}[key]
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
	if r.key == "sync" {
		return a.scheduleSync()
	}
	return nil
}

func (a *App) settingsKey(k tea.KeyPressMsg) tea.Cmd {
	move := func(d int) {
		for i := a.sIdx + d; i >= 0 && i < len(setRows); i += d {
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
