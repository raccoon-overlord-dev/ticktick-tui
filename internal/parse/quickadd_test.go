package parse

import (
	"reflect"
	"testing"
	"time"
)

// Wednesday 30 Sep 2026.
var now = time.Date(2026, 9, 30, 10, 0, 0, 0, time.UTC)

func TestParseTime(t *testing.T) {
	for in, want := range map[string][2]int{"17:00": {17, 0}, "9:30": {9, 30}, "5pm": {17, 0}, "12am": {0, 0}, "12pm": {12, 0}, "7:15am": {7, 15}} {
		h, m, ok := ParseTime(in)
		if !ok || h != want[0] || m != want[1] {
			t.Errorf("ParseTime(%q) = %d:%d %v", in, h, m, ok)
		}
	}
	for _, in := range []string{"17", "25:00", "9:75", "abc"} {
		if _, _, ok := ParseTime(in); ok {
			t.Errorf("ParseTime(%q) should fail", in)
		}
	}
}

func TestParseDay(t *testing.T) {
	for in, want := range map[string]int{"today": 0, "tmr": 1, "yesterday": -1, "fri": 2, "friday": 2, "wed": 7, "mon": 5, "sun": 4, "2026-10-08": 8, "2026-09-29": -1, "+7d": 7, "+0d": 0} {
		if got, ok := ParseDay(in, now); !ok || got != want {
			t.Errorf("ParseDay(%q) = %d %v, want %d", in, got, ok, want)
		}
	}
	if _, ok := ParseDay("fr", now); ok {
		t.Error("2-letter prefix should not match")
	}
}

func TestParseDue(t *testing.T) {
	d, err := ParseDue("tomorrow 17:00", now)
	if err != nil || d.Day != 1 || !d.HasTime || d.H != 17 {
		t.Fatalf("got %+v %v", d, err)
	}
	if d, _ := ParseDue("18:30", now); d.Day != 0 || d.H != 18 {
		t.Fatalf("time alone should be today: %+v", d)
	}
	if d, _ := ParseDue("fri", now); d.HasTime || d.Day != 2 {
		t.Fatalf("fri: %+v", d)
	}
	if d, err := ParseDue("  ", now); d != nil || err != nil {
		t.Fatal("empty should clear")
	}
	if _, err := ParseDue("someday", now); err == nil {
		t.Fatal("garbage accepted")
	}
	if got := (&Due{Day: 1, HasTime: true, H: 17}).At(now); got != time.Date(2026, 10, 1, 17, 0, 0, 0, time.UTC) {
		t.Fatalf("At = %v", got)
	}
}

func TestQuickAdd(t *testing.T) {
	lists := []List{{"inbox", "Inbox"}, {"p1", "👋Welcome"}, {"p2", "Home Office"}}
	a := QuickAdd("call mom tomorrow 17:00 !high #errand @homeo", lists, "inbox", nil, now, true)
	want := Add{Title: "call mom", Prio: 5, PrioSet: true, Due: &Due{Day: 1, HasTime: true, H: 17}, DueSet: true,
		Tags: []string{"errand"}, List: "p2", ListSet: true}
	if !reflect.DeepEqual(a, want) {
		t.Fatalf("got %+v", a)
	}
	a = QuickAdd("buy milk @welc", lists, "inbox", &Due{Day: 0}, now, true)
	if a.List != "p1" || a.DueSet || a.Due == nil || a.Due.Day != 0 || a.PrioSet {
		t.Fatalf("defaults/emoji list: %+v", a)
	}
	if a := QuickAdd("call mom tomorrow 17:00 !high", lists, "inbox", nil, now, false); a.Title != "call mom tomorrow 17:00" || a.Due != nil || a.Prio != 5 {
		t.Errorf("smart dates off: %+v", a)
	}
	a = QuickAdd("email @nowhere 5pm", lists, "inbox", nil, now, true)
	if a.Title != "email @nowhere" || a.Due.Day != 0 || a.Due.H != 17 {
		t.Fatalf("unknown list stays in title, time means today: %+v", a)
	}
}

func TestFuzzy(t *testing.T) {
	s, pos, ok := Fuzzy("Review Q4 roadmap draft", "rev")
	if !ok || s != 0 || !reflect.DeepEqual(pos, []int{0, 1, 2}) {
		t.Fatalf("substring: %d %v %v", s, pos, ok)
	}
	s, pos, ok = Fuzzy("Reply to Lena about the interview", "rev")
	if !ok || s < 100 || len(pos) != 3 {
		t.Fatalf("subsequence: %d %v %v", s, pos, ok)
	}
	if _, _, ok := Fuzzy("abc", "abd"); ok {
		t.Fatal("non-match accepted")
	}
	if _, pos, _ := Fuzzy("📖Study", "st"); pos[0] != 1 {
		t.Fatalf("rune positions: %v", pos)
	}
}
