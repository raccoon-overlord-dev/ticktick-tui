package ui

import (
	"image/color"
	"strings"
	"unicode/utf8"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/raccoon-overlord-dev/ticktick-tui/internal/parse"
	"github.com/raccoon-overlord-dev/ticktick-tui/internal/theme"
)

// pen renders text in theme roles, optionally on a background and/or faint (dimmed backdrop).
type pen struct {
	th    *theme.Theme
	bg    color.Color
	faint bool
}

func (p pen) s(role string) lipgloss.Style {
	st := lipgloss.NewStyle().Foreground(p.th.C(role))
	if p.bg != nil {
		st = st.Background(p.bg)
	}
	if p.faint {
		st = st.Faint(true)
	}
	return st
}

func (p pen) on(bg color.Color) pen { p.bg = bg; return p }

// sp is n spaces on the pen's background.
func (p pen) sp(n int) string {
	if n <= 0 {
		return ""
	}
	return p.s("text").Render(strings.Repeat(" ", n))
}

// line lays out one row inside a pane of width w (the row is w-2 wide, between the borders):
// 1 col padding, left, gap, right, 1 col padding. Content area is w-4. left is cut if both don't fit.
func (p pen) line(w int, left, right string) string {
	gap := w - 4 - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 0 {
		left = ansi.Truncate(left, max(w-4-lipgloss.Width(right)-1, 0), "")
		gap = w - 4 - lipgloss.Width(left) - lipgloss.Width(right)
	}
	return p.sp(1) + left + p.sp(gap) + right + p.sp(1)
}

// linkify renders plain text in st with its URLs as clickable (OSC 8) links in linkSt,
// cut to n cells.
func linkify(st, linkSt lipgloss.Style, s string, n int) string {
	var b strings.Builder
	for _, sp := range parse.Links(s) {
		if sp.Kind == parse.Link {
			b.WriteString(linkSt.Underline(true).Hyperlink(sp.URL).Render(sp.Text))
		} else {
			b.WriteString(st.Render(sp.Text))
		}
	}
	return ansi.Truncate(b.String(), max(n, 0), "…")
}

// trunc shortens plain text to n cells with an ellipsis.
func trunc(s string, n int) string {
	if n <= 0 {
		return ""
	}
	return ansi.Truncate(s, n, "…")
}

// tint blends role into base at pct (the design's color-mix). Falls back to surface2
// when either color isn't a hex value (the ANSI "terminal" theme).
func tint(th *theme.Theme, role string, pct float64) color.Color {
	b, c := th.Colors["base"], th.Colors[role]
	if !strings.HasPrefix(b, "#") || !strings.HasPrefix(c, "#") {
		return th.C("surface2")
	}
	br, bg, bb, _ := th.C("base").RGBA()
	cr, cg, cb, _ := th.C(role).RGBA()
	mix := func(x, y uint32) uint8 { return uint8((float64(x)*(1-pct) + float64(y)*pct) / 257) }
	return color.RGBA{mix(br, cr), mix(bg, cg), mix(bb, cb), 0xff}
}

type frameOpts struct {
	w, h        int
	title       string // plain text
	focused     bool
	topRight    string // plain, dim
	bottomRight string // plain, dim
	modal       bool   // always accent border + title (settings)
	body        []string
}

