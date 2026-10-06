package parse

import (
	"slices"
	"strconv"
	"strings"
	"time"
)

// RepeatForm is the web app's Custom repeat dialog as data. Rule writes the repeatFlag the
// web app writes for it; FormFromRule reads one back.
type RepeatForm struct {
	Mode      string   // "due" (by due date), "completion" (by completion date), "dates" (specific dates)
	Every     int      // interval
	Unit      string   // "day", "week", "month", "year"
	Days      []string // week: BYDAY codes, "MO"…
	By        string   // month: "each", "on" or "workday"; year: "each" or "on"
	MonthDays []int    // month, each: 1–31, -1 = last day
	Ord       int      // on the: 1–5, -1 = last
	Weekday   string   // on the: "MO"…
	Workday   int      // month, workday: 1 first, -1 last
	Month     int      // year: 1–12
	Day       int      // year, each: 1–31
	Skip      bool     // skip weekends
	Dates     []string // specific dates, "20261006", sorted
	Keep      []string // rule parts the form doesn't show (COUNT, UNTIL, TT_SKIP=HOLIDAY…), kept as they are
}

// CanSkip reports whether the form offers Skip weekends, as the web app does.
func (f RepeatForm) CanSkip() bool {
	switch f.Mode {
	case "completion":
		return f.Unit == "day" || f.Unit == "month"
	case "due":
		return f.Unit == "day" || (f.Unit == "month" && f.By == "each")
	}
	return false
}

// Rule returns the repeatFlag and whether it repeats from the completion date. Specific
// dates without any date return "".
func (f RepeatForm) Rule() (string, bool) {
	if f.Mode == "dates" {
		if len(f.Dates) == 0 {
			return "", false
		}
		return "ERULE:NAME=CUSTOM;BYDATE=" + strings.Join(f.Dates, ","), false
	}
	freq := map[string]string{"day": "DAILY", "week": "WEEKLY", "month": "MONTHLY", "year": "YEARLY"}[f.Unit]
	parts := []string{"FREQ=" + freq, "INTERVAL=" + strconv.Itoa(max(f.Every, 1))}
	on := func() string { return "BYDAY=" + strconv.Itoa(f.Ord) + f.Weekday }
	if f.Mode == "due" {
		switch f.Unit {
		case "week":
			if len(f.Days) > 0 {
				ds := slices.Clone(f.Days)
				slices.SortFunc(ds, func(a, b string) int { return weekIndex(a) - weekIndex(b) })
				parts = append(parts, "BYDAY="+strings.Join(ds, ","))
			}
		case "month":
			switch f.By {
			case "on":
				parts = append(parts, on())
			case "workday":
				w := strconv.Itoa(f.Workday)
				parts = append(parts, "BYMONTHDAY="+w, "TT_WORKDAY="+w)
			default:
				if len(f.MonthDays) > 0 {
					ds := slices.Clone(f.MonthDays)
					slices.SortFunc(ds, func(a, b int) int { return dayKey(a) - dayKey(b) })
					var s []string
					for _, d := range ds {
						s = append(s, strconv.Itoa(d))
					}
					parts = append(parts, "BYMONTHDAY="+strings.Join(s, ","))
				}
			}
		case "year":
			parts = append(parts, "BYMONTH="+strconv.Itoa(f.Month))
			if f.By == "on" {
				parts = append(parts, on())
			} else {
				parts = append(parts, "BYMONTHDAY="+strconv.Itoa(f.Day))
			}
		}
	}
	keep := slices.Clone(f.Keep)
	skip := []string{}
	if i := slices.IndexFunc(keep, func(p string) bool { return strings.HasPrefix(p, "TT_SKIP=") }); i >= 0 {
		skip = append(skip, strings.Split(strings.TrimPrefix(keep[i], "TT_SKIP="), ",")...)
		keep = slices.Delete(keep, i, i+1)
	}
	if f.Skip && f.CanSkip() {
		skip = append(skip, "WEEKEND")
	}
	parts = append(parts, keep...)
	if len(skip) > 0 {
		slices.Sort(skip) // HOLIDAY,WEEKEND
		parts = append(parts, "TT_SKIP="+strings.Join(slices.Compact(skip), ","))
	}
	return "RRULE:" + strings.Join(parts, ";"), f.Mode == "completion"
}

