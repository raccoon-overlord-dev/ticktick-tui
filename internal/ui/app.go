// Package ui holds the Bubble Tea models for ttui.
package ui

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"ttui/internal/api"
	"ttui/internal/auth"
	"ttui/internal/config"
	"ttui/internal/store"
	"ttui/internal/theme"
)

// Run starts the TUI and blocks until it exits.
func Run(version string) error { return run(version, nil) }

// RunDemo starts the TUI on st (e.g. store.Demo) without signing in or calling the API.
func RunDemo(version string, st *store.Store) error { return run(version, st) }

func run(version string, demo *store.Store) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}
	th, err := theme.Load(cfg.Appearance.Theme)
	if err != nil {
		if th, err = theme.Load("terminal"); err != nil {
			return err
		}
	}
	signed, err := auth.Load()
	if err != nil {
		return fmt.Errorf("auth.toml: %w", err)
	}
	a := &App{version: version, cfg: cfg, th: th, signed: signed, now: time.Now(),
		focus: "tasks", list: "today", sideKey: "l:today", folded: map[string]bool{}, idMap: map[string]string{}}
	if demo != nil {
		a.signed, a.st, a.demo = &auth.Auth{}, demo, true
	}
	_, err = tea.NewProgram(a).Run()
	return err
}

type screen int

const (
	screenAuth screen = iota
	screenMain
)

type App struct {
	version string
	cfg     *config.Config
	th      *theme.Theme
	w, h    int
	now     time.Time
	spin    int // spinner frame

	screen screen
	signed *auth.Auth
	st     *store.Store // nil until the first sync
	demo   bool         // `ttui dev demo`: fake data, no API
	err    string       // main-screen sync error

	auth authScreen

	// main screen
	focus                         string // lists | tasks | detail
	list                          string // active list id (see store.TasksFor)
	sideKey                       string // lists cursor: "l:<list>" or "f:<groupId>"
	taskID                        string
	df                            int  // detail field index
	sheet                         bool // 1-pane detail sheet open
	folded                        map[string]bool
	settings                      bool
	sIdx                          int
	offLists, offTasks, offDetail int
	cmd                           *cmdBar // command bar, nil when closed
	edit, editID                  string  // inline edit: field (title|due|tags|notes) and task
	in                            textInput
	recent                        []string // recently opened task ids (command bar RECENT)

	// writes and sync
	queue    []op
	busy     bool              // an op is in flight
	idMap    map[string]string // temporary id → server id for created tasks
	tmpN     int
	lastDone string // for "x to undo"
	syncGen  int
	syncing  bool

	flash      string
	flashRole  string
	flashUntil time.Time
}

type (
	tickMsg  time.Time
	flashMsg string
	storeMsg struct {
		st      *store.Store
		err     error
		pending *auth.Auth // auth to save if the fetch succeeds (sign-in); nil on startup
	}
)

// tick drives the clock, "synced ago" and flash expiry once a second; it runs
// at spinner speed only while a spinner is on screen (auth, first sync), since
// every tick re-renders the whole view.
func (a *App) tick() tea.Cmd {
	d := time.Second
	if a.screen == screenAuth || a.st == nil {
		d = 110 * time.Millisecond
	}
	return tea.Tick(d, func(t time.Time) tea.Msg { return tickMsg(t) })
}

// Init also asks the terminal for Unicode mode 2027. Bubble Tea only asks by itself
// outside SSH or for a few known TERMs; without a reply its renderer counts emoji
// like ⚠️ as 1 cell while lipgloss and the terminal use 2, so rows spill into the
// next column. A reply switches the renderer to grapheme widths.
func (a *App) Init() tea.Cmd {
	return tea.Batch(tea.Raw(ansi.RequestModeUnicodeCore), a.start())
}

func (a *App) start() tea.Cmd {
	if a.demo {
		a.screen = screenMain
		return a.tick()
	}
	if a.signed != nil {
		a.screen = screenMain
		a.st = store.LoadSnapshot() // render at once, then refresh
		return tea.Batch(a.tick(), a.syncNow())
	}
	return tea.Batch(a.tick(), a.auth.start())
}

