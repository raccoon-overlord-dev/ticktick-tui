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
		Columns       string `toml:"columns"`
		ShowCompleted bool   `toml:"show_completed"`
	} `toml:"layout"`
	Tasks struct {
		DueLabel       bool   `toml:"due_label"`
		SortInPriority string `toml:"sort_in_priority"`
		WeekStart      string `toml:"week_start"`
	} `toml:"tasks"`
	Keys struct {
		Keymap string `toml:"keymap"`
	} `toml:"keys"`
	Account struct {
		Server    string `toml:"server"`
		SyncEvery string `toml:"sync_every"`
	} `toml:"account"`
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
