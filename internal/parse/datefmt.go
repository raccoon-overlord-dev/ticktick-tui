package parse

import (
	"strconv"
	"strings"
	"time"
)

// Settings → Date & time, set by the UI from config.toml. Shared by every date ttui shows.
var (
	Clock12   bool           // 12-hour times ("5:30pm") instead of 24-hour ("17:30")
	DateOrder = "dd/mm/yyyy" // dd/mm/yyyy | yyyy/mm/dd | mm/dd/yyyy: numeric dates, and day-month order in words
)

// FmtTime is a time of day: "17:30" or "5:30pm".
func FmtTime(t time.Time) string {
	if Clock12 {
		return t.Format("3:04pm")
	}
	return t.Format("15:04")
}

// FmtDate is a numeric date in the chosen order: "08/10/2026", "2026/10/08", "10/08/2026".
func FmtDate(t time.Time) string {
	switch DateOrder {
	case "yyyy/mm/dd":
		return t.Format("2006/01/02")
	case "mm/dd/yyyy":
		return t.Format("01/02/2006")
	}
	return t.Format("02/01/2006")
}

// FmtDayMonth is "8 Oct" (day first) or "Oct 8".
func FmtDayMonth(t time.Time) string {
	if DateOrder == "dd/mm/yyyy" {
		return t.Format("2 Jan")
	}
	return t.Format("Jan 2")
}

// FmtLongDate is "8 Oct 2026" (day first) or "Oct 8, 2026".
func FmtLongDate(t time.Time) string {
	if DateOrder == "dd/mm/yyyy" {
		return t.Format("2 Jan 2006")
	}
	return t.Format("Jan 2, 2006")
}

// parseNumDate reads a numeric date in the chosen order, with / . or - between the parts;
// the year may be left out (the next such date from today). ISO dates are read by ParseDay.
func parseNumDate(w string, now time.Time) (time.Time, bool) {
	parts := strings.FieldsFunc(w, func(r rune) bool { return r == '/' || r == '.' || r == '-' })
	if len(parts) < 2 || len(parts) > 3 {
		return time.Time{}, false
	}
	n := make([]int, len(parts))
	for i, p := range parts {
		v, err := strconv.Atoi(p)
		if err != nil {
			return time.Time{}, false
		}
		n[i] = v
	}
	y, m, d := -1, 0, 0
	switch {
	case DateOrder == "yyyy/mm/dd" && len(n) == 3:
		y, m, d = n[0], n[1], n[2]
	case DateOrder == "yyyy/mm/dd":
		m, d = n[0], n[1]
	case DateOrder == "mm/dd/yyyy":
		m, d = n[0], n[1]
	default:
		d, m = n[0], n[1]
	}
	if len(n) == 3 && DateOrder != "yyyy/mm/dd" {
		y = n[2]
	}
	if y >= 0 && y < 100 {
		y += 2000
	}
	if y < 0 {
		y = now.Year()
	}
	t := time.Date(y, time.Month(m), d, 0, 0, 0, 0, now.Location())
	if t.Day() != d || int(t.Month()) != m { // 31/02 and the like
		return time.Time{}, false
	}
	if len(n) == 2 && t.Before(time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())) {
		t = t.AddDate(1, 0, 0) // no year: the next one
	}
	return t, true
}
