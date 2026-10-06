package parse

import (
	"errors"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
)

var timeRe = regexp.MustCompile(`^(\d{1,2})(?::(\d{2}))?(am|pm)?$`)

// ParseTime reads "17:00", "9:30", "5pm", "12am". A bare number is not a time.
func ParseTime(w string) (h, m int, ok bool) {
	g := timeRe.FindStringSubmatch(strings.ToLower(w))
	if g == nil || (g[2] == "" && g[3] == "") {
		return 0, 0, false
	}
	h, _ = strconv.Atoi(g[1])
	if g[2] != "" {
		m, _ = strconv.Atoi(g[2])
	}
	switch {
	case g[3] == "pm" && h < 12:
		h += 12
	case g[3] == "am" && h == 12:
		h = 0
	}
	if h > 23 || m > 59 {
		return 0, 0, false
	}
	return h, m, true
}

var weekdays = []string{"sunday", "monday", "tuesday", "wednesday", "thursday", "friday", "saturday"}

// ParseDay reads today/tod, tomorrow/tmr/tom, yesterday, weekday names ("fri", "friday") and
// ISO dates ("2026-10-08"),
// returning days from now. A weekday means its next occurrence (today's weekday = +7).
func ParseDay(w string, now time.Time) (int, bool) {
	switch w = strings.ToLower(w); w {
	case "today", "tod":
		return 0, true
	case "tomorrow", "tmr", "tom":
		return 1, true
	case "yesterday":
		return -1, true
	}
	if d, err := time.ParseInLocation("2006-01-02", w, now.Location()); err == nil {
		y, m, dd := now.Date()
		return int(d.Sub(time.Date(y, m, dd, 0, 0, 0, 0, now.Location())).Hours() / 24), true
	}
	if n, err := strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(w, "+"), "d")); err == nil && w[0] == '+' && strings.HasSuffix(w, "d") {
		return n, true // +3d = in 3 days
	}
	if len(w) < 3 {
		return 0, false
	}
	for i, d := range weekdays {
		if strings.HasPrefix(d, w) {
			diff := (i - int(now.Weekday()) + 7) % 7
			if diff == 0 {
				diff = 7
			}
			return diff, true
		}
	}
	return 0, false
}

// Due is a parsed due date relative to a reference day.
type Due struct {
	Day     int // days from the reference date
	HasTime bool
	H, M    int
}

// At returns the due moment in loc (midnight for all-day).
func (d Due) At(now time.Time) time.Time {
	y, mo, day := now.AddDate(0, 0, d.Day).Date()
	return time.Date(y, mo, day, d.H, d.M, 0, 0, now.Location())
}

// ParseDue reads the details "Due" field: "tomorrow 17:00", "fri", "17:00" (= today).
// Empty input returns nil (no due date).
func ParseDue(s string, now time.Time) (*Due, error) {
	words := strings.Fields(s)
	if len(words) == 0 {
		return nil, nil
	}
	d := &Due{} // a time without a day means today (Day 0)
	for _, w := range words {
		if h, m, ok := ParseTime(w); ok {
			d.H, d.M, d.HasTime = h, m, true
		} else if n, ok := ParseDay(w, now); ok {
			d.Day = n
		} else {
			return nil, errors.New("couldn't read date: " + w)
		}
	}
	return d, nil
}

// List is a list the quick-add "@list" token can refer to.
type List struct{ ID, Name string }

type Add struct {
	Title   string
	Prio    int // API value: 0 none, 1 low, 3 med, 5 high
	PrioSet bool
	Due     *Due
	DueSet  bool
	Tags    []string
	List    string
	ListSet bool
}

// QuickAdd parses "call mom tomorrow 17:00 !high #errand @home".
// defList / defDue are used when the text doesn't set them. With dates false, day and time
// words stay in the title (Settings → Smart dates off).
func QuickAdd(term string, lists []List, defList string, defDue *Due, now time.Time, dates bool) Add {
	a := Add{List: defList}
	var title []string
	var day *int
	var tm *Due
	for _, w := range strings.Fields(term) {
		lw := strings.ToLower(w)
		switch {
		case lw == "!h" || lw == "!high" || lw == "!3":
			a.Prio, a.PrioSet = 5, true
		case lw == "!m" || lw == "!med" || lw == "!medium" || lw == "!2":
			a.Prio, a.PrioSet = 3, true
		case lw == "!l" || lw == "!low" || lw == "!1":
			a.Prio, a.PrioSet = 1, true
		case len(lw) > 1 && lw[0] == '#':
			a.Tags = append(a.Tags, lw[1:])
		case len(lw) > 1 && lw[0] == '@':
			if id, ok := matchList(lw[1:], lists); ok {
				a.List, a.ListSet = id, true
			} else {
				title = append(title, w)
			}
		case !dates:
			title = append(title, w)
		default:
			if h, m, ok := ParseTime(lw); ok {
				tm = &Due{HasTime: true, H: h, M: m}
			} else if n, ok := ParseDay(lw, now); ok {
				day = &n
			} else {
				title = append(title, w)
			}
		}
	}
	a.Title = strings.Join(title, " ")
	switch {
	case day != nil || tm != nil:
		a.Due, a.DueSet = &Due{}, true
		if tm != nil {
			*a.Due = *tm
		}
		if day != nil {
			a.Due.Day = *day
		}
	case defDue != nil:
		a.Due = defDue
	}
	return a
}

// matchList is a prefix match on the list name, ignoring case, spaces and leading emoji.
func matchList(q string, lists []List) (string, bool) {
	for _, l := range lists {
		name := strings.ToLower(strings.ReplaceAll(l.Name, " ", ""))
		name = strings.TrimLeftFunc(name, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
		if strings.HasPrefix(name, q) {
			return l.ID, true
		}
	}
	return "", false
}

// Fuzzy matches term against label, case-insensitively. A substring match scores its index;
// otherwise an in-order subsequence (spaces in term ignored) scores 100 + its span.
// pos holds the matched rune indexes. Lower score is better.
func Fuzzy(label, term string) (score int, pos []int, ok bool) {
	if term == "" {
		return 0, nil, true
	}
	l := []rune(strings.ToLower(label))
	q := []rune(strings.ToLower(term))
	if i := strings.Index(string(l), string(q)); i >= 0 {
		start := len([]rune(string(l)[:i]))
		for k := range q {
			pos = append(pos, start+k)
		}
		return start, pos, true
	}
	q = []rune(strings.ReplaceAll(string(q), " ", ""))
	j := 0
	for k := 0; k < len(l) && j < len(q); k++ {
		if l[k] == q[j] {
			pos = append(pos, k)
			j++
		}
	}
	if j < len(q) || len(pos) == 0 {
		return 0, nil, false
	}
	return 100 + pos[len(pos)-1] - pos[0], pos, true
}
