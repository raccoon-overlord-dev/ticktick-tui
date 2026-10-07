package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/raccoon-overlord-dev/ticktick-tui/internal/api"
	"github.com/raccoon-overlord-dev/ticktick-tui/internal/config"
)

// Trashed is a task deleted from ttui. The Open API has no Trash (see docs/api-notes.md),
// so ttui keeps its own copy; restoring creates the task again.
type Trashed struct {
	Task    api.Task  `json:"task"`
	Deleted time.Time `json:"deleted"`
}

// TrashDays is how long a deleted task stays in the trash.
const TrashDays = 30

// PruneTrash returns ts without the tasks deleted more than TrashDays ago.
func PruneTrash(ts []Trashed, now time.Time) []Trashed {
	cut := now.AddDate(0, 0, -TrashDays)
	return slices.DeleteFunc(slices.Clone(ts), func(t Trashed) bool { return t.Deleted.Before(cut) })
}

func TrashPath() string { return filepath.Join(config.CacheDir(), "trash.json") }

// LoadTrash returns the trash, newest first, without expired tasks; nil when there is none.
func LoadTrash() []Trashed {
	b, err := os.ReadFile(TrashPath())
	if err != nil {
		return nil
	}
	var ts []Trashed
	if json.Unmarshal(b, &ts) != nil {
		return nil
	}
	return PruneTrash(ts, time.Now())
}

// SaveTrash writes the trash (0600: it holds task data).
func SaveTrash(ts []Trashed) error {
	b, err := json.Marshal(ts)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(config.CacheDir(), 0o700); err != nil {
		return err
	}
	tmp := TrashPath() + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, TrashPath())
}
