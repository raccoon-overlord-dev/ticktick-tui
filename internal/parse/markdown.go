// Package parse holds small text parsers: minimal markdown and span word-wrapping.
package parse

import (
	"regexp"
	"strings"

	"github.com/charmbracelet/x/ansi"
)

type SpanKind int

const (
	Plain SpanKind = iota
	Bold
	Code
)

type Span struct {
	Text string
	Kind SpanKind
}

type LineKind int

const (
	Blank LineKind = iota
	Para
	Bullet
	Heading
)

type Line struct {
	Kind  LineKind
	Spans []Span
}

var inlineRe = regexp.MustCompile("\\*\\*[^*]+\\*\\*|`[^`]+`")

// Markdown renders the subset the design uses: paragraphs, "- "/"* " bullets, "#" headings,
// **bold** and `code`. One Line per source line.
func Markdown(src string) []Line {
	var out []Line
	for _, l := range strings.Split(strings.ReplaceAll(src, "\r\n", "\n"), "\n") {
		switch {
		case strings.TrimSpace(l) == "":
			out = append(out, Line{Kind: Blank})
		case strings.HasPrefix(l, "- ") || strings.HasPrefix(l, "* "):
			out = append(out, Line{Kind: Bullet, Spans: Inline(l[2:])})
		case strings.HasPrefix(l, "#"):
			out = append(out, Line{Kind: Heading, Spans: Inline(strings.TrimSpace(strings.TrimLeft(l, "#")))})
		default:
			out = append(out, Line{Kind: Para, Spans: Inline(l)})
		}
	}
	return out
}

func Inline(s string) []Span {
	var out []Span
	last := 0
	for _, m := range inlineRe.FindAllStringIndex(s, -1) {
		if m[0] > last {
			out = append(out, Span{s[last:m[0]], Plain})
		}
		tok := s[m[0]:m[1]]
		if strings.HasPrefix(tok, "**") {
			out = append(out, Span{tok[2 : len(tok)-2], Bold})
		} else {
			out = append(out, Span{tok[1 : len(tok)-1], Code})
		}
		last = m[1]
	}
	if last < len(s) {
		out = append(out, Span{s[last:], Plain})
	}
	return out
}

// Wrap word-wraps spans to width cells, keeping each word's span kind.
// Words longer than width are split.
func Wrap(spans []Span, width int) [][]Span {
	if width < 1 {
		width = 1
	}
	var lines [][]Span
	var cur []Span
	curW := 0
	add := func(text string, k SpanKind) {
		if n := len(cur); n > 0 && cur[n-1].Kind == k {
			cur[n-1].Text += text
		} else {
			cur = append(cur, Span{text, k})
		}
		curW += ansi.StringWidth(text)
	}
	flush := func() {
		if n := len(cur); n > 0 { // drop the trailing space
			cur[n-1].Text = strings.TrimRight(cur[n-1].Text, " ")
		}
		lines = append(lines, cur)
		cur, curW = nil, 0
	}
	for _, sp := range spans {
		for i, word := range strings.Split(sp.Text, " ") {
			if i > 0 && curW > 0 && curW < width {
				add(" ", sp.Kind)
			}
			for word != "" {
				w := ansi.StringWidth(word)
				if curW+w <= width {
					add(word, sp.Kind)
					break
				}
				if curW > 0 {
					flush()
					continue
				}
				head := ansi.Truncate(word, width, "")
				add(head, sp.Kind)
				word = word[len(head):]
				flush()
			}
		}
	}
	if len(cur) > 0 || len(lines) == 0 {
		flush()
	}
	return lines
}
