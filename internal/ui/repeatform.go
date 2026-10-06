package ui

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/raccoon-overlord-dev/ticktick-tui/internal/api"
	"github.com/raccoon-overlord-dev/ticktick-tui/internal/parse"
	"github.com/raccoon-overlord-dev/ticktick-tui/internal/store"
)

// repForm is the Custom repeat dialog, laid out like the web app's: a mode, every N units,
// then the unit's own options. Rows are rebuilt from the form on every key.
type repForm struct {
	f    parse.RepeatForm
	row  int
	cell int // cursor in the weekday row or the day grid
	task *api.Task
}

var (
	repModes  = []string{"due", "completion", "dates"}
	repUnits  = []string{"day", "week", "month", "year"}
	repOrds   = []int{1, 2, 3, 4, 5, -1}
	repLabels = map[string]string{"due": "By due date", "completion": "By completion date", "dates": "By specific dates",
		"each": "Each", "on": "On the", "workday": "Workday"}
	ordLabel = map[int]string{1: "first", 2: "second", 3: "third", 4: "fourth", 5: "fifth", -1: "last"}
)

func (a *App) openRepeatForm(t *api.Task) {
	due, ok := store.DueTime(t)
	if !ok {
		a.setFlash("set a due date first · repeat needs one")
		return
	}
	a.rep = &repForm{f: parse.FormFromRule(t.RepeatFlag, t.RepeatFrom, due.In(time.Now().Location())), task: t}
}

func (r *repForm) rows() []string {
	f := r.f
	rows := []string{"mode"}
	if f.Mode == "dates" {
		return append(rows, "dates", "ok")
	}
	rows = append(rows, "every", "unit")
	if f.Mode == "due" {
		switch f.Unit {
		case "week":
			rows = append(rows, "days")
		case "month":
			rows = append(rows, "by")
			switch f.By {
			case "on":
				rows = append(rows, "ord", "weekday")
			case "workday":
				rows = append(rows, "workday")
			default:
				rows = append(rows, "grid")
			}
		case "year":
			rows = append(rows, "by", "month")
			if f.By == "on" {
				rows = append(rows, "ord", "weekday")
			} else {
				rows = append(rows, "day")
			}
		}
	}
	if f.CanSkip() {
		rows = append(rows, "skip")
	}
	return append(rows, "ok")
}

func cycle[T comparable](v *T, opts []T, dir int) {
	n := len(opts)
	*v = opts[((slices.Index(opts, *v)+dir)%n+n)%n]
}

func toggle[T comparable](xs []T, x T) []T {
	if i := slices.Index(xs, x); i >= 0 {
		return slices.Delete(xs, i, i+1)
	}
	return append(xs, x)
}

// change is ←→ (dir ±1) on a row.
func (r *repForm) change(row string, dir int) {
	f := &r.f
	switch row {
	case "mode":
		cycle(&f.Mode, repModes, dir)
	case "every":
		f.Every = max(1, f.Every+dir)
	case "unit":
		cycle(&f.Unit, repUnits, dir)
	case "by":
		opts := []string{"each", "on", "workday"}
		if f.Unit == "year" {
			opts = opts[:2]
		}
		cycle(&f.By, opts, dir)
	case "ord":
		cycle(&f.Ord, repOrds, dir)
	case "weekday":
		cycle(&f.Weekday, parse.WeekCodes, dir)
	case "workday":
		cycle(&f.Workday, []int{1, -1}, dir)
	case "month":
		f.Month = (f.Month-1+dir+12)%12 + 1
	case "day":
		f.Day = (f.Day-1+dir+31)%31 + 1
	case "days":
		r.cell = (r.cell + dir + 7) % 7
	case "grid":
		r.cell = (r.cell + dir + 32) % 32
	case "skip":
		f.Skip = !f.Skip
	}
	if f.Unit == "year" && f.By == "workday" {
		f.By = "each"
	}
}

