package ui

import (
	"slices"
	"strings"
	"unicode"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// textInput is a minimal line editor (optionally multi-line) with a cursor, a selection
// (shift+arrows, ctrl+a) and undo / redo for the current edit.
type textInput struct {
	r      []rune
	cur    int
	anchor int // other end of the selection, -1 for none
	undo   []snapshot
	redo   []snapshot
	last   string // kind of the last change, for grouping typing into words
	hidden bool   // cursor blink: off phase
}

type snapshot struct {
	r   []rune
	cur int
}

func newInput(s string) textInput { r := []rune(s); return textInput{r: r, cur: len(r), anchor: -1} }

func (t *textInput) value() string { return string(t.r) }

// selection returns the selected range; lo == hi when nothing is selected.
func (t *textInput) selection() (lo, hi int) {
	if t.anchor < 0 {
		return 0, 0
	}
	return min(t.anchor, t.cur), max(t.anchor, t.cur)
}

func (t *textInput) selected() string { lo, hi := t.selection(); return string(t.r[lo:hi]) }

func (t *textInput) selectAll() { t.anchor, t.cur = 0, len(t.r) }

// save records the text before a change. Changes of the same kind in a row ("type",
// "back", "del") are one undo step, and typing starts a new step at each word.
func (t *textInput) save(kind string) {
	if kind != "" && kind == t.last && !(kind == "type" && t.cur > 0 && unicode.IsSpace(t.r[t.cur-1])) {
		return
	}
	t.undo = append(t.undo, snapshot{slices.Clone(t.r), t.cur})
	t.redo, t.last = nil, kind
}

func (t *textInput) restore(from, to *[]snapshot) {
	if len(*from) == 0 {
		return
	}
	*to = append(*to, snapshot{slices.Clone(t.r), t.cur})
	s := (*from)[len(*from)-1]
	*from = (*from)[:len(*from)-1]
	t.r, t.cur, t.anchor, t.last = s.r, s.cur, -1, ""
}

// cutSel deletes the selection, if any.
func (t *textInput) cutSel() bool {
	lo, hi := t.selection()
	t.anchor = -1
	if lo == hi {
		return false
	}
	t.r, t.cur = append(t.r[:lo:lo], t.r[hi:]...), lo
	return true
}

// insert pastes s (or types it, kind "type"), replacing the selection.
func (t *textInput) insert(s string) { t.put(s, "") }

func (t *textInput) put(s, kind string) {
	if lo, hi := t.selection(); lo != hi {
		t.last = "" // replacing a selection starts a new undo step
	}
	t.save(kind)
	t.cutSel()
	ins := []rune(s)
	t.r = append(t.r[:t.cur:t.cur], append(ins, t.r[t.cur:]...)...)
	t.cur += len(ins)
}

// move applies a cursor key; it reports false for keys that aren't moves.
func (t *textInput) move(key string, multiline bool) bool {
	switch key {
	case "left", "ctrl+b":
		t.cur = max(t.cur-1, 0)
	case "right", "ctrl+f":
		t.cur = min(t.cur+1, len(t.r))
	case "home":
		t.cur = 0
	case "end", "ctrl+e":
		t.cur = len(t.r)
	case "up", "down":
		if !multiline {
			return false
		}
		t.vmove(key == "down")
	default:
		return false
	}
	t.last = ""
	return true
}

// key applies an editing key and reports whether it was handled.
func (t *textInput) key(k tea.KeyPressMsg, multiline bool) bool {
	key := k.String()
	t.hidden = false // show the cursor while typing
	if base, ok := strings.CutPrefix(key, "shift+"); ok && base != "tab" {
		anchor := t.anchor
		if anchor < 0 {
			anchor = t.cur
		}
		if t.move(base, multiline) {
			t.anchor = anchor
			return true
		}
	}
	lo, hi := t.selection()
	switch key {
	case "ctrl+a", "super+a":
		t.selectAll()
		return true
	case "ctrl+z", "super+z":
		t.restore(&t.undo, &t.redo)
		return true
	case "ctrl+y", "super+y", "ctrl+shift+z", "super+shift+z":
		t.restore(&t.redo, &t.undo)
		return true
	case "left", "right": // with a selection: go to its start / end
		if lo != hi {
			t.cur, t.anchor = map[bool]int{true: lo, false: hi}[key == "left"], -1
			return true
		}
	}
	if t.move(key, multiline) {
		t.anchor = -1
		return true
	}
	switch key {
	case "backspace", "delete", "ctrl+d":
		if lo != hi {
			t.save("")
			t.cutSel()
			return true
		}
		if key == "backspace" && t.cur > 0 {
			t.save("back")
			t.r = append(t.r[:t.cur-1:t.cur-1], t.r[t.cur:]...)
			t.cur--
		} else if key != "backspace" && t.cur < len(t.r) {
			t.save("del")
			t.r = append(t.r[:t.cur:t.cur], t.r[t.cur+1:]...)
		}
	case "ctrl+u":
		t.save("")
		t.r, t.cur, t.anchor = t.r[t.cur:], 0, -1
	case "ctrl+w":
		t.save("")
		t.anchor = -1
		i := t.cur
		for i > 0 && unicode.IsSpace(t.r[i-1]) {
			i--
		}
		for i > 0 && !unicode.IsSpace(t.r[i-1]) {
			i--
		}
		t.r, t.cur = append(t.r[:i:i], t.r[t.cur:]...), i
	case "enter":
		if !multiline {
			return false
		}
		t.insert("\n")
	default:
		if k.Text == "" {
			return false
		}
		t.put(k.Text, "type")
	}
	return true
}

// vmove moves the cursor to the same column on the previous or next line.
func (t *textInput) vmove(down bool) {
	start := t.cur
	for start > 0 && t.r[start-1] != '\n' {
		start--
	}
	col := t.cur - start
	if down {
		next := slices.Index(t.r[t.cur:], '\n')
		if next < 0 {
			t.cur = len(t.r)
			return
		}
		start = t.cur + next + 1
	} else {
		if start == 0 {
			t.cur = 0
			return
		}
		start--
		for start > 0 && t.r[start-1] != '\n' {
			start--
		}
	}
	end := start
	for end < len(t.r) && t.r[end] != '\n' {
		end++
	}
	t.cur = min(start+col, end)
}

// view renders one line of at most width cells, scrolled so the cursor stays visible.
func (t *textInput) view(p pen, role string, width int) string {
	start := 0
	for start < t.cur && ansi.StringWidth(string(t.r[start:t.cur]))+1 > width {
		start++
	}
	s := t.span(p, role, start, t.cur) + t.cursor(p, role) + t.span(p, role, min(t.cur+1, len(t.r)), len(t.r))
	return ansi.Truncate(s, width, "")
}

// span renders r[from:to] in role with the selected part highlighted. It styles line by
// line: Render on a multi-line string pads every line to the widest one, which pushed the
// cursor right after a line break.
func (t *textInput) span(p pen, role string, from, to int) string {
	lo, hi := t.selection()
	var b strings.Builder
	for from < to {
		end, st := to, p.s(role)
		switch {
		case from < lo:
			end = min(to, lo)
		case from < hi:
			end, st = min(to, hi), p.s("accent").Reverse(true)
		}
		ls := strings.Split(string(t.r[from:end]), "\n")
		for i, l := range ls {
			ls[i] = st.Render(l)
		}
		b.WriteString(strings.Join(ls, "\n"))
		from = end
	}
	return b.String()
}

// lines renders a multi-line value hard-wrapped to width, with the cursor,
// and reports which of the returned lines holds the cursor.
func (t *textInput) lines(p pen, role string, width int) ([]string, int) {
	before, after := t.span(p, role, 0, t.cur), t.span(p, role, min(t.cur+1, len(t.r)), len(t.r))
	cur := t.cursor(p, role)
	if t.cur < len(t.r) && t.r[t.cur] == '\n' { // cursor on a line break: show it at line end
		cur = lipgloss.NewStyle().Reverse(!t.hidden).Render(" ") + "\n"
	}
	wrap := func(s string) []string {
		var out []string
		for _, l := range strings.Split(s, "\n") {
			out = append(out, strings.Split(ansi.Hardwrap(l, width, true), "\n")...)
		}
		return out
	}
	// Hard wrapping is positional, so the text up to the cursor wraps the same way on its own.
	row := len(wrap(before+strings.TrimSuffix(cur, "\n"))) - 1
	return wrap(before + cur + after), row
}

// cursor renders the cell under the cursor, reversed unless blinked off.
func (t *textInput) cursor(p pen, role string) string {
	c := " "
	if t.cur < len(t.r) && t.r[t.cur] != '\n' {
		c = string(t.r[t.cur])
	}
	return p.s(role).Reverse(!t.hidden).Render(c)
}
