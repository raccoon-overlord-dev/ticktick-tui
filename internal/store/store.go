// Package store holds the in-memory TickTick data and the derived views (smart lists, groups, due labels).
package store

import (
	"cmp"
	"context"
	"slices"
	"strings"
	"time"

	"ttui/internal/api"
)

type Store struct {
	Projects []api.Project // sorted by sortOrder, closed ones dropped
	Groups   []api.Group   // folders, sorted by sortOrder
	Tasks    []api.Task    // open tasks + tasks completed in the last CompletedDays
	Tags     []string      // derived from tasks, sorted
	Email    string
	SyncedAt time.Time
}

// ponytail: only recently completed tasks are fetched; widen if people want older history.
const CompletedDays = 7

// Fetch loads everything in 5 requests (see docs/api-notes.md "Sync strategy").
func Fetch(ctx context.Context, c *api.Client) (*Store, error) {
	ps, err := c.Projects(ctx)
	if err != nil {
		return nil, err
	}
	gs, err := c.Groups(ctx)
	if err != nil {
		return nil, err
	}
	open, err := c.OpenTasks(ctx)
	if err != nil {
		return nil, err
	}
	done, err := c.CompletedTasks(ctx, time.Now().AddDate(0, 0, -CompletedDays))
	if err != nil {
		return nil, err
	}
	email, _ := c.Email(ctx) // optional
	return New(ps, gs, append(open, done...), email, time.Now()), nil
}

func New(ps []api.Project, gs []api.Group, ts []api.Task, email string, synced time.Time) *Store {
	s := &Store{Groups: gs, Tasks: ts, Email: email, SyncedAt: synced}
	for _, p := range ps {
		if !p.Closed {
			s.Projects = append(s.Projects, p)
		}
	}
	slices.SortStableFunc(s.Projects, func(a, b api.Project) int { return cmp.Compare(a.SortOrder, b.SortOrder) })
	slices.SortStableFunc(s.Groups, func(a, b api.Group) int { return cmp.Compare(a.SortOrder, b.SortOrder) })
	s.rebuildTags()
	return s
}

func (s *Store) rebuildTags() {
	s.Tags = nil
	seen := map[string]bool{}
	for _, t := range s.Tasks {
		for _, tag := range t.Tags {
			if l := strings.ToLower(tag); !seen[l] {
				seen[l] = true
				s.Tags = append(s.Tags, l)
			}
		}
	}
	slices.Sort(s.Tags)
}

// Task returns the task with id, or nil.
func (s *Store) Task(id string) *api.Task {
	for i := range s.Tasks {
		if s.Tasks[i].ID == id {
			return &s.Tasks[i]
		}
	}
	return nil
}

// Upsert replaces the task with t.ID, or adds t.
func (s *Store) Upsert(t api.Task) {
	if old := s.Task(t.ID); old != nil {
		*old = t
	} else {
		s.Tasks = append(s.Tasks, t)
	}
	s.rebuildTags()
}

func (s *Store) Remove(id string) {
	s.Tasks = slices.DeleteFunc(s.Tasks, func(t api.Task) bool { return t.ID == id })
	s.rebuildTags()
}

// Clone deep-copies t so later edits to the store don't change it.
func Clone(t api.Task) api.Task {
	t.Tags = slices.Clone(t.Tags)
	t.Items = slices.Clone(t.Items)
	return t
}

// List ids: "inbox", "today", "tomorrow", "next7", "f-high", "f-nodate", "completed",
// "p:<projectId>", "tag:<name>".

func IsInbox(projectID string) bool { return strings.HasPrefix(projectID, "inbox") }

func Done(t *api.Task) bool { return t.Status != 0 }

// TasksFor returns the tasks (open and done) that belong to list id.
func (s *Store) TasksFor(id string, now time.Time) []*api.Task {
	var out []*api.Task
	for i := range s.Tasks {
		t := &s.Tasks[i]
		if s.in(id, t, now) {
			out = append(out, t)
		}
	}
	return out
}

func (s *Store) in(id string, t *api.Task, now time.Time) bool {
	day, hasDay := DayDiff(t, now)
	switch id {
	case "inbox":
		return IsInbox(t.ProjectID)
	case "today":
		return hasDay && day <= 0
	case "tomorrow":
		return hasDay && day == 1
	case "next7":
		return hasDay && day <= 6
	case "f-high":
		return t.Priority == 5 && hasDay && day <= 6
	case "f-nodate":
		return !hasDay
	case "completed":
		return Done(t)
	}
	if p, ok := strings.CutPrefix(id, "p:"); ok {
		return t.ProjectID == p
	}
	if tag, ok := strings.CutPrefix(id, "tag:"); ok {
		return slices.ContainsFunc(t.Tags, func(x string) bool { return strings.EqualFold(x, tag) })
	}
	return false
}

