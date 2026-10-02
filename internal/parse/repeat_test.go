package parse

import "testing"

func TestParseRepeat(t *testing.T) {
	for in, want := range map[string]string{
		"daily":                        "RRULE:FREQ=DAILY;INTERVAL=1",
		"every 3 days":                 "RRULE:FREQ=DAILY;INTERVAL=3",
		"every 2 weeks on mon, fri":    "RRULE:FREQ=WEEKLY;INTERVAL=2;BYDAY=MO,FR",
		"mon,wed,fri":                  "RRULE:FREQ=WEEKLY;INTERVAL=1;BYDAY=MO,WE,FR",
		"weekdays":                     "RRULE:FREQ=WEEKLY;INTERVAL=1;BYDAY=MO,TU,WE,TH,FR",
		"every 23rd":                   "RRULE:FREQ=MONTHLY;INTERVAL=1;BYMONTHDAY=23",
		"monthly 23":                   "RRULE:FREQ=MONTHLY;INTERVAL=1;BYMONTHDAY=23",
		"last day of the month":        "RRULE:FREQ=MONTHLY;INTERVAL=1;BYMONTHDAY=-1",
		"first monday":                 "RRULE:FREQ=MONTHLY;INTERVAL=1;BYDAY=1MO",
		"every 3rd wed":                "RRULE:FREQ=MONTHLY;INTERVAL=1;BYDAY=3WE",
		"last fri":                     "RRULE:FREQ=MONTHLY;INTERVAL=1;BYDAY=-1FR",
		"last workday":                 "RRULE:FREQ=MONTHLY;INTERVAL=1;BYMONTHDAY=-1;TT_WORKDAY=-1",
		"first workday":                "RRULE:FREQ=MONTHLY;INTERVAL=1;BYMONTHDAY=1;TT_WORKDAY=1",
		"yearly x5":                    "RRULE:FREQ=YEARLY;INTERVAL=1;COUNT=5",
		"daily 10 times":               "RRULE:FREQ=DAILY;INTERVAL=1;COUNT=10",
		"daily until 2026-12-31":       "RRULE:FREQ=DAILY;INTERVAL=1;UNTIL=20261231",
		"daily skip weekends holidays": "RRULE:FREQ=DAILY;INTERVAL=1;TT_SKIP=HOLIDAY,WEEKEND",
		"curve":                        "ERULE:NAME=FORGETTINGCURVE;CYCLE=0",
		"dates fri, 2026-10-22":        "ERULE:NAME=CUSTOM;BYDATE=20261002,20261022",
	} {
		got, _, err := ParseRepeat(in, now)
		if err != nil || got != want {
			t.Errorf("ParseRepeat(%q) = %q, %v; want %q", in, got, err, want)
		}
		// what RepeatText shows must read back to the same rule
		if back, _, err := ParseRepeat(RepeatText(got, false), now); err != nil || back != got {
			t.Errorf("round trip %q: RepeatText %q → %q, %v", got, RepeatText(got, false), back, err)
		}
	}
	if _, from, _ := ParseRepeat("every week from completion", now); !from {
		t.Error("from completion not read")
	}
	for _, in := range []string{"", "sometimes", "daily weekly", "2nd workday", "daily x2 until fri", "every 0 days", "dates"} {
		if r, _, err := ParseRepeat(in, now); err == nil {
			t.Errorf("ParseRepeat(%q) = %q, want an error", in, r)
		}
	}
	if got := RepeatText("RRULE:FREQ=MONTHLY;INTERVAL=1;BYDAY=1MO", true); got != "every month 1st mon from completion" {
		t.Errorf("RepeatText = %q", got)
	}
}

func TestOrdinal(t *testing.T) {
	for n, want := range map[int]string{1: "1st", 2: "2nd", 3: "3rd", 4: "4th", 11: "11th", 12: "12th", 13: "13th", 21: "21st", 22: "22nd", 23: "23rd", 31: "31st"} {
		if got := Ordinal(n); got != want {
			t.Errorf("Ordinal(%d) = %q, want %q", n, got, want)
		}
	}
}
