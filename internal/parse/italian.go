package parse

import (
	"strconv"
	"strings"
)

// Italian input. The parsers themselves stay English: every Italian word is mapped to the
// English one they already read, so both languages work whatever Settings → Language says.

// itPhrases runs on the whole string, before the words are split, for the terms that are
// two words in Italian and one in English.
var itPhrases = strings.NewReplacer(
	"ultimo giorno lavorativo", "last workday", "primo giorno lavorativo", "first workday",
	"giorni lavorativi", "workday", "giorno lavorativo", "workday",
	"giorni feriali", "weekdays", "ultimo giorno", "last day",
	"dalla data di completamento", "from completion", "dal completamento", "from completion",
	"fino al", "until", "fino a", "until",
)

var itWords = map[string]string{
	"oggi": "today", "domani": "tomorrow", "ieri": "yesterday",
	"domenica": "sunday", "lunedì": "monday", "lunedi": "monday", "martedì": "tuesday", "martedi": "tuesday",
	"mercoledì": "wednesday", "mercoledi": "wednesday", "giovedì": "thursday", "giovedi": "thursday",
	"venerdì": "friday", "venerdi": "friday", "sabato": "saturday",
	"dom": "sun", "lun": "mon", "mar": "tue", "mer": "wed", "gio": "thu", "ven": "fri", "sab": "sat",
	"gennaio": "january", "febbraio": "february", "marzo": "march", "aprile": "april",
	"maggio": "may", "giugno": "june", "luglio": "july", "agosto": "august",
	"settembre": "september", "ottobre": "october", "novembre": "november", "dicembre": "december",
	"gen": "jan", "mag": "may", "giu": "jun", "lug": "jul", "ago": "aug", "set": "sep", "ott": "oct", "dic": "dec",
	"ogni": "every", "giorno": "day", "giorni": "days", "settimana": "week", "settimane": "weeks",
	"mese": "month", "mesi": "months", "anno": "year", "anni": "years",
	"giornaliero": "daily", "settimanale": "weekly", "mensile": "monthly", "annuale": "yearly", "annualmente": "yearly",
	"feriali": "weekdays", "lavorativo": "workday", "lavorativi": "workday", "festivi": "holidays",
	"primo": "first", "prima": "first", "secondo": "second", "terzo": "third", "quarto": "fourth", "quinto": "fifth",
	"ultima": "last", "ultimo": "last", "1°": "1st", "2°": "2nd", "3°": "3rd", "4°": "4th", "5°": "5th",
	"salta": "skip", "volta": "time", "volte": "times", "date": "dates", "curva": "curve",
	"dal": "from", "dopo": "after", "completamento": "completion",
	"il": "the", "lo": "the", "la": "the", "del": "of", "della": "of", "di": "of", "al": "on", "ad": "on", "e": "and",
}

// itWord maps one Italian word to its English equivalent, or returns it unchanged.
func itWord(w string) string {
	if e, ok := itWords[w]; ok {
		return e
	}
	if n, ok := strings.CutSuffix(w, "°"); ok { // 6°, 23°: the bare day of the month
		if _, err := strconv.Atoi(n); err == nil {
			return n
		}
	}
	return w
}

// itNormalize prepares a lowercased Italian repeat rule for ParseRepeat.
func itNormalize(s string) string {
	s = itPhrases.Replace(s)
	w := strings.Fields(strings.NewReplacer(",", " ", ";", " ").Replace(s))
	for i, x := range w {
		w[i] = itWord(x)
	}
	return strings.Join(w, " ")
}
