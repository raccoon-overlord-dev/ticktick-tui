package store

import (
	"time"

	"ttui/internal/api"
)

// Demo returns the design mockup's data (handoff screenshots), dated relative to now.
// Used by `ttui dev demo`; never touches the API.
func Demo(now time.Time) *Store {
	at := func(days int, hhmm string) string {
		d := now.AddDate(0, 0, days)
		t, _ := time.Parse("15:04", hhmm)
		return time.Date(d.Year(), d.Month(), d.Day(), t.Hour(), t.Minute(), 0, 0, time.Local).Format(api.DateLayout)
	}
	allDay := func(days int) (string, bool) { return at(days, "00:00"), true }
	task := func(id, list, title string, prio int, days int, hhmm string, extra func(*api.Task)) api.Task {
		t := api.Task{ID: id, ProjectID: list, Title: title, Priority: prio, CreatedTime: id}
		if days > -99 {
			if hhmm == "" {
				t.DueDate, t.IsAllDay = allDay(days)
			} else {
				t.DueDate = at(days, hhmm)
			}
		}
		if extra != nil {
			extra(&t)
		}
		return t
	}
	const none = -99
	items := func(done, total int, titles ...string) []api.Item {
		var out []api.Item
		for i := 0; i < total; i++ {
			st := 0
			if i < done {
				st = 2
			}
			out = append(out, api.Item{ID: string(rune('a' + i)), Title: titles[i], Status: st})
		}
		return out
	}
	repeat := func(r string) func(*api.Task) { return func(t *api.Task) { t.RepeatFlag = r } }

	projects := []api.Project{
		{ID: "roadmap", Name: "Q4 Roadmap", GroupID: "work", SortOrder: 1},
		{ID: "standups", Name: "Standups", GroupID: "work", SortOrder: 2},
		{ID: "hiring", Name: "Hiring", GroupID: "work", SortOrder: 3},
		{ID: "home", Name: "Home", GroupID: "personal", SortOrder: 4},
		{ID: "groceries", Name: "Groceries", GroupID: "personal", SortOrder: 5},
		{ID: "reading", Name: "Reading", GroupID: "personal", SortOrder: 6},
		{ID: "ttui", Name: "ttui", GroupID: "side", SortOrder: 7},
	}
	groups := []api.Group{{ID: "work", Name: "Work", SortOrder: 1}, {ID: "personal", Name: "Personal", SortOrder: 2}, {ID: "side", Name: "Side projects", SortOrder: 3}}

	tasks := []api.Task{
		task("01", "home", "Call dentist to reschedule", PrioHigh, -1, "10:30", nil),
		task("02", "ttui", "Ship ttui v0.3 release notes", PrioHigh, 0, "17:00", func(t *api.Task) {
			t.Tags = []string{"deep-work", "release"}
			t.Items = items(3, 5, "Draft changelog", "Screenshot the three themes", "Record 40s demo gif", "Proofread install section", "Tag v0.3.0 and push")
			t.Content = "Focus on the **responsive layout** and the new themes.\n\n- 3 → 2 → 1 column breakpoints\n- `colorful` and `lotr` palettes\n- natural-language quick add\n\nLink the demo gif at the top of the README."
		}),
		task("03", "roadmap", "Review Q4 roadmap draft", PrioHigh, 0, "", func(t *api.Task) {
			t.Items = items(1, 2, "Read draft", "Leave comments")
			t.Tags = []string{"deep-work"}
		}),
		task("04", "hiring", "Reply to Lena about the interview panel", PrioMed, 0, "14:00", func(t *api.Task) { t.Tags = []string{"waiting"} }),
		task("05", "inbox1", "Pick up dry cleaning", PrioMed, 0, "18:30", func(t *api.Task) { t.Tags = []string{"errand"} }),
		task("06", "home", "Pay electricity bill", PrioMed, 0, "", repeat("RRULE:FREQ=MONTHLY;INTERVAL=1")),
		task("07", "home", "Water the plants", PrioLow, 0, "", func(t *api.Task) {
			t.RepeatFlag = "RRULE:FREQ=WEEKLY;INTERVAL=1"
			t.Tags = []string{"home"}
		}),
		task("08", "reading", "Read ch. 7 of Designing Data-Intensive Apps", PrioLow, 0, "", nil),
		task("09", "inbox1", "Sort photos from Lisbon", PrioNone, 0, "", nil),
		task("10", "standups", "Standup notes", PrioNone, 0, "09:30", func(t *api.Task) {
			t.Status, t.RepeatFlag, t.CompletedTime = 2, "RRULE:FREQ=DAILY", at(0, "09:40")
		}),
		task("11", "home", "Morning run", PrioNone, 0, "07:00", func(t *api.Task) {
			t.Status, t.RepeatFlag, t.CompletedTime = 2, "RRULE:FREQ=DAILY", at(0, "07:45")
		}),
		task("12", "groceries", "Oat milk, eggs, coffee beans", PrioNone, 1, "", func(t *api.Task) { t.Tags = []string{"errand"} }),
		task("13", "roadmap", "Draft OKRs for platform team", PrioHigh, 3, "", nil),
		task("14", "hiring", "Schedule onsite for backend candidate", PrioMed, 1, "11:00", func(t *api.Task) { t.Tags = []string{"waiting"} }),
		task("15", "inbox1", "Book flights for December", PrioLow, none, "", nil),
		task("16", "reading", "Finish “The Dispossessed”", PrioNone, none, "", nil),
		task("17", "standups", "Weekly review", PrioLow, 2, "16:00", repeat("RRULE:FREQ=WEEKLY;INTERVAL=1")),
		task("18", "home", "Call landlord about heating", PrioMed, 5, "", func(t *api.Task) { t.Tags = []string{"waiting"} }),
		task("19", "groceries", "Pick up parcel", PrioNone, 1, "", func(t *api.Task) { t.Tags = []string{"errand"} }),
		task("20", "ttui", "Write install.sh", PrioMed, 8, "", func(t *api.Task) { t.Tags = []string{"release"} }),
	}
	return New(projects, groups, tasks, "jane@doe.dev", now.Add(-2*time.Minute))
}
