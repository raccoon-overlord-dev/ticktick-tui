package ui

import (
	"strings"
	"unicode"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// textInput is a minimal line editor (optionally multi-line) with a cursor.
type textInput struct {
	r   []rune
	cur int
}

func newInput(s string) textInput { r := []rune(s); return textInput{r: r, cur: len(r)} }

func (t *textInput) value() string { return string(t.r) }

func (t *textInput) insert(s string) {
	ins := []rune(s)
	t.r = append(t.r[:t.cur], append(ins, t.r[t.cur:]...)...)
	t.cur += len(ins)
}

// key applies an editing key and reports whether it was handled.
func (t *textInput) key(k tea.KeyPressMsg, multiline bool) bool {
	switch k.String() {
	case "left", "ctrl+b":
		t.cur = max(t.cur-1, 0)
	case "right", "ctrl+f":
		t.cur = min(t.cur+1, len(t.r))
	case "home", "ctrl+a":
		t.cur = 0
	case "end", "ctrl+e":
		t.cur = len(t.r)
	case "backspace":
		if t.cur > 0 {
			t.r = append(t.r[:t.cur-1], t.r[t.cur:]...)
			t.cur--
		}
	case "delete", "ctrl+d":
		if t.cur < len(t.r) {
			t.r = append(t.r[:t.cur], t.r[t.cur+1:]...)
		}
	case "ctrl+u":
		t.r, t.cur = t.r[t.cur:], 0
	case "ctrl+w":
		i := t.cur
		for i > 0 && unicode.IsSpace(t.r[i-1]) {
			i--
		}
		for i > 0 && !unicode.IsSpace(t.r[i-1]) {
			i--
		}
		t.r, t.cur = append(t.r[:i], t.r[t.cur:]...), i
	case "enter":
		if !multiline {
			return false
		}
		t.insert("\n")
	default:
		if k.Text == "" {
			return false
		}
		t.insert(k.Text)
	}
	return true
}

// view renders one line of at most width cells, scrolled so the cursor stays visible.
func (t *textInput) view(p pen, role string, width int) string {
	start := 0
	for start < t.cur && ansi.StringWidth(string(t.r[start:t.cur]))+1 > width {
		start++
	}
	after := ""
	if t.cur < len(t.r) {
		after = string(t.r[t.cur+1:])
	}
	s := p.s(role).Render(string(t.r[start:t.cur])) + cursorCell(p, t.r, t.cur, role) + p.s(role).Render(after)
	return ansi.Truncate(s, width, "")
}

// lines renders a multi-line value hard-wrapped to width, with the cursor.
func (t *textInput) lines(p pen, role string, width int) []string {
	before, after := string(t.r[:t.cur]), ""
	if t.cur < len(t.r) {
		after = string(t.r[t.cur+1:])
	}
	cur := cursorCell(p, t.r, t.cur, role)
	if t.cur < len(t.r) && t.r[t.cur] == '\n' { // cursor on a line break: show it at line end
		cur, after = lipgloss.NewStyle().Reverse(true).Render(" ")+"\n", string(t.r[t.cur+1:])
	}
	// Style line by line: Render on a multi-line string pads every line to the widest one,
	// which pushed the cursor right after a line break.
	render := func(s string) string {
		ls := strings.Split(s, "\n")
		for i, l := range ls {
			ls[i] = p.s(role).Render(l)
		}
		return strings.Join(ls, "\n")
	}
	var out []string
	for _, l := range strings.Split(render(before)+cur+render(after), "\n") {
		out = append(out, strings.Split(ansi.Hardwrap(l, width, true), "\n")...)
	}
	return out
}

func cursorCell(p pen, r []rune, i int, role string) string {
	c := " "
	if i < len(r) && r[i] != '\n' {
		c = string(r[i])
	}
	return p.s(role).Reverse(true).Render(c)
}
