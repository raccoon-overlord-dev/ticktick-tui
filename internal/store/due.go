package store

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/raccoon-overlord-dev/ticktick-tui/internal/api"
	"github.com/raccoon-overlord-dev/ticktick-tui/internal/parse"
)

// DueTime parses the due date. All-day tasks are read in the task's own time zone
// (TickTick stores them as local midnight in UTC); timed tasks in the local zone.
func DueTime(t *api.Task) (time.Time, bool) {
	if t.DueDate == "" {
		return time.Time{}, false
	}
	d, err := time.Parse(api.DateLayout, t.DueDate)
	if err != nil {
		return time.Time{}, false
	}
	if t.IsAllDay && t.TimeZone != "" {
		if loc, err := loadLocation(t.TimeZone); err == nil {
			d = d.In(loc)
			return time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, time.Local), true
		}
	}
	return d.In(time.Local), true
}

var zones sync.Map // time zone name → *time.Location

// loadLocation caches time.LoadLocation, which reads and parses the zoneinfo file on every call;
// DueTime runs for every task many times per frame.
func loadLocation(name string) (*time.Location, error) {
	if loc, ok := zones.Load(name); ok {
		return loc.(*time.Location), nil
	}
	loc, err := time.LoadLocation(name)
	if err == nil {
		zones.Store(name, loc)
	}
	return loc, err
}

// DayDiff is the number of calendar days from now's date to the due date (negative = overdue).
func DayDiff(t *api.Task, now time.Time) (int, bool) {
	d, ok := DueTime(t)
	if !ok {
		return 0, false
	}
	return days(now, d), true
}

// days is the number of calendar days from a's date to b's date.
func days(a, b time.Time) int {
	y1, m1, d1 := a.Date()
	y2, m2, d2 := b.Date()
	return int(time.Date(y2, m2, d2, 12, 0, 0, 0, time.UTC).Sub(time.Date(y1, m1, d1, 12, 0, 0, 0, time.UTC)).Hours() / 24)
}

type Due struct {
	Short string // task-row label: "yesterday", "3d ago", "17:00", "today", "tomorrow", "fri", "oct 8"
	Long  string // details: "Today, 17:00", "Yesterday, 10:30 · overdue", "Fri 2 Oct"
	Role  string // theme role
}

// DueLabel returns nil when the task has no due date.
func DueLabel(t *api.Task, now time.Time) *Due {
	d, ok := DueTime(t)
	if !ok {
		return nil
	}
	day, _ := DayDiff(t, now)
	tm := ""
	if !t.IsAllDay {
		tm = parse.FmtTime(d)
	}
	withTime := func(s string) string {
		if tm != "" {
			return s + ", " + tm
		}
		return s
	}
	full := d.Format("Mon ") + parse.FmtDayMonth(d)
	switch {
	case day < 0:
		short, long := fmt.Sprintf("%dd ago", -day), full
		if day == -1 {
			short, long = "yesterday", "Yesterday"
		}
		return &Due{short, withTime(long) + " · overdue", "error"}
	case day == 0:
		short := "today"
		if tm != "" {
			short = tm
		}
		return &Due{short, withTime("Today"), "secondary"}
	case day == 1:
		return &Due{"tomorrow", withTime("Tomorrow"), "sub"}
	case day < 7:
		return &Due{strings.ToLower(d.Format("Mon")), withTime(full), "muted"}
	}
	return &Due{strings.ToLower(parse.FmtDayMonth(d)), withTime(full), "muted"}
}

// RepeatLabel turns an RRULE into "Daily", "Weekdays", "Weekly", "Monthly", "Yearly" or "Custom".
func RepeatLabel(rrule string) string {
	if rrule == "" {
		return ""
	}
	parts := map[string]string{}
	for _, kv := range strings.Split(strings.TrimPrefix(rrule, "RRULE:"), ";") {
		k, v, _ := strings.Cut(kv, "=")
		parts[k] = v
	}
	if parts["INTERVAL"] != "" && parts["INTERVAL"] != "1" {
		return "Custom"
	}
	for k, v := range parts { // anything beyond the plain menu rules (days, ends, skips…)
		if k != "FREQ" && k != "INTERVAL" && !(k == "BYDAY" && v == "MO,TU,WE,TH,FR" && parts["FREQ"] == "WEEKLY") {
			return "Custom"
		}
	}
	switch parts["FREQ"] {
	case "DAILY":
		return "Daily"
	case "WEEKLY":
		if parts["BYDAY"] == "MO,TU,WE,TH,FR" {
			return "Weekdays"
		}
		return "Weekly"
	case "MONTHLY":
		return "Monthly"
	case "YEARLY":
		return "Yearly"
	}
	return "Custom"
}
