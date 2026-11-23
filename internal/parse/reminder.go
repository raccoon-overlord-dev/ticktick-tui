package parse

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/raccoon-overlord-dev/ticktick-tui/internal/i18n"
)

// Reminders are iCal triggers relative to the due date (see docs/api-notes.md): all-day
// tasks count from midnight of the due day ("TRIGGER:-P0DT15H0M0S" = 1 day early at 09:00),
// timed tasks from the due time ("TRIGGER:-PT30M"; the web app writes even days as minutes).

const day = 24 * time.Hour

// The server also writes "P0Y0M0DT0H0M0.000S" (years and months are always 0 there).
var triggerRe = regexp.MustCompile(`^(-)?P(?:0Y)?(?:0M)?(?:(\d+)W)?(?:(\d+)D)?(?:T(?:(\d+)H)?(?:(\d+)M)?(?:(\d+)(?:\.\d+)?S)?)?$`)

// ParseTrigger returns a trigger's offset from the due date (all-day: from its midnight).
func ParseTrigger(s string) (time.Duration, bool) {
	g := triggerRe.FindStringSubmatch(strings.TrimPrefix(strings.TrimSpace(s), "TRIGGER:"))
	if g == nil {
		return 0, false
	}
	n := func(i int) time.Duration { v, _ := strconv.Atoi(g[i]); return time.Duration(v) }
	d := n(2)*7*day + n(3)*day + n(4)*time.Hour + n(5)*time.Minute + n(6)*time.Second
	if g[1] != "" {
		d = -d
	}
	return d, true
}

// Trigger writes an offset the way the web app does.
func Trigger(d time.Duration, allDay bool) string {
	sign := ""
	if d < 0 {
		sign, d = "-", -d
	}
	if allDay {
		return fmt.Sprintf("TRIGGER:%sP%dDT%dH%dM0S", sign, d/day, d%day/time.Hour, d%time.Hour/time.Minute)
	}
	if d == 0 {
		return "TRIGGER:PT0S"
	}
	return fmt.Sprintf("TRIGGER:%sPT%dM", sign, d/time.Minute)
}

// AllDayTrigger is the offset for "days early, at h:m" on an all-day task.
func AllDayTrigger(daysEarly, h, m int) time.Duration {
	return -time.Duration(daysEarly)*day + time.Duration(h)*time.Hour + time.Duration(m)*time.Minute
}

// SplitAllDay is the inverse of AllDayTrigger: days early and the time of day.
func SplitAllDay(d time.Duration) (daysEarly int, at time.Duration) {
	at = (d%day + day) % day
	return int(-(d - at) / day), at
}

// FireTime is when a reminder goes off for a task due at due.
func FireTime(due time.Time, allDay bool, d time.Duration) time.Time {
	if allDay {
		y, m, dd := due.In(time.Local).Date()
		due = time.Date(y, m, dd, 0, 0, 0, 0, time.Local)
	}
	return due.Add(d)
}

// plural is "1 day" / "3 days" in the chosen language. The units are spelled out so the
// i18n completeness test can see them.
func plural(n int, unit string) string {
	switch unit {
	case "week":
		return i18n.Plural(n, "1 week", "%d weeks")
	case "day":
		return i18n.Plural(n, "1 day", "%d days")
	case "hour":
		return i18n.Plural(n, "1 hour", "%d hours")
	}
	return i18n.Plural(n, "1 minute", "%d minutes")
}

// ReminderText describes a trigger: "On the day, 09:00", "1 week early, 09:00",
// "30 minutes early", "On time".
func ReminderText(trigger string, allDay bool) string {
	d, ok := ParseTrigger(trigger)
	if !ok {
		return trigger
	}
	if allDay {
		n, at := SplitAllDay(d)
		clock := FmtTime(time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC).Add(at))
		switch {
		case n == 0:
			return i18n.Fmt("On the day, %s", clock)
		case n > 0 && n%7 == 0:
			return i18n.Fmt("%s early, %s", plural(n/7, "week"), clock)
		case n > 0:
			return i18n.Fmt("%s early, %s", plural(n, "day"), clock)
		}
		return i18n.Fmt("%s after, %s", plural(-n, "day"), clock)
	}
	if d == 0 {
		return i18n.T("On time")
	}
	early := d < 0
	if !early {
		d = -d
	}
	var p string
	switch m := int(-d / time.Minute); {
	case m%(60*24*7) == 0:
		p = plural(m/(60*24*7), "week")
	case m%(60*24) == 0:
		p = plural(m/(60*24), "day")
	case m%60 == 0:
		p = plural(m/60, "hour")
	default:
		p = plural(m, "minute")
	}
	if early {
		return i18n.Fmt("%s early", p)
	}
	return i18n.Fmt("%s after", p)
}