func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.w, a.h = msg.Width, msg.Height
		return a, nil
	case tickMsg:
		a.now, a.spin = time.Time(msg), a.spin+1
		if a.flash != "" && a.now.After(a.flashUntil) {
			a.flash = ""
		}
		return a, a.tick()
	case flashMsg:
		a.setFlash(string(msg))
		return a, nil
	case storeMsg:
		return a, a.onStore(msg)
	case opDoneMsg:
		return a, a.onOpDone(msg)
	case syncTickMsg:
		if int(msg) == a.syncGen {
			return a, a.syncNow()
		}
		return a, nil
	case tea.PasteMsg:
		switch {
		case a.cmd != nil:
			a.cmd.in.insert(strings.ReplaceAll(msg.Content, "\n", " "))
		case a.edit != "":
			a.in.insert(msg.Content)
		}
	case tea.KeyPressMsg:
		if msg.String() == "ctrl+c" {
			a.auth.cancel()
			return a, tea.Quit
		}
	}
	if a.screen == screenAuth {
		return a, a.updateAuth(msg)
	}
	if k, ok := msg.(tea.KeyPressMsg); ok {
		switch {
		case a.st == nil:
			if k.String() == "q" {
				return a, tea.Quit
			}
		case a.cmd != nil:
			return a, a.cmdKey(k)
		case a.edit != "":
			return a, a.editKey(k)
		case a.settings:
			return a, a.settingsKey(k)
		default:
			return a, a.mainKey(k)
		}
	}
	return a, nil
}

func (a *App) setFlash(s string) {
	a.flash, a.flashRole, a.flashUntil = s, "ok", time.Now().Add(2600*time.Millisecond)
}

func (a *App) editKey(k tea.KeyPressMsg) tea.Cmd {
	key := k.String()
	switch {
	case key == "esc" || key == "ctrl+s" || key == "ctrl+enter" || (key == "enter" && a.edit != "notes"):
		cmd, _ := a.commitEdit()
		return cmd
	}
	a.in.key(k, a.edit == "notes")
	return nil
}

func (a *App) onStore(msg storeMsg) tea.Cmd {
	var apiErr *api.Error
	unauthorized := errors.As(msg.err, &apiErr) && apiErr.Status == 401

	if msg.pending == nil { // background or startup sync
		a.syncing = false
		switch {
		case a.signed == nil: // signed out meanwhile
			return nil
		case unauthorized:
			return a.expired()
		case msg.err != nil && a.st == nil:
			a.err = msg.err.Error()
		case msg.err != nil:
			a.flashError("⟳ sync failed: " + shortErr(msg.err))
		case a.busy || len(a.queue) > 0:
			// local edits in flight; the next sync picks up the server state
		default:
			a.st, a.err = msg.st, ""
			a.st.Save()
		}
		return a.scheduleSync()
	}

	// sign-in attempt from the auth screen
	a.auth.checking = false
	if msg.err != nil {
		if msg.pending.Method == "token" {
			if unauthorized {
				a.auth.tokenErr = "✗ Token rejected (401). Check it and try again."
			} else {
				a.auth.tokenErr = "✗ " + msg.err.Error()
			}
		} else {
			a.auth.oauthErr = msg.err.Error()
		}
		return nil
	}
	if err := auth.Save(msg.pending); err != nil {
		a.auth.tokenErr, a.auth.oauthErr = "✗ "+err.Error(), err.Error()
		return nil
	}
	a.auth.cancel()
	a.signed, a.st, a.auth.success = msg.pending, msg.st, true
	a.st.Save()
	return a.scheduleSync()
}

func (a *App) signOut() tea.Cmd {
	if a.demo {
		a.setFlash("demo mode: not signed in")
		return nil
	}
	if err := auth.SignOut(); err != nil {
		a.setFlash("sign out failed: " + err.Error())
		return nil
	}
	a.signed, a.st, a.screen, a.auth, a.settings, a.cmd, a.edit = nil, nil, screenAuth, authScreen{}, false, nil, ""
	a.queue, a.busy, a.syncGen = nil, false, a.syncGen+1
	a.setFlash("signed out")
	return a.auth.start()
}

func fetchStore(token string, pending *auth.Auth) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		st, err := store.Fetch(ctx, api.New(token))
		return storeMsg{st: st, err: err, pending: pending}
	}
}

// openBrowser opens url with the platform opener; failures become a status message.
func openBrowser(url string) tea.Cmd {
	return func() tea.Msg {
		name := "xdg-open"
		if runtime.GOOS == "darwin" {
			name = "open"
		}
		if err := exec.Command(name, url).Run(); err != nil {
			return flashMsg("couldn't open a browser · press c to copy the URL")
		}
		return nil
	}
}

func (a *App) pen() pen { return pen{th: a.th} }

// overlayPen paints base under everything, as overlays must even in transparent mode.
func (a *App) overlayPen() pen {
	p := a.pen()
	if a.th.Colors["base"] != "" {
		p.bg = a.th.C("base")
	}
	return p
}