func (a *App) repKey(k tea.KeyPressMsg) tea.Cmd {
	r := a.rep
	rows := r.rows()
	r.row = min(r.row, len(rows)-1)
	row := rows[r.row]
	key := k.String()
	vim := a.cfg.Keys.Keymap != "arrows"
	if vim {
		if alias, ok := map[string]string{"j": "down", "k": "up", "h": "left", "l": "right"}[key]; ok {
			key = alias
		}
	}
	num := func(v *int, limit int) { // type digits into a number row
		if d := int(k.Code - '0'); len(key) == 1 && d >= 0 && d <= 9 {
			if *v = *v*10 + d; *v > limit {
				*v = d
			}
		}
		if key == "backspace" {
			*v /= 10
		}
	}
	switch key {
	case "esc", "q":
		a.rep = nil
	case "ctrl+s":
		return a.saveRepeat()
	case "up", "down", "tab", "shift+tab":
		d := map[bool]int{true: -1, false: 1}[key == "up" || key == "shift+tab"]
		if row == "grid" && (key == "up" || key == "down") && r.cell+7*d >= 0 && r.cell+7*d < 32 {
			r.cell += 7 * d
			return nil
		}
		r.row, r.cell = step(r.row, d, len(rows)), 0
	case "left", "right":
		r.change(row, map[bool]int{true: -1, false: 1}[key == "left"])
	case "enter", "space":
		switch row {
		case "ok":
			return a.saveRepeat()
		case "days":
			r.f.Days = toggle(r.f.Days, parse.WeekCodes[r.cell])
		case "grid":
			day := r.cell + 1
			if r.cell == 31 {
				day = -1
			}
			r.f.MonthDays = toggle(r.f.MonthDays, day)
		case "dates":
			c := &calendar{title: "repeat on", cur: midnight(time.Now()), multi: true, picked: map[string]bool{}}
			for _, d := range r.f.Dates {
				c.picked[d] = true
			}
			if len(r.f.Dates) > 0 {
				c.cur, _ = time.ParseInLocation("20060102", r.f.Dates[0], time.Local)
			}
			c.done = func(c *calendar) tea.Cmd {
				r.f.Dates = r.f.Dates[:0]
				for d := range c.picked {
					r.f.Dates = append(r.f.Dates, d)
				}
				slices.Sort(r.f.Dates)
				return nil
			}
			a.cal = c
		case "every", "day":
		default:
			r.change(row, 1)
		}
	default:
		switch row {
		case "every":
			num(&r.f.Every, 99)
			r.f.Every = max(r.f.Every, 1)
		case "day":
			num(&r.f.Day, 31)
			r.f.Day = max(r.f.Day, 1)
		}
	}
	return nil
}

func (a *App) saveRepeat() tea.Cmd {
	r := a.rep
	rule, from := r.f.Rule()
	if rule == "" {
		a.setFlash("pick at least one date")
		return nil
	}
	a.rep = nil
	a.setFlash("repeat → " + parse.RepeatText(rule, from))
	return a.setRepeat(r.task, rule, from)
}

