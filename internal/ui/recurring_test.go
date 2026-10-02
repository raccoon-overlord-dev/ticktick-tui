package ui

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"ttui/internal/api"
)

// Deleting one occurrence completes the task, then deletes the completed copy it leaves;
// on the last occurrence it deletes the task itself.
func TestSkipOccurrence(t *testing.T) {
	before := &api.Task{ID: "r", ProjectID: "p", Title: "Water plants", DueDate: "2026-10-02T07:00:00+0000", RepeatFlag: "RRULE:FREQ=DAILY"}
	for _, last := range []bool{false, true} {
		var deleted []string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var out any
			switch {
			case r.Method == http.MethodDelete:
				deleted = append(deleted, r.URL.Path)
			case r.URL.Path == "/project/p/task/r":
				next := api.Task{ID: "r", ProjectID: "p", DueDate: "2026-10-03T07:00:00.000+0000"}
				if last {
					next.Status = 2
				}
				out = next
			case r.URL.Path == "/task/completed":
				out = []api.Task{
					{ID: "other", ProjectID: "p", Title: "Water plants", DueDate: "2026-10-01T07:00:00.000+0000"},
					{ID: "copy", ProjectID: "p", Title: "Water plants", DueDate: "2026-10-02T07:00:00.000+0000"},
				}
			}
			json.NewEncoder(w).Encode(out)
		}))
		c := api.New("fake-token")
		c.BaseURL = srv.URL
		got, err := skipOccurrence(context.Background(), c, "p", "r", before)
		srv.Close()
		want := "/project/p/task/copy"
		if last {
			want = "/project/p/task/r"
		}
		if err != nil || len(deleted) != 1 || deleted[0] != want || (got == nil) != last {
			t.Fatalf("last=%v: err %v, deleted %v, task %v", last, err, deleted, got)
		}
	}
}
