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
	Link
)

type Span struct {
	Text string
	Kind SpanKind
	URL  string // Link only
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

// urlRe is a bare http(s) URL, minus trailing sentence punctuation.
const urlRe = `https?://[^\s<>()\[\]]*[^\s<>()\[\].,;:!?'"]`

var (
	inlineRe = regexp.MustCompile("\\*\\*[^*]+\\*\\*|`[^`]+`|\\[[^\\]]+\\]\\(" + urlRe + "\\)|" + urlRe)
	linkRe   = regexp.MustCompile(urlRe)
)

// Markdown renders the subset the design uses: paragraphs, "- "/"* " bullets, "#" headings,
// **bold**, `code`, [text](url) and bare URLs. One Line per source line.
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
			out = append(out, Span{Text: s[last:m[0]], Kind: Plain})
		}
		tok := s[m[0]:m[1]]
		switch {
		case strings.HasPrefix(tok, "**"):
			out = append(out, Span{Text: tok[2 : len(tok)-2], Kind: Bold})
		case strings.HasPrefix(tok, "`"):
			out = append(out, Span{Text: tok[1 : len(tok)-1], Kind: Code})
		case strings.HasPrefix(tok, "["):
			i := strings.Index(tok, "](")
			out = append(out, Span{Text: tok[1:i], Kind: Link, URL: tok[i+2 : len(tok)-1]})
		default:
			out = append(out, Span{Text: tok, Kind: Link, URL: tok})
		}
		last = m[1]
	}
	if last < len(s) {
		out = append(out, Span{Text: s[last:], Kind: Plain})
	}
	return out
}

// Links splits plain text (a checklist item) into Plain and bare-URL Link spans.
func Links(s string) []Span {
	var out []Span
	last := 0
	for _, m := range linkRe.FindAllStringIndex(s, -1) {
		if m[0] > last {
			out = append(out, Span{Text: s[last:m[0]], Kind: Plain})
		}
		out = append(out, Span{Text: s[m[0]:m[1]], Kind: Link, URL: s[m[0]:m[1]]})
		last = m[1]
	}
	if last < len(s) {
		out = append(out, Span{Text: s[last:], Kind: Plain})
	}
	return out
}

// Wrap word-wraps spans to width cells, keeping each word's span kind (and link).
// Words longer than width are split.
func Wrap(spans []Span, width int) [][]Span {
	if width < 1 {
		width = 1
	}
	var lines [][]Span
	var cur []Span
	curW := 0
	add := func(text string, sp Span) {
		if n := len(cur); n > 0 && cur[n-1].Kind == sp.Kind && cur[n-1].URL == sp.URL {
			cur[n-1].Text += text
		} else {
			cur = append(cur, Span{Text: text, Kind: sp.Kind, URL: sp.URL})
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
				add(" ", sp)
			}
			for word != "" {
				w := ansi.StringWidth(word)
				if curW+w <= width {
					add(word, sp)
					break
				}
				if curW > 0 {
					flush()
					continue
				}
				head := ansi.Truncate(word, width, "")
				add(head, sp)
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
