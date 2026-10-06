package parse

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"
)

// RepeatHelp is the hint shown in the Custom repeat editor.
const RepeatHelp = "every 3 days · mon,wed · 1st,15th · last fri · every year oct 6th · first workday · until 2026-12-31 · x5 · from completion"

var (
	byDay     = []string{"SU", "MO", "TU", "WE", "TH", "FR", "SA"}
	ordinals  = map[string]int{"first": 1, "1st": 1, "second": 2, "2nd": 2, "third": 3, "3rd": 3, "fourth": 4, "4th": 4, "fifth": 5, "5th": 5, "last": -1}
	ordText   = map[string]string{"1": "1st", "2": "2nd", "3": "3rd", "4": "4th", "5": "5th", "-1": "last"}
	months    = []string{"january", "february", "march", "april", "may", "june", "july", "august", "september", "october", "november", "december"}
	units     = map[string]string{"day": "DAILY", "week": "WEEKLY", "month": "MONTHLY", "year": "YEARLY"}
	freqWords = map[string]string{"daily": "DAILY", "weekly": "WEEKLY", "monthly": "MONTHLY", "yearly": "YEARLY", "annually": "YEARLY"}
	fillers   = []string{"every", "each", "on", "the", "of", "a", "and", "repeat", "in"}
)

