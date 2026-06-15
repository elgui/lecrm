package ooo

import (
	"regexp"
	"strconv"
	"strings"
	"time"
)

// resumeHourUTC is the hour-of-day (UTC) a rescheduled OOO enrollment resumes on
// its return date. A bare "de retour le 15 mai" carries no time; 09:00 UTC keeps
// the resume inside business hours for European mailboxes without sending at
// midnight. The same hour normalises the +5-business-day default so both paths
// produce a deterministic, comparable timestamp.
const resumeHourUTC = 9

// ParseReturnDate extracts an out-of-office return date from a reply, resolving
// it against now (ADR-004 rev 2 §5: the "de retour le 15 mai" parsing). It
// returns (returnDate, true) on success; callers fall back to +5 business days
// when it returns false.
//
// It recognises, in French and English:
//
//	ISO            2026-05-15
//	D Month [Y]    15 mai, 15 mai 2026, 15th May
//	Month D[,] [Y] May 15, May 15 2026, mai 15
//	numeric DMY    15/05, 15/05/2026, 15.05.26   (only when a return keyword is present)
//
// A missing year is inferred as the next occurrence on/after now (an OOO return
// is in the near future). The numeric DMY form is day-first (French default) and
// is only honoured alongside a "back on / de retour le / jusqu'au …" keyword, so
// stray phone or version numbers in a snippet are not misread as dates.
func ParseReturnDate(body ReplyBody, now time.Time) (time.Time, bool) {
	text := foldAccents(strings.ToLower(body.Subject + "\n" + body.Body))

	if t, ok := parseISO(text, now); ok {
		return t, true
	}
	if t, ok := parseDayMonth(text, now); ok {
		return t, true
	}
	if t, ok := parseMonthDay(text, now); ok {
		return t, true
	}
	if hasReturnKeyword(text) {
		if t, ok := parseNumeric(text, now); ok {
			return t, true
		}
	}
	return time.Time{}, false
}

var (
	reISO      = regexp.MustCompile(`\b(\d{4})-(\d{1,2})-(\d{1,2})\b`)
	reDayMonth = regexp.MustCompile(`\b(\d{1,2})(?:er|st|nd|rd|th)?\s+([a-zéûôîè]+)\.?(?:\s+(\d{4}))?\b`)
	reMonthDay = regexp.MustCompile(`\b([a-zéûôîè]+)\.?\s+(\d{1,2})(?:er|st|nd|rd|th)?(?:[,\s]+(\d{4}))?\b`)
	reNumeric  = regexp.MustCompile(`\b(\d{1,2})[/.](\d{1,2})(?:[/.](\d{2,4}))?\b`)
	reReturnKW = regexp.MustCompile(`(?i)\b(?:back\s+on|return(?:ing)?\s+(?:on|the)|de\s+retour|jusqu['’ ]?au|a\s+partir\s+du|until|reviens|resume[rd]?)\b`)
)

func hasReturnKeyword(text string) bool { return reReturnKW.MatchString(text) }

func parseISO(text string, now time.Time) (time.Time, bool) {
	m := reISO.FindStringSubmatch(text)
	if m == nil {
		return time.Time{}, false
	}
	y, _ := strconv.Atoi(m[1])
	mo, _ := strconv.Atoi(m[2])
	d, _ := strconv.Atoi(m[3])
	return assemble(y, time.Month(mo), d, now)
}

func parseDayMonth(text string, now time.Time) (time.Time, bool) {
	for _, m := range reDayMonth.FindAllStringSubmatch(text, -1) {
		mo, ok := lookupMonth(m[2])
		if !ok {
			continue
		}
		d, _ := strconv.Atoi(m[1])
		y := 0
		if m[3] != "" {
			y, _ = strconv.Atoi(m[3])
		}
		if t, ok := assemble(y, mo, d, now); ok {
			return t, true
		}
	}
	return time.Time{}, false
}