// FormFromRule reads a repeatFlag into the form. Fields the rule doesn't set start from the
// due date, as the web app's dialog does. A rule the form can't show (Ebbinghaus) gives the
// defaults.
func FormFromRule(rule, repeatFrom string, due time.Time) RepeatForm {
	wd := byDay[due.Weekday()]
	f := RepeatForm{Mode: "due", Every: 1, Unit: "week", Days: []string{wd}, By: "each", MonthDays: []int{due.Day()},
		Ord: min((due.Day()-1)/7+1, 5), Weekday: wd, Workday: 1, Month: int(due.Month()), Day: due.Day()}
	if r, ok := strings.CutPrefix(rule, "ERULE:"); ok {
		if p := ruleParts(r); p["NAME"] == "CUSTOM" {
			f.Mode, f.Dates = "dates", strings.Split(p["BYDATE"], ",")
			slices.Sort(f.Dates)
		}
		return f
	}
	r, ok := strings.CutPrefix(rule, "RRULE:")
	if !ok {
		return f
	}
	if repeatFrom == "1" {
		f.Mode = "completion"
	}
	var setpos int
	for _, kv := range strings.Split(r, ";") {
		k, v, _ := strings.Cut(kv, "=")
		switch k {
		case "FREQ":
			f.Unit = map[string]string{"DAILY": "day", "WEEKLY": "week", "MONTHLY": "month", "YEARLY": "year"}[v]
		case "INTERVAL":
			f.Every, _ = strconv.Atoi(v)
		case "BYMONTH":
			f.Month, _ = strconv.Atoi(v)
		case "BYSETPOS":
			setpos, _ = strconv.Atoi(v)
		case "BYDAY":
			ds := strings.Split(v, ",")
			if code := ds[0][max(len(ds[0])-2, 0):]; len(ds) == 1 && len(ds[0]) > 2 {
				f.By, f.Weekday = "on", code
				f.Ord, _ = strconv.Atoi(ds[0][:len(ds[0])-2])
			} else {
				f.Days, f.Weekday = ds, code
			}
		case "BYMONTHDAY":
			f.MonthDays = nil
			for _, x := range strings.Split(v, ",") {
				n, _ := strconv.Atoi(x)
				f.MonthDays = append(f.MonthDays, n)
			}
			f.Day = f.MonthDays[0]
		case "TT_WORKDAY":
			f.By = "workday"
			f.Workday, _ = strconv.Atoi(v)
		case "TT_SKIP":
			var rest []string
			for _, s := range strings.Split(v, ",") {
				if s == "WEEKEND" {
					f.Skip = true
				} else {
					rest = append(rest, s)
				}
			}
			if len(rest) > 0 {
				f.Keep = append(f.Keep, "TT_SKIP="+strings.Join(rest, ","))
			}
		default:
			f.Keep = append(f.Keep, kv)
		}
	}
	if setpos != 0 { // "BYDAY=WE;BYSETPOS=3" = 3rd Wednesday
		f.By, f.Ord = "on", setpos
	}
	if f.Unit == "" {
		return FormFromRule("", "", due)
	}
	f.Every = max(f.Every, 1)
	return f
}

func weekIndex(code string) int { return (slices.Index(byDay, code) + 6) % 7 } // Monday first

// dayKey sorts month days with the last day at the end.
func dayKey(d int) int {
	if d < 0 {
		return 100 - d
	}
	return d
}

// WeekCodes are the BYDAY codes Monday first, as the web app's dialog shows them.
var WeekCodes = []string{"MO", "TU", "WE", "TH", "FR", "SA", "SU"}

// WeekdayName is "Monday" for "MO".
func WeekdayName(code string) string {
	i := slices.Index(byDay, code)
	if i < 0 {
		return code
	}
	return strings.ToUpper(weekdays[i][:1]) + weekdays[i][1:]
}

// MonthName is "October" for 10.
func MonthName(m int) string {
	if m < 1 || m > 12 {
		return "?"
	}
	return strings.ToUpper(months[m-1][:1]) + months[m-1][1:]
}