// ParseRepeat reads the Custom repeat field into a TickTick repeatFlag and whether it
// repeats from the completion date. It writes the same rules the web app does (see
// docs/api-notes.md): "every 2 weeks", "mon,wed,fri", "weekdays", "23rd", "1st,15th,last day",
// "3rd wed", "last fri", "every year oct 6th", "every year oct 1st tue", "first workday",
// "last workday", "until 2026-12-31", "x5",
// "skip weekends", "from completion", "curve" (Ebbinghaus), "dates fri, 2026-10-22".
func ParseRepeat(s string, now time.Time) (rule string, fromCompletion bool, err error) {
	w := strings.Fields(strings.NewReplacer(",", " ", ";", " ").Replace(strings.ToLower(s)))
	if len(w) == 0 {
		return "", false, errors.New("empty repeat")
	}
	var freq, month, until string
	var days, monthDays []string
	var skip []string
	interval, count, workday := 1, 0, 0

	if w[0] == "curve" || w[0] == "ebbinghaus" {
		return "ERULE:NAME=FORGETTINGCURVE;CYCLE=0", false, nil
	}
	if w[0] == "dates" {
		var ds []string
		for _, x := range w[1:] {
			n, ok := ParseDay(x, now)
			if !ok {
				return "", false, errors.New("couldn't read date: " + x)
			}
			ds = append(ds, now.AddDate(0, 0, n).Format("20060102"))
		}
		if len(ds) == 0 {
			return "", false, errors.New("dates: list them, e.g. dates fri 2026-10-22")
		}
		return "ERULE:NAME=CUSTOM;BYDATE=" + strings.Join(ds, ","), false, nil
	}

	// A day of the month ("23rd", "3rd wed") means monthly unless the rule says yearly.
	soft := false
	setFreq := func(f string, fromDay bool) error {
		switch {
		case freq == "":
			freq, soft = f, fromDay
		case freq == f:
			soft = soft && fromDay
		case f == "YEARLY" && freq == "MONTHLY" && soft:
			freq, soft = f, false
		case f == "MONTHLY" && fromDay && freq == "YEARLY":
		default:
			return errors.New("repeat: two frequencies")
		}
		return nil
	}
	for i := 0; i < len(w); i++ {
		x := w[i]
		next := ""
		if i+1 < len(w) {
			next = strings.TrimSuffix(w[i+1], "s")
		}
		var e error
		switch {
		case slices.Contains(fillers, x):
		case freqWords[x] != "":
			e = setFreq(freqWords[x], false)
		case units[strings.TrimSuffix(x, "s")] != "":
			e = setFreq(units[strings.TrimSuffix(x, "s")], false)
		case x == "weekday" || x == "weekdays":
			days, e = append(days, "MO", "TU", "WE", "TH", "FR"), setFreq("WEEKLY", false)
		case x == "weekend" || x == "weekends":
			days, e = append(days, "SA", "SU"), setFreq("WEEKLY", false)
		case weekday(x) != "":
			days, e = append(days, weekday(x)), setFreq("WEEKLY", false)
		case monthNum(x) > 0:
			month, e = strconv.Itoa(monthNum(x)), setFreq("YEARLY", false)
		case ordinals[x] != 0 && i+1 < len(w) && weekday(w[i+1]) != "":
			days, e = append(days, fmt.Sprint(ordinals[x], weekday(w[i+1]))), setFreq("MONTHLY", true)
			i++
		case (x == "first" || x == "last") && next == "workday": // as the web app writes it
			workday, e = ordinals[x], setFreq("MONTHLY", false)
			monthDays = []string{strconv.Itoa(workday)}
			i++
		case x == "last" && next == "day":
			monthDays, e = append(monthDays, "-1"), setFreq("MONTHLY", true)
			i++
		case x == "until" && i+1 < len(w):
			n, ok := ParseDay(w[i+1], now)
			if !ok {
				return "", false, errors.New("couldn't read date: " + w[i+1])
			}
			until = now.AddDate(0, 0, n).Format("20060102")
			i++
		case x == "skip" && i+1 < len(w):
			for i+1 < len(w) && (strings.HasPrefix(w[i+1], "weekend") || strings.HasPrefix(w[i+1], "holiday")) {
				skip = append(skip, map[bool]string{true: "WEEKEND", false: "HOLIDAY"}[strings.HasPrefix(w[i+1], "weekend")])
				i++
			}
		case (x == "from" || x == "after") && strings.HasPrefix(next, "complet"):
			fromCompletion = true
			i++
		case strings.HasPrefix(x, "x") && isNum(x[1:]):
			count, _ = strconv.Atoi(x[1:])
		case isNum(x) && (next == "time"):
			count, _ = strconv.Atoi(x)
			i++
		case isNum(x) && units[next] != "":
			interval, _ = strconv.Atoi(x)
		case monthDayNum(x) > 0:
			monthDays, e = append(monthDays, strconv.Itoa(monthDayNum(x))), setFreq("MONTHLY", true)
		default:
			return "", false, errors.New("couldn't read repeat: " + x)
		}
		if e != nil {
			return "", false, e
		}
	}
	if freq == "" {
		return "", false, errors.New("repeat: say how often, e.g. every 2 weeks")
	}
	if interval < 1 {
		return "", false, errors.New("repeat: interval must be at least 1")
	}
	if until != "" && count > 0 {
		return "", false, errors.New("repeat: until or xN, not both")
	}
	parts := []string{"FREQ=" + freq, "INTERVAL=" + strconv.Itoa(interval)}
	if month != "" {
		parts = append(parts, "BYMONTH="+month)
	}
	if len(days) > 0 {
		parts = append(parts, "BYDAY="+strings.Join(days, ","))
	}
	if len(monthDays) > 0 {
		parts = append(parts, "BYMONTHDAY="+strings.Join(monthDays, ","))
	}
	if workday != 0 {
		parts = append(parts, "TT_WORKDAY="+strconv.Itoa(workday))
	}
	if count > 0 {
		parts = append(parts, "COUNT="+strconv.Itoa(count))
	}
	if until != "" {
		parts = append(parts, "UNTIL="+until)
	}
	if len(skip) > 0 {
		slices.Sort(skip) // HOLIDAY,WEEKEND
		parts = append(parts, "TT_SKIP="+strings.Join(slices.Compact(skip), ","))
	}
	return "RRULE:" + strings.Join(parts, ";"), fromCompletion, nil
}

// RepeatText turns a repeatFlag back into ParseRepeat's syntax, for showing a custom rule
// and pre-filling its editor. Parts it doesn't know are shown raw.
func RepeatText(rule string, fromCompletion bool) string {
	s := repeatText(rule)
	if fromCompletion {
		s += " from completion"
	}
	return s
}

