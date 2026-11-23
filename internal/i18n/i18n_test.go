package i18n

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// callRe finds the translated literals: T("…"), i18n.T("…"), Fmt("…", …) and both forms of
// Plural(n, "…", "…").
var callRe = regexp.MustCompile(`(?:i18n\.)?(?:T|Fmt)\((` + "`" + `[^` + "`" + `]*` + "`" + `|"(?:[^"\\]|\\.)*")|(?:i18n\.)?Plural\([^,]+, ("(?:[^"\\]|\\.)*"), ("(?:[^"\\]|\\.)*")\)`)

// litRe finds every string literal, for the drift check: many translated strings are
// declared in a table (setRows, helpKeys, the hint bars) and reach T through a variable.
var litRe = regexp.MustCompile("`[^`]*`" + `|"(?:[^"\\]|\\.)*"`)

// literals returns every string literal in the code, translated or not.
func literals(t *testing.T) map[string]bool {
	t.Helper()
	out := map[string]bool{}
	walk(t, func(path string, src []byte) {
		for _, lit := range litRe.FindAllString(string(src), -1) {
			if lit[0] == '"' {
				if v, err := strconv.Unquote(lit); err == nil {
					out[v] = true
				}
				continue
			}
			out[strings.Trim(lit, "`")] = true
		}
	})
	return out
}

// walk calls f for every non-test Go file outside this package.
func walk(t *testing.T, f func(path string, src []byte)) {
	t.Helper()
	root := filepath.Join("..", "..", "internal")
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return err
		}
		if filepath.Base(filepath.Dir(path)) == "i18n" { // this package's own examples
			return nil
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		f(path, src)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// used returns every English string the code asks i18n to translate.
func used(t *testing.T) map[string][]string {
	t.Helper()
	out := map[string][]string{}
	walk(t, func(path string, src []byte) {
		for _, m := range callRe.FindAllStringSubmatch(string(src), -1) {
			for _, lit := range m[1:] {
				if lit == "" {
					continue
				}
				s := lit
				if lit[0] == '"' {
					if v, err := strconv.Unquote(lit); err == nil {
						s = v
					}
				} else {
					s = strings.Trim(lit, "`")
				}
				out[s] = append(out[s], path)
			}
		}
	})
	if len(out) == 0 {
		t.Fatal("no T(…) calls found: the scanner is broken, not the code")
	}
	return out
}

// Every translated string must have an Italian entry, and every entry must still be used.
func TestItalianIsComplete(t *testing.T) {
	u := used(t)
	for s, where := range u {
		v, ok := it[s]
		switch {
		case !ok:
			t.Errorf("missing Italian for %q (%s)", s, where[0])
		case strings.TrimSpace(v) == "":
			t.Errorf("empty Italian for %q", s)
		}
	}
	lits := literals(t)
	for s := range it {
		if !lits[s] {
			t.Errorf("stale entry %q: that string is nowhere in the code", s)
		}
	}
}

// A translated format string must keep its verbs, or Sprintf writes %!d(MISSING) on screen.
func TestItalianKeepsFormatVerbs(t *testing.T) {
	verb := regexp.MustCompile(`%[-+ #0-9.]*[a-zA-Z]`)
	for en, loc := range it {
		a, b := verb.FindAllString(en, -1), verb.FindAllString(loc, -1)
		if strings.Join(a, "") != strings.Join(b, "") {
			t.Errorf("%q: verbs %v become %v in %q", en, a, b, loc)
		}
	}
}

func TestFallsBackToEnglish(t *testing.T) {
	t.Cleanup(func() { Lang = "en" })
	Lang = "it"
	if got := T("no translation for this one"); got != "no translation for this one" {
		t.Errorf("got %q", got)
	}
	if got := Date("Mon 8 Oct"); got != "lun 8 ott" {
		t.Errorf("Date: %q", got)
	}
	Lang = "en"
	if got := Date("Mon 8 Oct"); got != "Mon 8 Oct" {
		t.Errorf("Date en: %q", got)
	}
}
