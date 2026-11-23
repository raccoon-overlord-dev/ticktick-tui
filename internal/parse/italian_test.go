package parse

import (
	"testing"
	"time"

	"github.com/raccoon-overlord-dev/ticktick-tui/internal/i18n"
)

// Italian input is accepted whatever the language setting, so a task typed in either
// language keeps working after a switch.
func TestItalianDays(t *testing.T) {
	now := time.Date(2026, 11, 23, 9, 0, 0, 0, time.Local) // a Monday
	for _, c := range []struct{ word, same string }{
		{"oggi", "today"}, {"domani", "tomorrow"}, {"ieri", "yesterday"},
		{"ven", "fri"}, {"venerdì", "friday"}, {"venerdi", "friday"}, {"dom", "sun"},
	} {
		got, ok := ParseDay(c.word, now)
		want, okEn := ParseDay(c.same, now)
		if !ok || !okEn || got != want {
			t.Errorf("%q: %d %v, want %q = %d", c.word, got, ok, c.same, want)
		}
	}
}

func TestItalianRepeat(t *testing.T) {
	now := time.Date(2026, 11, 23, 9, 0, 0, 0, time.Local)
	for _, c := range []struct{ it, en string }{
		{"ogni 2 settimane", "every 2 weeks"},
		{"ogni lun,mer", "every mon,wed"},
		{"ogni giorno", "every day"},
		{"giorni feriali", "weekdays"},
		{"ultimo giorno lavorativo", "last workday"},
		{"primo lun", "first mon"},
		{"ogni anno ottobre 6", "every year october 6th"},
		{"ogni settimana fino al 2026-12-31", "every week until 2026-12-31"},
		{"ogni mese dal completamento", "every month from completion"},
		{"ogni settimana salta weekend", "every week skip weekends"},
	} {
		got, gotFrom, err := ParseRepeat(c.it, now)
		want, wantFrom, errEn := ParseRepeat(c.en, now)
		if err != nil || errEn != nil || got != want || gotFrom != wantFrom {
			t.Errorf("%q → %q (%v), want %q from %q (%v)", c.it, got, err, want, c.en, errEn)
		}
	}
}

// The Italian repeat text must be readable back by the parser: it is what the Custom repeat
// field shows and what the user edits.
func TestItalianRepeatTextRoundTrips(t *testing.T) {
	t.Cleanup(func() { i18n.Lang = "en" })
	now := time.Date(2026, 11, 23, 9, 0, 0, 0, time.Local)
	for _, en := range []string{
		"every 2 weeks", "every day", "weekdays", "last workday", "first workday",
		"every month last day", "3rd wed", "every week until 2026-12-31", "every month x5",
		"every month from completion", "every week skip weekends", "every year oct 6th",
	} {
		rule, from, err := ParseRepeat(en, now)
		if err != nil {
			t.Fatalf("%q: %v", en, err)
		}
		i18n.Lang = "it"
		text := RepeatText(rule, from)
		back, backFrom, err := ParseRepeat(text, now)
		i18n.Lang = "en"
		if err != nil || back != rule || backFrom != from {
			t.Errorf("%q → %q → %q (%v), want %q", en, text, back, err, rule)
		}
	}
}

func TestItalianReminderText(t *testing.T) {
	t.Cleanup(func() { i18n.Lang = "en" })
	for _, c := range []struct{ trigger, en, it string }{
		{"TRIGGER:PT0S", "On time", "In orario"},
		{"TRIGGER:-PT30M", "30 minutes early", "30 minuti prima"},
		{"TRIGGER:-P1D", "1 day early", "1 giorno prima"},
	} {
		if got := ReminderText(c.trigger, false); got != c.en {
			t.Errorf("en %q: %q", c.trigger, got)
		}
		i18n.Lang = "it"
		got := ReminderText(c.trigger, false)
		i18n.Lang = "en"
		if got != c.it {
			t.Errorf("it %q: %q, want %q", c.trigger, got, c.it)
		}
	}
}
