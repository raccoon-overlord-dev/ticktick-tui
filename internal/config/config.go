// Package config loads and saves ~/.config/ttui/config.toml and resolves ttui's XDG paths.
package config

import (
	_ "embed"
	"errors"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

//go:embed default.toml
var defaultTOML string

type Config struct {
	Appearance struct {
		Theme           string `toml:"theme"`
		Background      string `toml:"background"`
		PriorityHeaders string `toml:"priority_headers"`
		FocusedPanel    string `toml:"focused_panel"`
		NerdFontIcons   bool   `toml:"nerd_font_icons"`
	} `toml:"appearance"`
	Layout struct {
		Columns       string   `toml:"columns"`
		ShowCompleted bool     `toml:"show_completed"`
		OpenFolders   []string `toml:"open_folders"` // folder (group) ids shown expanded in Lists
		SmartHidden   []string `toml:"smart_hidden"` // list ids left out of smart lists and filters (H); the API doesn't expose TickTick's own setting
	} `toml:"layout"`
	Tasks struct {
		DueLabel      bool            `toml:"due_label"`
		WeekStart     string          `toml:"week_start"`
		DueMenu       []string        `toml:"due_menu"`
		CompletedDays string          `toml:"completed_days"` // completed tasks downloaded: "7" | "30" | "90" | "365"
		SmartDates    bool            `toml:"smart_dates"`    // quick add reads day and time words as the due date
		Sort                          // default for lists without their own
		ListSort      map[string]Sort `toml:"list_sort,omitempty"` // per list, set with s; keyed by list ("inbox", "today", "p:<id>", "tag:<name>")
	} `toml:"tasks"`
	Keys struct {
		Keymap string `toml:"keymap"`
	} `toml:"keys"`
	Account struct {
		Server      string `toml:"server"`
		SyncEvery   string `toml:"sync_every"`
		UpdateCheck bool   `toml:"update_check"`
	} `toml:"account"`
}

// Sort is how a task list is grouped and ordered, as in the web app's sort menu.
type Sort struct {
	GroupBy string `toml:"group_by"` // list | date | created | tag | priority | none
	SortBy  string `toml:"sort_by"`  // date | created | modified | title | tag | priority
	Order   string `toml:"order"`    // oldest | newest
}

// Dir is $XDG_CONFIG_HOME/ttui, or ~/.config/ttui (macOS included).
func Dir() string { return xdgDir("XDG_CONFIG_HOME", ".config") }

// CacheDir is $XDG_CACHE_HOME/ttui, or ~/.cache/ttui.
func CacheDir() string { return xdgDir("XDG_CACHE_HOME", ".cache") }

func xdgDir(env, fallback string) string {
	if d := os.Getenv(env); d != "" {
		return filepath.Join(d, "ttui")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, fallback, "ttui")
}

// Path is the config file; --config overrides it.
var Path = filepath.Join(Dir(), "config.toml")

func Default() *Config {
	var c Config
	if _, err := toml.Decode(defaultTOML, &c); err != nil {
		panic("config: bad embedded default.toml: " + err.Error())
	}
	return &c
}

// Load reads Path over the defaults. On first run it writes the commented default file.
func Load() (*Config, error) {
	c := Default()
	_, err := toml.DecodeFile(Path, c)
	if errors.Is(err, os.ErrNotExist) {
		if err := os.MkdirAll(filepath.Dir(Path), 0o700); err != nil {
			return c, err
		}
		return c, os.WriteFile(Path, []byte(defaultTOML), 0o644)
	}
	return c, err
}

// Save overwrites Path (comments from the default file are not kept).
func Save(c *Config) error {
	f, err := os.Create(Path)
	if err != nil {
		return err
	}
	if err := toml.NewEncoder(f).Encode(c); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}
