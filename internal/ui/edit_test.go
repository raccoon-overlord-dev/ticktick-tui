package ui

import (
	"errors"
	"testing"
	"time"

	"ttui/internal/api"
	"ttui/internal/auth"
	"ttui/internal/config"
	"ttui/internal/parse"
	"ttui/internal/store"
)

func testApp(t *testing.T) *App {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	st := store.New(nil, nil, []api.Task{{ID: "a", ProjectID: "inbox1", Title: "Task A", Priority: 5}}, "", time.Now())
	return &App{cfg: config.Default(), st: st, signed: &auth.Auth{AccessToken: "fake"}, list: "inbox",
		folded: map[string]bool{}, idMap: map[string]string{}}
}

func TestOptimisticRevert(t *testing.T) {
	a := testApp(t)
	a.cyclePrio(a.st.Task("a")) // high → medium, queued and "in flight"
	if a.st.Task("a").Priority != store.PrioMed || !a.busy {
		t.Fatalf("not applied optimistically: %+v busy=%v", a.st.Task("a"), a.busy)
	}
	a.onOpDone(opDoneMsg{op: a.queue[0], err: errors.New("boom")})
	if a.st.Task("a").Priority != store.PrioHigh || a.flashRole != "error" {
		t.Fatalf("not reverted: %+v flash=%q", a.st.Task("a"), a.flash)
	}
}

func TestCreateReplacesTempID(t *testing.T) {
	a := testApp(t)
	a.createTask(parse.Add{Title: "New", List: "inbox"})
	tmp := a.taskID
	if a.st.Task(tmp) == nil || a.queue[0].fields["projectId"] != nil {
		t.Fatalf("temp task missing or inbox projectId sent: %+v", a.queue[0].fields)
	}
	// An edit made before the create returns is queued against the temp id.
	a.cyclePrio(a.st.Task(tmp))
	a.onOpDone(opDoneMsg{op: a.queue[0], t: &api.Task{ID: "real", ProjectID: "inbox1", Title: "New"}})
	if a.taskID != "real" || a.st.Task(tmp) != nil || a.idMap[tmp] != "real" || a.st.Task("real") == nil {
		t.Fatalf("temp id not replaced: sel=%s idMap=%v", a.taskID, a.idMap)
	}
	if !a.busy || a.queue[0].taskID != tmp { // the queued edit is sent next, resolved via idMap
		t.Fatalf("queued edit lost: %+v", a.queue)
	}

	b := testApp(t)
	b.createTask(parse.Add{Title: "Fails", List: "inbox"})
	tmp = b.taskID
	b.onOpDone(opDoneMsg{op: b.queue[0], err: errors.New("boom")})
	if b.st.Task(tmp) != nil {
		t.Fatal("failed create not removed")
	}
}