func parseMonthDay(text string, now time.Time) (time.Time, bool) {
	for _, m := range reMonthDay.FindAllStringSubmatch(text, -1) {
		mo, ok := lookupMonth(m[1])
		if !ok {
			continue
		}
		d, _ := strconv.Atoi(m[2])
		y := 0
		if m[3] != "" {
			y, _ = strconv.Atoi(m[3])
		}
		if t, ok := assemble(y, mo, d, now); ok {
			return t, true
		}
	}
	return time.Time{}, false
}

func parseNumeric(text string, now time.Time) (time.Time, bool) {
	for _, m := range reNumeric.FindAllStringSubmatch(text, -1) {
		d, _ := strconv.Atoi(m[1])
		moN, _ := strconv.Atoi(m[2])
		if moN < 1 || moN > 12 {
			continue // day-first: a >12 second field is not a month
		}
		y := 0
		if m[3] != "" {
			y, _ = strconv.Atoi(m[3])
			if y < 100 {
				y += 2000
			}
		}
		if t, ok := assemble(y, time.Month(moN), d, now); ok {
			return t, true
		}
	}
	return time.Time{}, false
}

// assemble builds a return timestamp from (year, month, day), inferring a missing
// year as the next occurrence on/after now and rejecting impossible dates (e.g.
// 30 February, which time.Date would silently roll into March).
func assemble(year int, mo time.Month, day int, now time.Time) (time.Time, bool) {
	if mo < time.January || mo > time.December || day < 1 || day > 31 {
		return time.Time{}, false
	}
	if year == 0 {
		year = now.Year()
	}
	t := time.Date(year, mo, day, resumeHourUTC, 0, 0, 0, time.UTC)
	if t.Day() != day || t.Month() != mo {
		return time.Time{}, false // rolled over → impossible date
	}
	// Infer the next occurrence when no explicit year was given: an OOO return is
	// in the near future, so a date already past this year means next year.
	if year == now.Year() && t.Before(now.Add(-24*time.Hour)) {
		t = t.AddDate(1, 0, 0)
	}
	return t, true
}

// months maps French + English month names (and common abbreviations) to
// time.Month. Keys are accent-folded lowercase, matching ParseReturnDate input.
var months = map[string]time.Month{
	// French
	"janvier": time.January, "janv": time.January,
	"fevrier": time.February, "fevr": time.February, "fev": time.February,
	"mars":   time.March,
	"avril":  time.April, "avr": time.April,
	"mai":    time.May,
	"juin":   time.June,
	"juillet": time.July, "juil": time.July, "juill": time.July,
	"aout":      time.August,
	"septembre": time.September,
	"octobre":   time.October,
	"novembre":  time.November,
	"decembre":  time.December,
	// English
	"january": time.January, "jan": time.January,
	"february": time.February, "feb": time.February,
	"march":  time.March, "mar": time.March,
	"may":    time.May,
	"july":   time.July, "jul": time.July,
	"august": time.August, "aug": time.August,
	"september": time.September, "sept": time.September, "sep": time.September,
	"october": time.October, "oct": time.October,
	"november": time.November, "nov": time.November,
	"december": time.December, "dec": time.December,
	// shared spellings (april/avril collide-free; june/juin distinct)
	"april": time.April,
	"june":  time.June,
}

func lookupMonth(word string) (time.Month, bool) {
	m, ok := months[strings.TrimSuffix(word, ".")]
	return m, ok
}

// AddBusinessDays returns t advanced by n weekdays (Saturday and Sunday
// skipped), normalised to resumeHourUTC in UTC. It is the +5-business-day OOO
// default (ADR-004 rev 2 §5) and is exported for reuse by the scheduler. Public
// holidays are intentionally ignored at v1 — the default is a coarse "resume in
// about a week" fallback, not a precise calendar.
func AddBusinessDays(t time.Time, n int) time.Time {
	d := t.UTC()
	for n > 0 {
		d = d.AddDate(0, 0, 1)
		if wd := d.Weekday(); wd != time.Saturday && wd != time.Sunday {
			n--
		}
	}
	return time.Date(d.Year(), d.Month(), d.Day(), resumeHourUTC, 0, 0, 0, time.UTC)
}
