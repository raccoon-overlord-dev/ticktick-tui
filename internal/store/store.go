// Package store holds the in-memory TickTick data and the derived views (smart lists, groups, due labels).
package store

import (
	"cmp"
	"context"
	"errors"
	"slices"
	"strings"
	"sync"
	"time"

	"ttui/internal/api"
	"ttui/internal/config"
)

type Store struct {
	Projects []api.Project // sorted by sortOrder, closed ones dropped
	Groups   []api.Group   // folders, sorted by sortOrder
	Tasks    []api.Task    // open tasks + tasks completed in the last CompletedDays
	Tags     []string      // derived from tasks, sorted
	Email    string
	SyncedAt time.Time
	// SmartHidden lists (project ids) are left out of the date smart lists and filters, like
	// TickTick's "Show in smart list: Do not show". Set by the UI from config: the API doesn't expose it.
	SmartHidden []string `json:"-"`
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
	if err == nil && len(open) >= api.FilterCap { // cut short: ask each list instead
		open, err = openByList(ctx, c, ps)
	}
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

// openByList fetches the open tasks of the Inbox and every open list, a few at a time.
func openByList(ctx context.Context, c *api.Client, ps []api.Project) ([]api.Task, error) {
	ids := []string{"inbox"}
	for _, p := range ps {
		if !p.Closed {
			ids = append(ids, p.ID)
		}
	}
	res := make([][]api.Task, len(ids))
	errs := make([]error, len(ids))
	sem := make(chan struct{}, 6)
	var wg sync.WaitGroup
	for i, id := range ids {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			d, err := c.ProjectData(ctx, id)
			if err == nil {
				res[i] = d.Tasks
			}
			errs[i] = err
		}()
	}
	wg.Wait()
	if err := errors.Join(errs...); err != nil {
		return nil, err
	}
	return slices.Concat(res...), nil
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

// InboxID is the Inbox's real project id ("inbox<user id>"), read from a task in it;
// the API has no other way to get it. Empty if no synced task is in the Inbox.
func (s *Store) InboxID() string {
	for _, t := range s.Tasks {
		if IsInbox(t.ProjectID) && t.ProjectID != "inbox" { // "inbox" is the placeholder of a task still being created
			return t.ProjectID
		}
	}
	return ""
}

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
	case "today", "tomorrow", "next7", "f-high", "f-nodate":
		if slices.Contains(s.SmartHidden, t.ProjectID) {
			return false
		}
	}
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
	rank  int
}

// TaskGroups splits tasks by so.GroupBy (list, date, created, tag, priority, none), sorts each
// group by so.SortBy and so.Order, then adds Completed (newest first) if showCompleted.
func (s *Store) TaskGroups(tasks []*api.Task, so config.Sort, showCompleted bool, now time.Time) []Group {
	var out []Group
	byKey := map[string]int{}
	for _, t := range tasks {
		if Done(t) {
			continue
		}
		label, role, rank := s.groupOf(t, so.GroupBy, now)
		i, ok := byKey[label]
		if !ok {
			i = len(out)
			byKey[label] = i
			out = append(out, Group{Label: label, Role: role, rank: rank})
		}
		out[i].Items = append(out[i].Items, t)
	}
	slices.SortStableFunc(out, func(a, b Group) int { return cmp.Or(cmp.Compare(a.rank, b.rank), cmp.Compare(a.Label, b.Label)) })
	for _, g := range out {
		sortTasks(g.Items, so, now)
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

// groupOf returns t's group label, header role and group rank (lower comes first; equal ranks sort by label).
func (s *Store) groupOf(t *api.Task, by string, now time.Time) (string, string, int) {
	switch by {
	case "list":
		if IsInbox(t.ProjectID) {
			return "Inbox", "accent", -1
		}
		rank := slices.IndexFunc(s.Projects, func(p api.Project) bool { return p.ID == t.ProjectID })
		if rank < 0 {
			rank = len(s.Projects)
		}
		return s.ListPath(t.ProjectID), "accent", rank
	case "date":
		d, ok := DayDiff(t, now)
		switch {
		case !ok:
			return "No date", "dim", 5
		case d < 0:
			return "Overdue", "error", 0
		case d == 0:
			return "Today", "accent", 1
		case d == 1:
			return "Tomorrow", "accent", 2
		case d <= 7:
			return "Next 7 days", "accent", 3
		}
		return "Later", "accent", 4
	case "created":
		c, err := time.Parse(api.DateLayout, t.CreatedTime)
		switch d := days(c.In(now.Location()), now); {
		case err != nil:
			return "Unknown", "dim", 4
		case d <= 0:
			return "Today", "accent", 0
		case d == 1:
			return "Yesterday", "accent", 1
		case d <= 7:
			return "Last 7 days", "accent", 2
		}
		return "Earlier", "accent", 3
	case "tag":
		// first tag only, so each task shows once
		if len(t.Tags) == 0 {
			return "No tag", "dim", 1
		}
		return "#" + strings.ToLower(t.Tags[0]), "secondary", 0
	case "none":
		return "", "", 0
	}
	p := normPrio(t.Priority)
	return PrioName(p), PrioRole(p), -p
}

func normPrio(p int) int {
	switch p {
	case PrioHigh, PrioMed, PrioLow:
		return p
	}
	return PrioNone
}

// sortTasks sorts by so.SortBy; Order "newest" reverses it. Tasks missing the key (no date,
// no tag) stay last either way. Ties fall back to due date, then created time.
func sortTasks(ts []*api.Task, so config.Sort, now time.Time) {
	dir := 1
	if so.Order == "newest" {
		dir = -1
	}
	due := func(t *api.Task) (int, bool) { d, ok := DayDiff(t, now); return d, ok }
	byDue := func(a, b *api.Task) int {
		da, _ := due(a)
		db, _ := due(b)
		return cmp.Or(cmp.Compare(da, db), cmp.Compare(clock(a), clock(b)))
	}
	// missing compares presence: tasks without the key go last regardless of dir
	missing := func(okA, okB bool) int {
		switch {
		case okA == okB:
			return 0
		case okA:
			return -1
		}
		return 1
	}
	slices.SortStableFunc(ts, func(a, b *api.Task) int {
		var c int
		switch so.SortBy {
		case "title":
			c = dir * cmp.Compare(strings.ToLower(a.Title), strings.ToLower(b.Title))
		case "created":
			c = dir * cmp.Compare(a.CreatedTime, b.CreatedTime)
		case "modified":
			c = dir * cmp.Compare(a.ModifiedTime, b.ModifiedTime)
		case "priority":
			c = dir * cmp.Compare(normPrio(b.Priority), normPrio(a.Priority))
		case "tag":
			if c = missing(len(a.Tags) > 0, len(b.Tags) > 0); c == 0 && len(a.Tags) > 0 {
				c = dir * cmp.Compare(strings.ToLower(a.Tags[0]), strings.ToLower(b.Tags[0]))
			}
		default: // date: day, then time of day (all-day last)
			_, oka := due(a)
			_, okb := due(b)
			if c = missing(oka, okb); c == 0 {
				c = dir * byDue(a, b)
			}
		}
		if c != 0 {
			return c
		}
		_, oka := due(a)
		_, okb := due(b)
		return cmp.Or(missing(oka, okb), byDue(a, b), cmp.Compare(a.CreatedTime, b.CreatedTime))
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
