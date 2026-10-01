package parse

import (
	"reflect"
	"testing"
)

func TestMarkdown(t *testing.T) {
	got := Markdown("Focus on the **responsive layout** now.\n\n- `colorful` and lotr\n# Head")
	want := []Line{
		{Para, []Span{{"Focus on the ", Plain}, {"responsive layout", Bold}, {" now.", Plain}}},
		{Kind: Blank},
		{Bullet, []Span{{"colorful", Code}, {" and lotr", Plain}}},
		{Heading, []Span{{"Head", Plain}}},
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
	got := text(Wrap([]Span{{"Focus on the ", Plain}, {"responsive layout", Bold}, {" and the new themes.", Plain}}, 20))
	want := []string{"Focus on the", "responsive layout", "and the new themes."}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q", got)
	}
	got = text(Wrap([]Span{{"abcdefghij", Plain}}, 4))
	if !reflect.DeepEqual(got, []string{"abcd", "efgh", "ij"}) {
		t.Fatalf("hard split got %q", got)
	}
	// Bold span keeps its kind after wrapping.
	ls := Wrap([]Span{{"aa ", Plain}, {"bb cc", Bold}}, 5)
	if ls[1][0].Kind != Bold || ls[1][0].Text != "cc" {
		t.Fatalf("kind lost: %#v", ls)
	}
}
