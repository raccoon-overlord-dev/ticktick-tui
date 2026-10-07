package parse

import (
	"testing"
	"time"
)

// The triggers below are what the web app wrote (read back 2026-10-07).
func TestReminders(t *testing.T) {
	for _, c := range []struct {
		trig   string
		allDay bool
		text   string
		d      time.Duration
	}{
		{"TRIGGER:P0DT9H0M0S", true, "On the day, 09:00", AllDayTrigger(0, 9, 0)},
		{"TRIGGER:-P0DT15H0M0S", true, "1 day early, 09:00", AllDayTrigger(1, 9, 0)},
		{"TRIGGER:-P6DT15H0M0S", true, "1 week early, 09:00", AllDayTrigger(7, 9, 0)},
		{"TRIGGER:-P13DT13H30M0S", true, "2 weeks early, 10:30", AllDayTrigger(14, 10, 30)},
		{"TRIGGER:PT0S", false, "On time", 0},
		{"TRIGGER:-PT30M", false, "30 minutes early", -30 * time.Minute},
		{"TRIGGER:-PT31680M", false, "22 days early", -22 * 24 * time.Hour},
	} {
		d, ok := ParseTrigger(c.trig)
		if !ok || d != c.d || ReminderText(c.trig, c.allDay) != c.text || Trigger(d, c.allDay) != c.trig {
			t.Errorf("%s: %v %v %q %q", c.trig, d, ok, ReminderText(c.trig, c.allDay), Trigger(d, c.allDay))
		}
	}
	due := time.Date(2026, 10, 10, 0, 0, 0, 0, time.Local)
	if got := FireTime(due, true, AllDayTrigger(1, 9, 0)); !got.Equal(time.Date(2026, 10, 9, 9, 0, 0, 0, time.Local)) {
		t.Errorf("fire %v", got)
	}
	if d, ok := ParseTrigger("TRIGGER:P0Y0M0DT0H0M0.000S"); !ok || d != 0 {
		t.Error("server's on-time form not read")
	}
	if _, ok := ParseTrigger("TRIGGER:soon"); ok {
		t.Error("bad trigger read")
	}
}
