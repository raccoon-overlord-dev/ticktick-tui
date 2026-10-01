package config

import (
	"path/filepath"
	"testing"
)

func TestSortRoundTrip(t *testing.T) {
	c := Default()
	if c.Tasks.GroupBy != "priority" || c.Tasks.SortBy != "date" || c.Tasks.Order != "oldest" {
		t.Fatalf("defaults: %+v", c.Tasks.Sort)
	}
	Path = filepath.Join(t.TempDir(), "c.toml")
	c.Tasks.ListSort = map[string]Sort{"p:x": {GroupBy: "tag", SortBy: "title", Order: "newest"}}
	if err := Save(c); err != nil {
		t.Fatal(err)
	}
	got, err := Load()
	if err != nil || got.Tasks.ListSort["p:x"].GroupBy != "tag" || got.Tasks.GroupBy != "priority" {
		t.Fatalf("round trip: %v %+v", err, got.Tasks)
	}
}