func (a *App) style(role string) lipgloss.Style { return a.pen().s(role) }

func (a *App) View() tea.View {
	v := tea.NewView("")
	v.AltScreen = true
	if a.cfg.Appearance.Background == "solid" && a.th.Colors["base"] != "" {
		v.BackgroundColor = a.th.C("base")
	}
	if a.w < 20 || a.h < 5 { // before the first WindowSizeMsg, or a uselessly tiny terminal
		return v
	}
	var body, mode, modeRole string
	var hints [][2]string
	switch {
	case a.screen == screenAuth:
		b, h := a.viewAuth()
		body = lipgloss.Place(a.w, a.h-1, lipgloss.Center, lipgloss.Center, b)
		mode, modeRole, hints = "AUTH", "accent", h
	case a.st == nil:
		body = lipgloss.Place(a.w, a.h-1, lipgloss.Center, lipgloss.Center, a.viewLoading())
		mode, modeRole, hints = "NORMAL", "accent", [][2]string{{"q", "quit"}}
	default:
		body, hints = a.viewMain()
		mode, modeRole = "NORMAL", "accent"
		switch {
		case a.cmd != nil:
			mode, modeRole = "SEARCH", "info"
			if m, _ := cmdMode(a.cmd.in.value()); m == "cmd" {
				mode, modeRole = "COMMAND", "secondary"
			}
		case a.edit != "":
			mode, modeRole = "INSERT", "ok"
		case a.settings:
			mode, modeRole = "SETTINGS", "warn"
		}
	}
	v.SetContent(body + "\n" + a.statusBar(mode, modeRole, hints))
	return v
}

func (a *App) viewLoading() string {
	if a.err != "" {
		return a.style("error").Render("✗ "+a.err) + "\n" + a.style("dim").Render("q quit")
	}
	return a.style("muted").Render(spinnerFrames[a.spin%len(spinnerFrames)] + " Syncing…")
}

func (a *App) statusBar(mode, modeRole string, hints [][2]string) string {
	st := a.style
	left := " " + st(modeRole).Bold(true).Render(mode)
	if a.flash != "" {
		left += "  " + st(a.flashRole).Render(a.flash)
	} else {
		n := 2
		switch {
		case a.w >= 130:
			n = 9
		case a.w >= 110:
			n = 6
		case a.w >= 80:
			n = 4
		}
		for i, hk := range hints {
			if i == n {
				break
			}
			left += "  " + st("sub").Render(hk[0]) + " " + st("dim").Render(hk[1])
		}
	}

	var parts []string
	switch {
	case a.signed == nil:
		parts = append(parts, st("dim").Render("not signed in"))
	case a.st != nil && a.w >= 100:
		parts = append(parts, st("ok").Render(a.icon("", "⟳")+" synced "+ago(a.now.Sub(a.st.SyncedAt))))
		if user, _, _ := strings.Cut(a.st.Email, "@"); user != "" {
			parts = append(parts, st("muted").Render(a.icon(" ", "")+user))
		}
	case a.st != nil && a.w >= 70:
		parts = append(parts, st("ok").Render(a.icon("", "⟳")))
	}
	parts = append(parts, st("text").Render(a.now.Format("15:04")))
	right := strings.Join(parts, st("dim").Render(" "+a.th.Separator+" ")) + " "

	gap := a.w - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		return lipgloss.NewStyle().MaxWidth(a.w).Render(left)
	}
	return left + strings.Repeat(" ", gap) + right
}

func ago(d time.Duration) string {
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	}
	return fmt.Sprintf("%dh ago", int(d.Hours()))
}

func (a *App) icon(nerd, ascii string) string {
	if a.cfg.Appearance.NerdFontIcons {
		return nerd
	}
	return ascii
}

// box draws the auth screen's rounded box of total width w with title set into the top border.
func (a *App) box(title, role string, w int, lines []string) string {
	bc := a.style(role)
	inner := w - 4
	fill := max(w-5-lipgloss.Width(title), 0)
	var b strings.Builder
	b.WriteString(bc.Render("╭─ ") + bc.Bold(true).Render(title) + bc.Render(" "+strings.Repeat("─", fill)+"╮") + "\n")
	for _, l := range lines {
		pad := max(inner-lipgloss.Width(l), 0)
		b.WriteString(bc.Render("│") + " " + l + strings.Repeat(" ", pad) + " " + bc.Render("│") + "\n")
	}
	b.WriteString(bc.Render("╰" + strings.Repeat("─", w-2) + "╯"))
	return b.String()
}

var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