// OpenCount is the number of open tasks in list id.
func (s *Store) OpenCount(id string, now time.Time) int {
	n := 0
	for _, t := range s.TasksFor(id, now) {
		if !Done(t) {
			n++
		}
	}
	return n
}

func (s *Store) Project(id string) *api.Project {
	for i := range s.Projects {
		if s.Projects[i].ID == id {
			return &s.Projects[i]
		}
	}
	return nil
}

func (s *Store) Group(id string) *api.Group {
	for i := range s.Groups {
		if s.Groups[i].ID == id {
			return &s.Groups[i]
		}
	}
	return nil
}

// ListPath is "Folder › List" for a task's list.
func (s *Store) ListPath(projectID string) string {
	if IsInbox(projectID) {
		return "Inbox"
	}
	p := s.Project(projectID)
	if p == nil {
		return "—"
	}
	if g := s.Group(p.GroupID); g != nil {
		return g.Name + " › " + p.Name
	}
	return p.Name
}

// Priority levels as used by the API.
const (
	PrioNone = 0
	PrioLow  = 1
	PrioMed  = 3
	PrioHigh = 5
)

// PrioName is "High" / "Medium" / "Low" / "None"; PrioRole is the theme role.
func PrioName(p int) string {
	switch p {
	case PrioHigh:
		return "High"
	case PrioMed:
		return "Medium"
	case PrioLow:
		return "Low"
	}
	return "None"
}

func PrioRole(p int) string {
	switch p {
	case PrioHigh:
		return "p_high"
	case PrioMed:
		return "p_med"
	case PrioLow:
		return "p_low"
	}
	return "p_none"
}

type Group struct {
	Label string // "High", ..., "Completed"
	Role  string // theme role for the header
	Done  bool
	Items []*api.Task
}

// Groups splits tasks into priority groups (High → None), then Completed if showCompleted.
// sortBy is "due", "title" or "created".
func Groups(tasks []*api.Task, sortBy string, showCompleted bool, now time.Time) []Group {
	var out []Group
	for _, p := range []int{PrioHigh, PrioMed, PrioLow, PrioNone} {
		g := Group{Label: PrioName(p), Role: PrioRole(p)}
		for _, t := range tasks {
			if !Done(t) && normPrio(t.Priority) == p {
				g.Items = append(g.Items, t)
			}
		}
		if len(g.Items) > 0 {
			sortTasks(g.Items, sortBy, now)
			out = append(out, g)
		}
	}
	if showCompleted {
		g := Group{Label: "Completed", Role: "ok", Done: true}
		for _, t := range tasks {
			if Done(t) {
				g.Items = append(g.Items, t)
			}
		}
		if len(g.Items) > 0 {
			slices.SortStableFunc(g.Items, func(a, b *api.Task) int { return strings.Compare(b.CompletedTime, a.CompletedTime) })
			out = append(out, g)
		}
	}
	return out
}

func normPrio(p int) int {
	switch p {
	case PrioHigh, PrioMed, PrioLow:
		return p
	}
	return PrioNone
}

func sortTasks(ts []*api.Task, by string, now time.Time) {
	slices.SortStableFunc(ts, func(a, b *api.Task) int {
		switch by {
		case "title":
			return cmp.Compare(strings.ToLower(a.Title), strings.ToLower(b.Title))
		case "created":
			return cmp.Compare(a.CreatedTime, b.CreatedTime)
		}
		// due: day (no date last), then time of day (all-day last), then created
		da, oka := DayDiff(a, now)
		db, okb := DayDiff(b, now)
		if !oka {
			da = 1 << 30
		}
		if !okb {
			db = 1 << 30
		}
		return cmp.Or(cmp.Compare(da, db), cmp.Compare(clock(a), clock(b)), cmp.Compare(a.CreatedTime, b.CreatedTime))
	})
}

// clock is "HH:MM" for timed tasks and "99" for all-day ones, so all-day sorts last.
func clock(t *api.Task) string {
	if d, ok := DueTime(t); ok && !t.IsAllDay {
		return d.Format("15:04")
	}
	return "99"
}

// OpenTotal is the number of open tasks across all lists.
func (s *Store) OpenTotal() int {
	n := 0
	for i := range s.Tasks {
		if !Done(&s.Tasks[i]) {
			n++
		}
	}
	return n
}
