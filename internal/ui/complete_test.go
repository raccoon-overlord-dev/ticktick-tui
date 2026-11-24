package ui

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/raccoon-overlord-dev/ticktick-tui/internal/api"
)

// After a successful complete, the re-read only counts when it tells us something new: a
// repeating task's next occurrence. A read that says the task is still open (the API lags
// behind the complete), or that fails, must leave the local completed state alone.
func TestCompleteTask(t *testing.T) {
	for _, c := range []struct {
		name   string
		repeat string
		read   any    // nil: answer 500
		want   string // due date of the task returned, "" for none
	}{
		{"stale read", "", api.Task{ID: "a", ProjectID: "p", Status: 0, DueDate: "2026-10-02T07:00:00.000+0000"}, ""},
		{"read fails", "", nil, ""},
		{"completed", "", api.Task{ID: "a", ProjectID: "p", Status: 2}, "x"},
		{"repeating", "RRULE:FREQ=DAILY", api.Task{ID: "a", ProjectID: "p", Status: 0, DueDate: "2026-10-03T07:00:00.000+0000"}, "2026-10-03T07:00:00.000+0000"},
	} {
		t.Run(c.name, func(t *testing.T) {
			completed := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/project/p/task/a/complete" {
					completed++
					return
				}
				if c.read == nil {
					w.WriteHeader(http.StatusInternalServerError)
					return
				}
				json.NewEncoder(w).Encode(c.read)
			}))
			defer srv.Close()
			cl := api.New("fake-token")
			cl.BaseURL = srv.URL
			before := &api.Task{ID: "a", ProjectID: "p", Title: "Pay the rent", RepeatFlag: c.repeat}
			got, err := completeTask(context.Background(), cl, "p", "a", before)
			if err != nil || completed != 1 {
				t.Fatalf("err %v, %d completes", err, completed)
			}
			switch {
			case c.want == "" && got != nil:
				t.Errorf("kept the server's copy: %+v", got)
			case c.want != "" && got == nil:
				t.Error("dropped the server's copy")
			case c.want != "" && c.want != "x" && got.DueDate != c.want:
				t.Errorf("due = %q, want %q", got.DueDate, c.want)
			}
		})
	}
}
