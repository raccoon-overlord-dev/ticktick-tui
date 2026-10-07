package parse

import (
	"testing"
	"time"
)

func TestDateFormats(t *testing.T) {
	defer func(o string, c bool) { DateOrder, Clock12 = o, c }(DateOrder, Clock12)
	now := time.Date(2026, 10, 7, 12, 0, 0, 0, time.Local)
	d := time.Date(2026, 10, 8, 17, 30, 0, 0, time.Local)
	for _, c := range []struct{ order, num, dm, in string }{
		{"dd/mm/yyyy", "08/10/2026", "8 Oct", "8/10"},
		{"mm/dd/yyyy", "10/08/2026", "Oct 8", "10/8"},
		{"yyyy/mm/dd", "2026/10/08", "Oct 8", "10/8"},
	} {
		DateOrder = c.order
		if FmtDate(d) != c.num || FmtDayMonth(d) != c.dm {
			t.Errorf("%s: %s %s", c.order, FmtDate(d), FmtDayMonth(d))
		}
		for _, in := range []string{c.num, c.in} {
			if due, err := ParseDue(in, now); err != nil || due.Day != 1 {
				t.Errorf("%s: ParseDue(%q) = %+v, %v", c.order, in, due, err)
			}
		}
	}
	DateOrder = "dd/mm/yyyy"
	if due, _ := ParseDue("1/10", now); due == nil || due.Day != 359 { // past: next year
		t.Errorf("1/10 → %+v", due)
	}
	if _, err := ParseDue("31/02", now); err == nil {
		t.Error("31/02 accepted")
	}
	Clock12 = true
	if FmtTime(d) != "5:30pm" {
		t.Error(FmtTime(d))
	}
}
