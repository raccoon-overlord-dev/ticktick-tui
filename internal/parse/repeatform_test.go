package parse

import (
	"reflect"
	"testing"
)

// Each web-app rule reads into the form and writes back unchanged.
func TestRepeatFormRoundTrip(t *testing.T) {
	for _, c := range []struct{ rule, from string }{
		{"RRULE:FREQ=WEEKLY;INTERVAL=2;BYDAY=MO,TH", "0"},
		{"RRULE:FREQ=MONTHLY;INTERVAL=1;BYMONTHDAY=1,15,-1;TT_SKIP=WEEKEND", "0"},
		{"RRULE:FREQ=MONTHLY;INTERVAL=1;BYDAY=5FR", "0"},
		{"RRULE:FREQ=MONTHLY;INTERVAL=1;BYDAY=-1TU", "0"},
		{"RRULE:FREQ=MONTHLY;INTERVAL=1;BYMONTHDAY=-1;TT_WORKDAY=-1", "0"},
		{"RRULE:FREQ=YEARLY;INTERVAL=1;BYMONTH=10;BYMONTHDAY=6", "0"},
		{"RRULE:FREQ=YEARLY;INTERVAL=3;BYMONTH=5;BYDAY=2SU", "0"},
		{"RRULE:FREQ=DAILY;INTERVAL=2;TT_SKIP=WEEKEND", "1"},
		{"RRULE:FREQ=WEEKLY;INTERVAL=1", "1"},
		{"RRULE:FREQ=DAILY;INTERVAL=1;COUNT=5;TT_SKIP=HOLIDAY,WEEKEND", "0"},
		{"ERULE:NAME=CUSTOM;BYDATE=20261006,20261012", ""},
	} {
		f := FormFromRule(c.rule, c.from, now)
		got, from := f.Rule()
		if got != c.rule || from != (c.from == "1") {
			t.Errorf("%s: form %+v wrote %s (from completion %v)", c.rule, f, got, from)
		}
	}
	// New rule: defaults come from the due date (Wednesday 30 Sep).
	f := FormFromRule("", "", now)
	want := RepeatForm{Mode: "due", Every: 1, Unit: "week", Days: []string{"WE"}, By: "each", MonthDays: []int{30},
		Ord: 5, Weekday: "WE", Workday: 1, Month: 9, Day: 30}
	if !reflect.DeepEqual(f, want) {
		t.Errorf("defaults: %+v", f)
	}
	if r, _ := f.Rule(); r != "RRULE:FREQ=WEEKLY;INTERVAL=1;BYDAY=WE" {
		t.Errorf("default rule %s", r)
	}
	f.Unit, f.By, f.MonthDays = "month", "each", []int{-1, 15, 1}
	if r, _ := f.Rule(); r != "RRULE:FREQ=MONTHLY;INTERVAL=1;BYMONTHDAY=1,15,-1" {
		t.Errorf("month days sorted: %s", r)
	}
}