func repeatText(rule string) string {
	var out []string
	if r, ok := strings.CutPrefix(rule, "ERULE:"); ok {
		p := ruleParts(r)
		switch p["NAME"] {
		case "FORGETTINGCURVE":
			return "curve"
		case "CUSTOM":
			var ds []string
			for _, d := range strings.Split(p["BYDATE"], ",") {
				if t, err := time.Parse("20060102", d); err == nil {
					ds = append(ds, t.Format("2006-01-02"))
				}
			}
			return "dates " + strings.Join(ds, " ")
		}
		return rule
	}
	p := ruleParts(strings.TrimPrefix(rule, "RRULE:"))
	unit := map[string]string{"DAILY": "day", "WEEKLY": "week", "MONTHLY": "month", "YEARLY": "year"}[p["FREQ"]]
	if unit == "" {
		return rule
	}
	if n := p["INTERVAL"]; n != "" && n != "1" {
		out = append(out, "every "+n+" "+unit+"s")
	} else {
		out = append(out, "every "+unit)
	}
	known := map[string]bool{"FREQ": true, "INTERVAL": true, "BYMONTH": true}
	if m, err := strconv.Atoi(p["BYMONTH"]); err == nil && m >= 1 && m <= 12 {
		out = append(out, months[m-1][:3])
	}
	if d := p["BYDAY"]; d != "" {
		known["BYDAY"] = true
		if d == "MO,TU,WE,TH,FR" {
			out = append(out, "weekdays")
		} else {
			var ws []string
			for _, x := range strings.Split(d, ",") {
				n, day := x[:len(x)-2], x[len(x)-2:]
				i := slices.Index(byDay, day)
				if i < 0 {
					return rule
				}
				name := weekdays[i][:3]
				if n != "" {
					name = ordText[n] + " " + name
				}
				ws = append(ws, name)
			}
			out = append(out, strings.Join(ws, ","))
		}
	}
	switch wd, md := p["TT_WORKDAY"], p["BYMONTHDAY"]; {
	case wd == "1":
		out = append(out, "first workday")
	case wd == "-1":
		out = append(out, "last workday")
	case md != "":
		var ds []string
		for _, x := range strings.Split(md, ",") {
			if n, _ := strconv.Atoi(x); n > 0 {
				ds = append(ds, Ordinal(n))
			} else {
				ds = append(ds, "last day")
			}
		}
		out = append(out, strings.Join(ds, ","))
	}
	known["TT_WORKDAY"], known["BYMONTHDAY"] = true, true
	if c := p["COUNT"]; c != "" {
		out = append(out, "x"+c)
	}
	if u := p["UNTIL"]; len(u) >= 8 {
		if t, err := time.Parse("20060102", u[:8]); err == nil {
			out = append(out, "until "+t.Format("2006-01-02"))
		}
	}
	if s := p["TT_SKIP"]; s != "" {
		out = append(out, "skip "+strings.ToLower(strings.ReplaceAll(s, ",", " ")))
	}
	known["COUNT"], known["UNTIL"], known["TT_SKIP"] = true, true, true
	for k, v := range p {
		if !known[k] {
			out = append(out, k+"="+v)
		}
	}
	return strings.Join(out, " ")
}

func ruleParts(r string) map[string]string {
	p := map[string]string{}
	for _, kv := range strings.Split(r, ";") {
		k, v, _ := strings.Cut(kv, "=")
		p[k] = v
	}
	return p
}

// weekday returns the RRULE code of a weekday name ("wed", "wednesday"), or "".
func weekday(w string) string {
	if len(w) < 3 {
		return ""
	}
	for i, d := range weekdays {
		if strings.HasPrefix(d, w) {
			return byDay[i]
		}
	}
	return ""
}

// monthNum returns 1–12 for a month name of at least 3 letters ("oct", "october"), or 0.
func monthNum(w string) int {
	if len(w) < 3 {
		return 0
	}
	for i, m := range months {
		if strings.HasPrefix(m, w) {
			return i + 1
		}
	}
	return 0
}

func isNum(s string) bool {
	n, err := strconv.Atoi(s)
	return err == nil && n > 0
}

// monthDayNum reads a day of the month, "23" or "23rd" (1–31), or 0.
func monthDayNum(w string) int {
	for _, suf := range []string{"st", "nd", "rd", "th"} {
		w = strings.TrimSuffix(w, suf)
	}
	if n, err := strconv.Atoi(w); err == nil && n >= 1 && n <= 31 {
		return n
	}
	return 0
}

// Ordinal writes 1st, 2nd, 3rd, 4th … 11th, 12th, 13th … 21st.
func Ordinal(n int) string {
	suf := "th"
	if n%100 < 11 || n%100 > 13 {
		if s, ok := map[int]string{1: "st", 2: "nd", 3: "rd"}[n%10]; ok {
			suf = s
		}
	}
	return fmt.Sprint(n, suf)
}
