package parse

import (
	"reflect"
	"testing"
)

func TestMarkdown(t *testing.T) {
	got := Markdown("Focus on the **responsive layout** now.\n\n- `colorful` and lotr\n# Head")
	want := []Line{
		{Para, []Span{{Text: "Focus on the ", Kind: Plain}, {Text: "responsive layout", Kind: Bold}, {Text: " now.", Kind: Plain}}},
		{Kind: Blank},
		{Bullet, []Span{{Text: "colorful", Kind: Code}, {Text: " and lotr", Kind: Plain}}},
		{Heading, []Span{{Text: "Head", Kind: Plain}}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v", got)
	}
}

func TestWrap(t *testing.T) {
	text := func(ls [][]Span) []string {
		var out []string
		for _, l := range ls {
			s := ""
			for _, sp := range l {
				s += sp.Text
			}
			out = append(out, s)
		}
		return out
	}
	got := text(Wrap([]Span{{Text: "Focus on the ", Kind: Plain}, {Text: "responsive layout", Kind: Bold}, {Text: " and the new themes.", Kind: Plain}}, 20))
	want := []string{"Focus on the", "responsive layout", "and the new themes."}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q", got)
	}
	got = text(Wrap([]Span{{Text: "abcdefghij", Kind: Plain}}, 4))
	if !reflect.DeepEqual(got, []string{"abcd", "efgh", "ij"}) {
		t.Fatalf("hard split got %q", got)
	}
	// Bold span keeps its kind after wrapping.
	ls := Wrap([]Span{{Text: "aa ", Kind: Plain}, {Text: "bb cc", Kind: Bold}}, 5)
	if ls[1][0].Kind != Bold || ls[1][0].Text != "cc" {
		t.Fatalf("kind lost: %#v", ls)
	}
}

func TestLinks(t *testing.T) {
	got := Inline("see https://a.dev/x?q=1. and [docs](http://b.dev) or **b**")
	want := []Span{
		{Text: "see ", Kind: Plain},
		{Text: "https://a.dev/x?q=1", Kind: Link, URL: "https://a.dev/x?q=1"},
		{Text: ". and ", Kind: Plain},
		{Text: "docs", Kind: Link, URL: "http://b.dev"},
		{Text: " or ", Kind: Plain},
		{Text: "b", Kind: Bold},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v", got)
	}
	if ls := Links("buy https://shop.dev, **not bold**"); len(ls) != 3 || ls[1].URL != "https://shop.dev" || ls[2].Text != ", **not bold**" {
		t.Fatalf("Links got %#v", ls)
	}
	// A wrapped link keeps its URL on every line.
	ls := Wrap([]Span{{Text: "my long link", Kind: Link, URL: "https://c.dev"}}, 7)
	if ls[1][0].URL != "https://c.dev" {
		t.Fatalf("url lost: %#v", ls)
	}
}

func TestAttachments(t *testing.T) {
	cases := []struct {
		src  string
		want []string
	}{
		{"", nil},
		{"![file](6af8506f8c6c4f0220332154/piano-di-lavoro.pdf)", []string{"6af8506f8c6c4f0220332154/piano-di-lavoro.pdf"}},
		{"notes\n![file](a1/one.pdf)\n![file](b2/two with space.png)\n", []string{"a1/one.pdf", "b2/two with space.png"}},
		{"![alt](https://host/pic.png)", nil},             // a real markdown image
		{"[piano](https://host/x/y.pdf) plain text", nil}, // a link, not an attachment
		{"![file](nofilename)", nil},                      // no "<id>/<name>" shape
	}
	for _, c := range cases {
		got := Attachments(c.src)
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("Attachments(%q) = %q, want %q", c.src, got, c.want)
		}
	}
}