// viewRepeat renders the repeat dialog and returns it with its size.
func (a *App) viewRepeat() (string, int, int) {
	r := a.rep
	f := r.f
	p := a.overlayPen()
	w := min(52, a.w-4)
	rows := r.rows()
	r.row = min(r.row, len(rows)-1)
	var lines []string
	sel := 0
	for i, row := range rows {
		on := i == r.row
		rp := p
		if on {
			sel, rp = len(lines), p.on(a.selBg(true))
		}
		choice := func(s string) string {
			if on {
				return rp.s("accent").Render("‹ ") + rp.s("text").Bold(true).Render(s) + rp.s("accent").Render(" ›")
			}
			return rp.s("text").Render(s)
		}
		label := func(s string) string { return rp.s("muted").Render(padRight(s, 11)) }
		switch row {
		case "mode":
			lines = append(lines, rp.line(w, label("Repeat")+choice(repLabels[f.Mode]), ""))
		case "every":
			lines = append(lines, rp.line(w, label("Every")+rp.s("text").Bold(on).Render(strconv.Itoa(f.Every)), hintIf(rp, on, "←→ or type")))
		case "unit":
			u := f.Unit
			if f.Every > 1 {
				u += "s"
			}
			lines = append(lines, rp.line(w, label("Unit")+choice(u), ""))
		case "days":
			var b strings.Builder
			for j, code := range parse.WeekCodes {
				st := rp.s("text")
				if slices.Contains(f.Days, code) {
					st = rp.s("ok").Reverse(true).Bold(true)
				}
				if on && j == r.cell {
					st = st.Underline(true)
					if !slices.Contains(f.Days, code) {
						st = rp.s("accent").Bold(true).Underline(true)
					}
				}
				b.WriteString(st.Render(" "+parse.WeekdayName(code)[:1]+" ") + rp.sp(1))
			}
			lines = append(lines, rp.line(w, label("On")+b.String(), hintIf(rp, on, "␣ toggle")))
		case "by":
			lbl := "Month by"
			if f.Unit == "year" {
				lbl = "Year by"
			}
			lines = append(lines, rp.line(w, label(lbl)+choice(repLabels[f.By]), ""))
		case "grid":
			for gr := 0; gr*7 < 32; gr++ {
				var b strings.Builder
				for j := gr * 7; j < min(gr*7+7, 32); j++ {
					day, txt := j+1, fmt.Sprintf("%2d", j+1)
					if j == 31 {
						day, txt = -1, "Last day"
					}
					st := rp.s("text")
					if slices.Contains(f.MonthDays, day) {
						st = rp.s("ok").Reverse(true).Bold(true)
					}
					if on && j == r.cell {
						st = st.Underline(true)
						if !slices.Contains(f.MonthDays, day) {
							st = rp.s("accent").Bold(true).Underline(true)
						}
					}
					b.WriteString(rp.sp(1) + st.Render(txt) + rp.sp(1))
				}
				lead, right := rp.sp(11), ""
				if gr == 0 {
					lead, right = label("Days"), hintIf(rp, on, "␣ toggle")
				}
				lines = append(lines, rp.line(w, lead+b.String(), right))
			}
		case "ord":
			lines = append(lines, rp.line(w, label("On the")+choice(ordLabel[f.Ord]), ""))
		case "weekday":
			lines = append(lines, rp.line(w, label("Weekday")+choice(parse.WeekdayName(f.Weekday)), ""))
		case "workday":
			lines = append(lines, rp.line(w, label("Workday")+choice(map[int]string{1: "First workday", -1: "Last workday"}[f.Workday]), ""))
		case "month":
			lines = append(lines, rp.line(w, label("Month")+choice(parse.MonthName(f.Month)), ""))
		case "day":
			lines = append(lines, rp.line(w, label("Day")+rp.s("text").Bold(on).Render(strconv.Itoa(f.Day)), hintIf(rp, on, "←→ or type")))
		case "skip":
			box := "[ ]"
			if f.Skip {
				box = "[x]"
			}
			lines = append(lines, rp.line(w, rp.s("accent").Render(box)+rp.sp(1)+rp.s("text").Render("Skip weekends"), hintIf(rp, on, "␣ toggle")))
		case "dates":
			var ds []string
			for _, d := range f.Dates {
				if t, err := time.Parse("20060102", d); err == nil {
					ds = append(ds, t.Format("2 Jan"))
				}
			}
			v := rp.s("dim").Render("none yet")
			if len(ds) > 0 {
				v = rp.s("text").Render(trunc(strings.Join(ds, ", "), w-4-11-10))
			}
			lines = append(lines, rp.line(w, label("Dates")+v, hintIf(rp, on, "⏎ pick")))
		case "ok":
			lines = append(lines, "", rp.line(w, rp.s("accent").Bold(true).Render("[ OK ]"), ""))
			if on {
				sel = len(lines) - 1
			}
		}
	}
	rule, from := f.Rule()
	summary := p.s("dim").Render("pick at least one date")
	if rule != "" {
		summary = p.s("secondary").Render("↻ " + trunc(parse.RepeatText(rule, from), w-6))
	}
	lines = append(lines, "", p.line(w, summary, ""), p.s("line").Render(strings.Repeat("─", w-2)),
		p.line(w, p.s("dim").Render("↑↓ field · ←→ change · ␣ pick · ctrl+s save"), ""))
	body := window(lines, sel, max(a.h-3, 3), new(int))
	h := len(body) + 2
	return a.frame(p, frameOpts{w: w, h: h, title: "Custom repeat", modal: true, body: body}), w, h
}

func hintIf(p pen, on bool, s string) string {
	if !on {
		return ""
	}
	return p.s("dim").Render(s)
}
