package store

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/raccoon-overlord-dev/ticktick-tui/internal/config"
)

// SnapshotPath is the cache of the last good sync, so startup can render at once.
func SnapshotPath() string { return filepath.Join(config.CacheDir(), "snapshot.json") }

// Save writes the snapshot (0600: it holds task data).
func (s *Store) Save() error {
	b, err := json.Marshal(s)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(config.CacheDir(), 0o700); err != nil {
		return err
	}
	tmp := SnapshotPath() + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, SnapshotPath())
}

// LoadSnapshot returns nil when there is no usable snapshot.
func LoadSnapshot() *Store {
	b, err := os.ReadFile(SnapshotPath())
	if err != nil {
		return nil
	}
	var s Store
	if json.Unmarshal(b, &s) != nil {
		return nil
	}
	return &s
}
