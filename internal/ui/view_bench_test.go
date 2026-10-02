package ui

import (
	"fmt"
	"testing"
	"time"

	"ttui/internal/api"
	"ttui/internal/auth"
	"ttui/internal/config"
	"ttui/internal/store"
	"ttui/internal/theme"
)

// BenchmarkView renders a realistic main screen (all-day tasks with a time zone, as the API sends them).
// Run: go test ./internal/ui -run x -bench View
func BenchmarkView(b *testing.B) {
	var ps []api.Project
	for i := range 10 {
		ps = append(ps, api.Project{ID: fmt.Sprint("p", i), Name: fmt.Sprint("List ", i)})
	}
	var ts []api.Task
	for i := range 200 {
		t := api.Task{ID: fmt.Sprint(i), ProjectID: fmt.Sprint("p", i%10), Title: fmt.Sprint("Task number ", i), Priority: []int{0, 1, 3, 5}[i%4], Tags: []string{fmt.Sprint("t", i%5)}}
		if i%2 == 0 {
			t.DueDate, t.IsAllDay, t.TimeZone = time.Now().AddDate(0, 0, i%9-2).UTC().Format(api.DateLayout), true, "Europe/Rome"
		}
		ts = append(ts, t)
	}
	th, _ := theme.Load("terminal")
	a := &App{cfg: config.Default(), th: th, st: store.New(ps, nil, ts, "", time.Now()), signed: &auth.Auth{AccessToken: "fake"},
		list: "today", focus: "tasks", sideKey: "l:today", screen: screenMain, idMap: map[string]string{}, w: 160, h: 45, now: time.Now()}
	b.ResetTimer()
	for range b.N {
		a.View()
	}
}