// frame draws a rounded pane with the title set into the top border (handoff "Pane chrome").
func (a *App) frame(p pen, o frameOpts) string {
	borderRole := "line"
	if o.modal || (o.focused && a.cfg.Appearance.FocusedPanel != "title") {
		borderRole = "accent"
	}
	bc := p.s(borderRole)

	title := trunc(o.title, max(o.w-8, 1))
	topRight := trunc(o.topRight, o.w-10-lipgloss.Width(title))
	var tseg string
	switch {
	case title == "":
		tseg = ""
	case o.modal:
		tseg = p.s("accent").Bold(true).Render(" " + title + " ")
	case o.focused && a.cfg.Appearance.FocusedPanel == "title":
		tseg = lipgloss.NewStyle().Background(a.th.C("accent")).Foreground(a.th.C("base")).Bold(true).Faint(p.faint).Render(" " + title + " ")
	case o.focused:
		tseg = p.s("accent").Bold(true).Render(" " + title + " ")
	default:
		tseg = p.s("sub").Render(" " + title + " ")
	}
	tr := ""
	if topRight != "" {
		tr = p.s("dim").Render(" " + topRight + " ")
	}
	fill := max(o.w-4-lipgloss.Width(tseg)-lipgloss.Width(tr), 0)

	var b strings.Builder
	b.WriteString(bc.Render("╭─") + tseg + bc.Render(strings.Repeat("─", fill)) + tr + bc.Render("─╮"))
	inner := o.w - 2
	for i := 0; i < o.h-2; i++ {
		row := p.sp(inner)
		if i < len(o.body) {
			row = o.body[i]
			if pad := inner - lipgloss.Width(row); pad > 0 {
				row += p.sp(pad)
			}
		}
		b.WriteString("\n" + bc.Render("│") + row + bc.Render("│"))
	}
	br := ""
	if o.bottomRight != "" {
		br = p.s("dim").Render(" " + o.bottomRight + " ")
	}
	b.WriteString("\n" + bc.Render("╰"+strings.Repeat("─", max(o.w-3-lipgloss.Width(br), 0))) + br + bc.Render("─╯"))
	return b.String()
}

// window returns the slice of lines to show so that line sel stays visible, updating *off.
// step moves index i by d among n items. One step past either end wraps around; bigger
// jumps (pgup/pgdn, g/G) stop at the ends.
func step(i, d, n int) int {
	switch {
	case n == 0:
		return 0
	case d == 1 && i >= n-1:
		return 0
	case d == -1 && i <= 0:
		return n - 1
	}
	return max(0, min(i+d, n-1))
}

func window(lines []string, sel, visible int, off *int) []string {
	if visible <= 0 {
		return nil
	}
	if sel >= 0 {
		if sel < *off {
			*off = sel
		}
		if sel >= *off+visible {
			*off = sel - visible + 1
		}
	}
	*off = max(min(*off, len(lines)-visible), 0)
	return lines[*off:min(*off+visible, len(lines))]
}

// layout returns the pane count and widths (lists, tasks, detail) for a terminal cols wide.
// The frame has 1 col of outer padding each side and a 1 col gutter between panes.
func layout(cols int, setting string) (n int, lists, tasks, detail int) {
	switch setting {
	case "3":
		n = 3
	case "2":
		n = 2
	case "1":
		n = 1
	default:
		n = 1
		if cols >= 120 {
			n = 3
		} else if cols >= 80 {
			n = 2
		}
	}
	avail := cols - 2
	switch n {
	case 3:
		lists = 24
		if cols >= 140 {
			lists = 28
		}
		rest := avail - lists - 2
		tasks = (rest + 1) / 2
		detail = rest - tasks
	case 2:
		rest := avail - 1
		tasks = int(float64(rest)*1.05/2.05 + 0.5)
		detail = rest - tasks
	default:
		tasks = avail
	}
	return n, lists, tasks, detail
}

// wcSafe rewrites the grapheme clusters whose width differs between grapheme and wcwidth
// counting (⚠️, ♻️, 👨🏻‍💻…) as their first rune plus padding to the same grapheme width.
// Layout is measured in graphemes, but Bubble Tea's renderer counts with wcwidth unless the
// terminal confirms mode 2027, and even then Ghostty misdrew them: rows spilled into the
// next column. Applied to every frame so terminals can't disagree on those clusters.
func wcSafe(s string) string {
	var b strings.Builder
	var state byte
	for len(s) > 0 {
		seq, w, n, ns := ansi.DecodeSequence(s, state, nil)
		if w > 0 && ansi.StringWidthWc(seq) != w {
			r, _ := utf8.DecodeRuneInString(seq)
			rw := ansi.StringWidthWc(string(r))
			if rw > w {
				r, rw = ' ', 1
			}
			seq = string(r) + strings.Repeat(" ", w-rw)
		}
		b.WriteString(seq)
		s, state = s[n:], ns
	}
	return b.String()
}
