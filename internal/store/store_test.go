package store

import (
	"testing"
	"time"

	"ttui/internal/api"
)

// now is Wed 30 Sep 2026, 10:00 local.
var now = time.Date(2026, 9, 30, 10, 0, 0, 0, time.Local)

func due(days int, hhmm string) string {
	d := now.AddDate(0, 0, days)
	h, m := 0, 0
	if hhmm != "" {
		t, _ := time.Parse("15:04", hhmm)
		h, m = t.Hour(), t.Minute()
	}
	return time.Date(d.Year(), d.Month(), d.Day(), h, m, 0, 0, time.Local).Format(api.DateLayout)
}

func TestDueLabel(t *testing.T) {
	cases := []struct {
		days  int
		hhmm  string
		short string
		long  string
		role  string
	}{
		{-1, "10:30", "yesterday", "Yesterday, 10:30 · overdue", "error"},
		{-3, "", "3d ago", "Sun 27 Sep · overdue", "error"},
		{0, "17:00", "17:00", "Today, 17:00", "secondary"},
		{0, "", "today", "Today", "secondary"},
		{1, "", "tomorrow", "Tomorrow", "sub"},
		{2, "", "fri", "Fri 2 Oct", "muted"},
		{8, "", "oct 8", "Thu 8 Oct", "muted"},
	}
	for _, c := range cases {
		task := &api.Task{DueDate: due(c.days, c.hhmm), IsAllDay: c.hhmm == ""}
		got := DueLabel(task, now)
		if got == nil || got.Short != c.short || got.Long != c.long || got.Role != c.role {
			t.Errorf("days=%d %q: got %+v", c.days, c.hhmm, got)
		}
	}
	if DueLabel(&api.Task{}, now) != nil {
		t.Error("no due date should give nil")
	}
	// API responses carry milliseconds.
	ms := &api.Task{DueDate: "2026-10-02T09:00:00.000+0000"}
	if _, ok := DueTime(ms); !ok {
		t.Error("millisecond date not parsed")
	}
}

func TestAllDayUsesTaskZone(t *testing.T) {
	// All-day on 2 Oct in Rome = 2026-10-01T22:00:00Z.
	task := &api.Task{DueDate: "2026-10-01T22:00:00.000+0000", IsAllDay: true, TimeZone: "Europe/Rome"}
	d, _ := DueTime(task)
	if d.Day() != 2 || d.Month() != 10 {
		t.Fatalf("got %v", d)
	}
}

func TestListsAndGroups(t *testing.T) {
	tasks := []api.Task{
		{ID: "a", ProjectID: "inbox123", Title: "b-high-today", Priority: 5, DueDate: due(0, "17:00")},
		{ID: "b", ProjectID: "p1", Title: "a-high-today-allday", Priority: 5, DueDate: due(0, ""), IsAllDay: true},
		{ID: "c", ProjectID: "p1", Title: "overdue-med", Priority: 3, DueDate: due(-2, "09:00"), Tags: []string{"Work"}},
		{ID: "d", ProjectID: "p1", Title: "tomorrow-low", Priority: 1, DueDate: due(1, "")},
		{ID: "e", ProjectID: "p2", Title: "nodate", Priority: 0},
		{ID: "f", ProjectID: "p2", Title: "next week high", Priority: 5, DueDate: due(6, "")},
		{ID: "g", ProjectID: "p2", Title: "done today", Status: 2, DueDate: due(0, ""), CompletedTime: "2026-09-30T08:00:00.000+0000"},
	}
	s := New([]api.Project{{ID: "p2", SortOrder: 2}, {ID: "p1", SortOrder: 1}, {ID: "px", Closed: true}}, nil, tasks, "", now)

	ids := func(list string) string {
		out := ""
		for _, t := range s.TasksFor(list, now) {
			out += t.ID
		}
		return out
	}
	for list, want := range map[string]string{
		"inbox": "a", "today": "abcg", "tomorrow": "d", "next7": "abcdfg",
		"f-high": "abf", "f-nodate": "e", "completed": "g", "p:p1": "bcd", "tag:work": "c",
	} {
		if got := ids(list); got != want {
			t.Errorf("%s = %q, want %q", list, got, want)
		}
	}
	if n := s.OpenCount("today", now); n != 3 {
		t.Errorf("today open = %d", n)
	}
	if s.Projects[0].ID != "p1" || len(s.Projects) != 2 {
		t.Errorf("projects not sorted/filtered: %+v", s.Projects)
	}
	if len(s.Tags) != 1 || s.Tags[0] != "work" {
		t.Errorf("tags = %v", s.Tags)
	}

	gs := Groups(s.TasksFor("today", now), "due", true, now)
	labels := ""
	for _, g := range gs {
		labels += g.Label + ":"
		for _, t := range g.Items {
			labels += t.ID
		}
		labels += " "
	}
	// High: timed before all-day; Medium: overdue; Completed last.
	if labels != "High:ab Medium:c Completed:g " {
		t.Errorf("groups = %q", labels)
	}
	gs = Groups(s.TasksFor("today", now), "title", false, now)
	if gs[0].Items[0].ID != "b" || len(gs) != 2 {
		t.Errorf("title sort / hide completed: %+v", gs)
	}
}

func TestRepeatLabel(t *testing.T) {
	for in, want := range map[string]string{
		"": "", "RRULE:FREQ=DAILY;INTERVAL=1": "Daily", "RRULE:FREQ=WEEKLY;BYDAY=MO,TU,WE,TH,FR": "Weekdays",
		"RRULE:FREQ=WEEKLY;INTERVAL=1": "Weekly", "RRULE:FREQ=MONTHLY": "Monthly", "RRULE:FREQ=WEEKLY;INTERVAL=2": "Custom",
	} {
		if got := RepeatLabel(in); got != want {
			t.Errorf("RepeatLabel(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestMutationsAndSnapshot(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	s := New(nil, nil, []api.Task{{ID: "a", Tags: []string{"x"}}}, "", now)
	before := Clone(*s.Task("a"))
	s.Task("a").Tags[0] = "changed"
	if before.Tags[0] != "x" {
		t.Fatal("Clone shares slices")
	}
	s.Upsert(api.Task{ID: "b", Tags: []string{"y"}})
	s.Upsert(before)
	if len(s.Tasks) != 2 || s.Task("a").Tags[0] != "x" || len(s.Tags) != 2 {
		t.Fatalf("upsert: %+v tags=%v", s.Tasks, s.Tags)
	}
	s.Remove("b")
	if s.Task("b") != nil || len(s.Tags) != 1 {
		t.Fatal("remove")
	}
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}
	got := LoadSnapshot()
	if got == nil || got.Task("a") == nil || !got.SyncedAt.Equal(now) {
		t.Fatalf("snapshot round trip: %+v", got)
	}
}
