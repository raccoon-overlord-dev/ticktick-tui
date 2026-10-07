package ui

import (
	"fmt"
	"math"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/raccoon-overlord-dev/ticktick-tui/internal/api"
	"github.com/raccoon-overlord-dev/ticktick-tui/internal/parse"
	"github.com/raccoon-overlord-dev/ticktick-tui/internal/store"
)

// calendar is the month-grid date picker: due dates (d → Pick a date, the new-task Due
// field) and a repeat's specific dates (several days).
type calendar struct {
	title   string
	cur     time.Time       // cursor day, local midnight
	multi   bool            // pick several days: space toggles, ⏎ confirms
	picked  map[string]bool // multi: "20060102"
	clock   bool            // also offers a time (t)
	at      string          // the time as typed, "" = all day
	editing bool            // typing the time
	in      textInput
	done    func(c *calendar) tea.Cmd
}

func midnight(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, t.Location())
}

// addMonths moves t by n months, keeping the day where the month has it (31 Jan + 1 = 28 Feb).
func addMonths(t time.Time, n int) time.Time {
	first := time.Date(t.Year(), t.Month()+time.Month(n), 1, 0, 0, 0, 0, t.Location())
	last := first.AddDate(0, 1, -1).Day()
	return first.AddDate(0, 0, min(t.Day(), last)-1)
}

// daysFrom counts calendar days from now's date to d (DST-safe).
func daysFrom(now, d time.Time) int {
	return int(math.Round(midnight(d).Sub(midnight(now)).Hours() / 24))
}

// pickDue opens the calendar for t's due date, with its time.
func (a *App) pickDue(t *api.Task) {
	now := time.Now()
	c := &calendar{title: "due date", cur: midnight(now), clock: true}
	if d, ok := store.DueTime(t); ok {
		d = d.In(now.Location())
		c.cur = midnight(d)
		if !t.IsAllDay {
			c.at = d.Format("15:04")
		}
	}
	c.done = func(c *calendar) tea.Cmd {
		d := &parse.Due{Day: daysFrom(time.Now(), c.cur)}
		label := c.cur.Format("Mon ") + parse.FmtDayMonth(c.cur)
		if c.at != "" {
			d.H, d.M, d.HasTime = parseClock(c.at)
			label += " " + c.at
		}
		a.setFlash("due → " + label)
		return a.setDue(t, d)
	}
	a.cal = c
}

func parseClock(s string) (h, m int, ok bool) { return parse.ParseTime(strings.TrimSpace(s)) }

func (a *App) calKey(k tea.KeyPressMsg) tea.Cmd {
	c := a.cal
	key := k.String()
	if c.editing {
		switch key {
		case "enter":
			v := strings.TrimSpace(c.in.value())
			if _, _, ok := parseClock(v); v != "" && !ok {
				a.setFlash("time: try 17:00 or 5pm · empty = all day")
				return nil
			}
			c.at, c.editing = v, false
		case "esc":
			c.editing = false
		default:
			c.in.key(k, false)
		}
		return nil
	}
	vim := a.cfg.Keys.Keymap != "arrows"
	switch {
	case key == "left" || (vim && key == "h"):
		c.cur = c.cur.AddDate(0, 0, -1)
	case key == "right" || (vim && key == "l"):
		c.cur = c.cur.AddDate(0, 0, 1)
	case key == "up" || (vim && key == "k"):
		c.cur = c.cur.AddDate(0, 0, -7)
	case key == "down" || (vim && key == "j"):
		c.cur = c.cur.AddDate(0, 0, 7)
	case key == "pgup" || key == "[" || key == "<":
		c.cur = addMonths(c.cur, -1)
	case key == "pgdown" || key == "]" || key == ">":
		c.cur = addMonths(c.cur, 1)
	case key == "home" || key == ".":
		c.cur = midnight(time.Now())
	case key == "t" && c.clock:
		c.editing, c.in = true, newInput(c.at)
		c.in.selectAll()
	case key == "space" && c.multi:
		k := c.cur.Format("20060102")
		if c.picked[k] {
			delete(c.picked, k)
		} else {
			c.picked[k] = true
		}
	case key == "enter" || key == "space" || key == "ctrl+s":
		if c.multi && len(c.picked) == 0 {
			c.picked[c.cur.Format("20060102")] = true
		}
		a.cal = nil
		return c.done(c)
	case key == "esc" || key == "q":
		a.cal = nil
	}
	return nil
}

// viewCal renders the calendar box and returns it with its size.
func (a *App) viewCal() (string, int, int) {
	c := a.cal
	p := a.overlayPen()
	w := min(36, a.w-4)
	now := midnight(time.Now())
	first := time.Date(c.cur.Year(), c.cur.Month(), 1, 0, 0, 0, 0, c.cur.Location())
	ws := time.Monday
	if a.cfg.Tasks.WeekStart == "sun" {
		ws = time.Sunday
	}
	start := first.AddDate(0, 0, -((int(first.Weekday()) - int(ws) + 7) % 7))

	lines := []string{p.line(w, p.s("accent").Bold(true).Render(c.cur.Format("January 2006")), p.s("dim").Render("‹ pgup  pgdn ›")), ""}
	var head []string
	for i := range 7 {
		head = append(head, p.s("muted").Render(fmt.Sprintf(" %s ", time.Weekday((int(ws) + i) % 7).String()[:2])))
	}
	lines = append(lines, p.line(w, strings.Join(head, ""), ""))
	for row := range 6 {
		var b strings.Builder
		for col := range 7 {
			d := start.AddDate(0, 0, row*7+col)
			st := p.s("text")
			switch {
			case d.Equal(c.cur) && c.multi && c.picked[d.Format("20060102")]:
				st = p.s("ok").Reverse(true).Bold(true).Underline(true)
			case d.Equal(c.cur):
				st = p.s("accent").Reverse(true).Bold(true)
			case c.multi && c.picked[d.Format("20060102")]:
				st = p.s("ok").Reverse(true)
			case d.Month() != c.cur.Month():
				st = p.s("dim")
			case d.Equal(now):
				st = p.s("accent").Bold(true).Underline(true)
			}
			b.WriteString(p.sp(1) + st.Render(fmt.Sprintf("%2d", d.Day())) + p.sp(1))
		}
		lines = append(lines, p.line(w, b.String(), ""))
	}
	lines = append(lines, "")
	if c.clock {
		v := p.s("text").Render(c.at)
		if c.at == "" {
			v = p.s("dim").Render("all day")
		}
		if c.editing {
			v = c.in.view(p, "text", 12)
		}
		lines = append(lines, p.line(w, p.s("muted").Render("Time  ")+v, p.s("dim").Render("t change")))
	}
	if c.multi {
		lines = append(lines, p.line(w, p.s("muted").Render(fmt.Sprintf("%d picked", len(c.picked))), p.s("dim").Render("␣ toggle")))
	}
	keys := "←→↑↓ day · ⏎ pick · esc"
	if c.editing {
		keys = "⏎ set time · esc back"
	}
	lines = append(lines, p.s("line").Render(strings.Repeat("─", w-2)), p.line(w, p.s("dim").Render(keys), ""))
	h := len(lines) + 2
	return a.frame(p, frameOpts{w: w, h: h, title: c.title, modal: true, topRight: "home today", body: lines}), w, h
}
