// Package theme loads color themes: built-in (embedded) or ~/.config/ttui/themes/<name>.toml.
package theme

import (
	"embed"
	"fmt"
	"image/color"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/BurntSushi/toml"

	"github.com/raccoon-overlord-dev/ticktick-tui/internal/config"
)

//go:embed themes/*.toml
var builtin embed.FS

// Theme maps role names (base, text, accent, p_high, ...) to colors.
// A value is "#rrggbb", an ANSI index "0"-"255", or "" for the terminal default.
type Theme struct {
	Name      string            `toml:"name"`
	Separator string            `toml:"separator"`
	Colors    map[string]string `toml:"colors"`
}

// C returns the color for role; unknown or empty roles give the terminal default.
func (t *Theme) C(role string) color.Color { return lipgloss.Color(t.Colors[role]) }

// Names lists the themes: terminal, the other built-in ones, then the user's own files
// that don't replace a built-in one.
func Names() []string {
	tomls := func(es []os.DirEntry) (out []string) {
		for _, e := range es {
			if n, ok := strings.CutSuffix(e.Name(), ".toml"); ok && n != "terminal" {
				out = append(out, n)
			}
		}
		slices.Sort(out)
		return out
	}
	built, _ := builtin.ReadDir("themes")
	user, _ := os.ReadDir(filepath.Join(config.Dir(), "themes"))
	names := append([]string{"terminal"}, tomls(built)...)
	for _, n := range tomls(user) {
		if !slices.Contains(names, n) {
			names = append(names, n)
		}
	}
	return names
}

// Load prefers a user theme file, then a built-in one.
func Load(name string) (*Theme, error) {
	data, err := os.ReadFile(filepath.Join(config.Dir(), "themes", name+".toml"))
	if err != nil {
		data, err = builtin.ReadFile("themes/" + name + ".toml")
	}
	if err != nil {
		return nil, fmt.Errorf("theme %q not found", name)
	}
	var t Theme
	if _, err := toml.Decode(string(data), &t); err != nil {
		return nil, fmt.Errorf("theme %q: %w", name, err)
	}
	if t.Separator == "" {
		t.Separator = "·"
	}
	return &t, nil
}
