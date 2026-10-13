// Package theme loads color themes: built-in (embedded), the terminalcolors.com palettes
// (palettes/<name>-dark.toml and -light.toml, picked by light/dark mode) or
// ~/.config/ttui/themes/<name>.toml.
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

//go:embed themes/*.toml palettes/*.toml
var builtin embed.FS

// Theme maps role names (base, text, accent, p_high, ...) to colors.
// A value is "#rrggbb", an ANSI index "0"-"255", or "" for the terminal default.
type Theme struct {
	Name      string            `toml:"name"`
	Family    string            `toml:"family"`  // palettes: "Catppuccin"
	Variant   string            `toml:"variant"` // palettes: "Mocha"
	Mode      string            `toml:"mode"`    // palettes: dark | light
	Separator string            `toml:"separator"`
	Colors    map[string]string `toml:"colors"`
}

// C returns the color for role; unknown or empty roles give the terminal default.
func (t *Theme) C(role string) color.Color { return lipgloss.Color(t.Colors[role]) }

// Names lists the custom themes (not the palettes): terminal, the other built-in ones, then
// the user's own files that don't replace a built-in one.
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

// Load prefers a user theme file, then a built-in one, then the palette name in dark or
// light (or its other mode, for a palette that has only one).
func Load(name string, dark bool) (*Theme, error) {
	mode := map[bool][2]string{true: {"dark", "light"}, false: {"light", "dark"}}[dark]
	data, err := os.ReadFile(filepath.Join(config.Dir(), "themes", name+".toml"))
	for _, f := range []string{"themes/" + name, "palettes/" + name + "-" + mode[0], "palettes/" + name + "-" + mode[1]} {
		if err != nil {
			data, err = builtin.ReadFile(f + ".toml")
		}
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

// Palettes are the terminalcolors.com themes that have a dark (or light) variant, by family.
func Palettes(dark bool) []*Theme {
	mode := map[bool]string{true: "-dark.toml", false: "-light.toml"}[dark]
	es, _ := builtin.ReadDir("palettes")
	var out []*Theme
	for _, e := range es {
		if n, ok := strings.CutSuffix(e.Name(), mode); ok {
			if t, err := Load(n, dark); err == nil {
				out = append(out, t)
			}
		}
	}
	slices.SortFunc(out, func(a, b *Theme) int { return strings.Compare(strings.ToLower(a.Family), strings.ToLower(b.Family)) })
	return out
}

// Label is the name shown in Settings: "Catppuccin · Mocha", "Nord", or the file name.
func (t *Theme) Label() string {
	switch {
	case t.Family == "":
		return t.Name
	case t.Variant == "":
		return t.Family
	}
	return t.Family + " · " + t.Variant
}
