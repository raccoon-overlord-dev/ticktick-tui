package ui

import (
	"context"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"ttui/internal/auth"
)

// authScreen is the sign-in state (handoff §4, screenshots 6a-6c).
type authScreen struct {
	tokenTab bool
	flow     *auth.Flow // nil when no callback server is running
	oauthErr string

	token    string
	tokenErr string
	checking bool // a token/summary check is in flight

	success bool
	notice  string // e.g. "Session expired", shown under the heading
}

type (
	flowStartedMsg struct {
		flow *auth.Flow
		err  error
	}
	flowDoneMsg struct {
		flow *auth.Flow
		tok  *auth.Token
		err  error
	}
)

func (s *authScreen) start() tea.Cmd {
	s.cancel()
	s.oauthErr = ""
	return func() tea.Msg {
		f, err := auth.Start()
		return flowStartedMsg{f, err}
	}
}

func (s *authScreen) cancel() {
	if s.flow != nil {
		s.flow.Cancel()
		s.flow = nil
	}
}

func waitFlow(f *auth.Flow) tea.Cmd {
	return func() tea.Msg {
		tok, err := f.Wait(context.Background())
		return flowDoneMsg{f, tok, err}
	}
}

func (a *App) updateAuth(msg tea.Msg) tea.Cmd {
	s := &a.auth
	switch msg := msg.(type) {
	case flowStartedMsg:
		if msg.err != nil {
			s.oauthErr = msg.err.Error()
			return nil
		}
		s.flow = msg.flow
		return tea.Batch(waitFlow(msg.flow), openBrowser(msg.flow.URL))
	case flowDoneMsg:
		if msg.flow != s.flow { // a cancelled flow finishing late
			return nil
		}
		s.flow = nil
		if msg.err != nil {
			s.oauthErr = msg.err.Error()
			return nil
		}
		s.checking = true
		return fetchStore(msg.tok.AccessToken, auth.FromToken(msg.tok, "ticktick.com"))
	case tea.PasteMsg:
		if s.tokenTab && !s.success {
			s.token += strings.TrimSpace(msg.Content)
			s.tokenErr = ""
		}
		return nil
	case tea.KeyPressMsg:
		return a.authKey(msg)
	}
	return nil
}

func (a *App) authKey(k tea.KeyPressMsg) tea.Cmd {
	s := &a.auth
	key := k.String()
	if s.success {
		if key == "enter" {
			a.screen, a.auth = screenMain, authScreen{}
		}
		return nil
	}
	if key == "tab" {
		s.tokenTab = !s.tokenTab
		return nil
	}
	if s.tokenTab {
		switch key {
		case "enter":
			if s.token == "" || s.checking {
				return nil
			}
			s.checking, s.tokenErr = true, ""
			return fetchStore(s.token, &auth.Auth{Method: "token", Server: "ticktick.com", AccessToken: s.token})
		case "backspace":
			if s.token != "" {
				s.token = s.token[:len(s.token)-1]
			}
		case "ctrl+u":
			s.token = ""
		case "esc":
			s.tokenTab = false
		default:
			if k.Text != "" {
				s.token += k.Text
				s.tokenErr = ""
			}
		}
		return nil
	}
	switch key {
	case "q", "esc":
		s.cancel()
		return tea.Quit
	case "o":
		if s.flow == nil {
			return s.start()
		}
		return openBrowser(s.flow.URL)
	case "c":
		if s.flow != nil {
			return tea.Batch(tea.SetClipboard(s.flow.URL), func() tea.Msg { return flashMsg("URL copied") })
		}
	}
	return nil
}

func (a *App) viewAuth() (string, [][2]string) {
	s, st := &a.auth, a.style
	w := min(62, a.w)
	inner := w - 4

	tab := func(label string, active bool) string {
		if active {
			return st("accent").Bold(true).Underline(true).Render(label)
		}
		return st("muted").Render(label)
	}
	head := []string{
		st("accent").Render("❯") + " " + st("text").Bold(true).Render("ttui") + " " +
			st("dim").Render(a.version+" · TickTick in your terminal"),
		"",
		st("text").Bold(true).Render("Sign in to TickTick"),
		"",
	}
	if s.notice != "" {
		head = append(head, st("warn").Render(s.notice), "")
	}
	head = append(head,
		tab("Browser (OAuth)", !s.tokenTab)+"   "+tab("API token", s.tokenTab),
		"",
	)

	var box string
	var hints [][2]string
	spin := st("secondary").Render(spinnerFrames[a.spin%len(spinnerFrames)])
	switch {
	case s.success:
		who := "Signed in"
		if a.st.Email != "" {
			who += " as " + a.st.Email
		}
		box = a.box("Connected", "ok", w, []string{
			st("ok").Render("✓") + " " + st("text").Render(who),
			"  " + st("muted").Render(fmt.Sprintf("Synced %d lists · %d tasks · %d tags", len(a.st.Projects)+1, a.st.OpenTotal(), len(a.st.Tags))),
			"",
			st("accent").Render("⏎ continue to Today"),
		})
		hints = [][2]string{{"⏎", "continue"}}

	case s.tokenTab:
		role, lines := "line", []string{st("text").Render("Paste your personal API token"), ""}
		mask := strings.Repeat("•", min(len(s.token), inner-4))
		lines = append(lines, st("accent").Render("❯")+" "+st("text").Render(mask)+st("accent").Render("▏"), "")
		switch {
		case s.checking:
			lines = append(lines, spin+" "+st("muted").Render("Checking token…"), "")
		case s.tokenErr != "":
			role = "error"
			for _, l := range strings.Split(ansi.Wrap(s.tokenErr, inner, ""), "\n") {
				lines = append(lines, st("error").Render(l))
			}
			lines = append(lines, "")
		}
		lines = append(lines, st("dim").Render("Stored in ~/.config/ttui/auth.toml (0600)"))
		box = a.box("API token", role, w, lines)
		hints = [][2]string{{"⏎", "verify"}, {"tab", "use browser"}}

	default:
		var lines []string
		if s.flow != nil {
			lines = append(lines, st("text").Render("1  We opened your browser. If not, visit:"))
			for _, l := range strings.Split(ansi.Hardwrap(s.flow.URL, inner-3, false), "\n") {
				lines = append(lines, "   "+st("info").Underline(true).Render(l))
			}
			lines = append(lines, "", st("text").Render("2  Approve access for ttui"), "",
				spin+"  "+st("muted").Render("Waiting for callback on localhost:8421"))
		} else if s.checking {
			lines = append(lines, spin+"  "+st("muted").Render("Signing in…"))
		} else if s.oauthErr != "" {
			for _, l := range strings.Split(ansi.Wrap("✗ "+s.oauthErr, inner, ""), "\n") {
				lines = append(lines, st("error").Render(l))
			}
			lines = append(lines, "", st("muted").Render("Press o to try again, or tab to use an API token."))
		} else {
			lines = append(lines, spin+"  "+st("muted").Render("Starting…"))
		}
		box = a.box("Browser", "accent", w, lines)
		hints = [][2]string{{"tab", "use token"}, {"o", "reopen browser"}, {"c", "copy url"}}
	}
	return lipgloss.NewStyle().Width(w).Render(strings.Join(head, "\n") + "\n" + box), hints
}
