// Package i18n translates ttui's interface. Keys are the English strings themselves, so English
// is complete by construction and a missing translation falls back to it (see i18n_test.go, which
// fails when a T("…") in the code has no entry in the table).
package i18n

import (
	"fmt"
	"strings"
)

// Lang is the language ttui shows ("en" or "it"), set from config.toml by the UI
// (Settings → Language, (*App).applyLocale).
var Lang = "en"

// T translates an English interface string.
func T(s string) string {
	if Lang != "en" {
		if v, ok := tables[Lang][s]; ok {
			return v
		}
	}
	return s
}

// Plural picks the form for n and fills in its %d: Plural(2, "1 day", "%d days") → "2 giorni".
func Plural(n int, one, many string) string {
	if n == 1 {
		return T(one)
	}
	return fmt.Sprintf(T(many), n)
}

// Fmt is T on a format string: Fmt("%d open", 3).
func Fmt(format string, args ...any) string { return fmt.Sprintf(T(format), args...) }

// Date translates the English day and month names time.Format writes ("Mon", "Jan"); the input
// is only ever a formatted date, so replacing the names in place is safe.
func Date(s string) string {
	if r := dateRepl[Lang]; r != nil {
		return r.Replace(s)
	}
	return s
}

var tables = map[string]map[string]string{"it": it}

var dateRepl = map[string]*strings.Replacer{"it": strings.NewReplacer(itDates...)}

// Has reports whether lang translates s. Tests use it for strings declared in a table, which
// reach T through a variable and so are invisible to the completeness scanner.
func Has(lang, s string) bool {
	_, ok := tables[lang][s]
	return ok
}
