package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRetry(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls < 3 {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(`{"errorCode":"exceed_query_limit"}`))
			return
		}
		w.Write([]byte(`[]`))
	}))
	defer srv.Close()
	old := Backoff
	Backoff = []time.Duration{time.Millisecond, time.Millisecond, time.Millisecond}
	defer func() { Backoff = old }()

	c := New("fake-token")
	c.BaseURL = srv.URL
	if _, err := c.Projects(context.Background()); err != nil || calls != 3 {
		t.Fatalf("err=%v calls=%d", err, calls)
	}
}

func TestClient(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer fake-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch r.URL.Path {
		case "/project":
			w.Write([]byte(`[{"id":"p1","name":"Work","groupId":"g1"}]`))
		case "/project/inbox/data":
			w.Write([]byte(`{"tasks":[{"id":"t1","projectId":"inbox123","title":"Buy milk","priority":5}],"columns":[]}`))
		default:
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(`{"errorCode":"boom"}`))
		}
	}))
	defer srv.Close()
	ctx := context.Background()

	c := New("fake-token")
	c.BaseURL = srv.URL

	ps, err := c.Projects(ctx)
	if err != nil || len(ps) != 1 || ps[0].GroupID != "g1" {
		t.Fatalf("Projects = %+v, %v", ps, err)
	}
	d, err := c.ProjectData(ctx, "inbox")
	if err != nil || len(d.Tasks) != 1 || d.Tasks[0].Priority != 5 {
		t.Fatalf("ProjectData = %+v, %v", d, err)
	}

	var apiErr *Error
	if err := c.Do(ctx, "GET", "/nope", nil, nil); !errors.As(err, &apiErr) || apiErr.Status != 500 {
		t.Fatalf("want *Error 500, got %v", err)
	}
	c.Token = "wrong"
	if _, err := c.Projects(ctx); !errors.As(err, &apiErr) || apiErr.Status != 401 {
		t.Fatalf("want *Error 401, got %v", err)
	}
}

func TestNotes(t *testing.T) {
	var tasks []Task
	json.Unmarshal([]byte(`[{"kind":"CHECKLIST","content":"","desc":"d"},{"kind":"TEXT","content":"c"}]`), &tasks)
	if k, n := tasks[0].Notes(); k != "desc" || *n != "d" {
		t.Errorf("checklist: got %s %q", k, *n)
	}
	if k, n := tasks[1].Notes(); k != "content" || *n != "c" {
		t.Errorf("text: got %s %q", k, *n)
	}
}
