package ooo

import (
	"testing"
	"time"
)

// fixedNow is the deterministic clock for date-parsing tests: a Wednesday so the
// AddBusinessDays weekend-skip is exercised. 2026-05-13 is a Wednesday.
var fixedNow = time.Date(2026, time.May, 13, 8, 0, 0, 0, time.UTC)

func TestParseReturnDate(t *testing.T) {
	tests := []struct {
		name    string
		subject string
		body    string
		want    time.Time
		wantOK  bool
	}{
		{
			name:   "iso date",
			body:   "Currently out of the office, returning on 2026-06-20.",
			want:   time.Date(2026, time.June, 20, resumeHourUTC, 0, 0, 0, time.UTC),
			wantOK: true,
		},
		{
			name:   "french de retour le D mois",
			body:   "Je suis absent, de retour le 15 mai.",
			want:   time.Date(2026, time.May, 15, resumeHourUTC, 0, 0, 0, time.UTC),
			wantOK: true,
		},
		{
			name: "english month day infers next year when already past",
			body: "I am out of office and will return on May 5.",
			// May 5 is before now (2026-05-13) with no explicit year → next year.
			want:   time.Date(2027, time.May, 5, resumeHourUTC, 0, 0, 0, time.UTC),
			wantOK: true,
		},
		{
			name:   "english back on D Month with ordinal",
			body:   "Out of the office — back on 12th June.",
			want:   time.Date(2026, time.June, 12, resumeHourUTC, 0, 0, 0, time.UTC),
			wantOK: true,
		},
		{
			name:   "french numeric with return keyword",
			body:   "Absente, de retour le 02/06.",
			want:   time.Date(2026, time.June, 2, resumeHourUTC, 0, 0, 0, time.UTC),
			wantOK: true,
		},
		{
			name:   "numeric without keyword is ignored",
			body:   "Call me at extension 15/06 if urgent.",
			wantOK: false,
		},
		{
			name:   "no date at all",
			body:   "I'm on holiday and will respond when I return.",
			wantOK: false,
		},
		{
			name:   "impossible date rejected",
			body:   "de retour le 30/02",
			wantOK: false,
		},
		{
			name:   "explicit year honoured",
			body:   "back on 3 March 2027",
			want:   time.Date(2027, time.March, 3, resumeHourUTC, 0, 0, 0, time.UTC),
			wantOK: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := ParseReturnDate(ReplyBody{Subject: tc.subject, Body: tc.body}, fixedNow)
			if ok != tc.wantOK {
				t.Fatalf("ParseReturnDate ok=%v, want %v (got %v)", ok, tc.wantOK, got)
			}
			if !tc.wantOK {
				return
			}
			if !got.Equal(tc.want) {
				t.Errorf("ParseReturnDate = %s, want %s", got.Format(time.RFC3339), tc.want.Format(time.RFC3339))
			}
		})
	}
}

func TestParseReturnDate_InfersNextYearForPastDate(t *testing.T) {
	// "1 May" is already past relative to fixedNow (2026-05-13) → next year.
	got, ok := ParseReturnDate(ReplyBody{Body: "de retour le 1 mai"}, fixedNow)
	if !ok {
		t.Fatal("expected a parse")
	}
	want := time.Date(2027, time.May, 1, resumeHourUTC, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Errorf("past-date inference = %s, want %s", got.Format(time.RFC3339), want.Format(time.RFC3339))
	}
}

func TestAddBusinessDays(t *testing.T) {
	tests := []struct {
		name string
		from time.Time
		n    int
		want time.Time
	}{
		{
			name: "wednesday +5 skips one weekend",
			from: time.Date(2026, time.May, 13, 8, 0, 0, 0, time.UTC), // Wednesday
			n:    5,
			want: time.Date(2026, time.May, 20, resumeHourUTC, 0, 0, 0, time.UTC), // next Wednesday
		},
		{
			name: "friday +1 lands monday",
			from: time.Date(2026, time.May, 15, 8, 0, 0, 0, time.UTC), // Friday
			n:    1,
			want: time.Date(2026, time.May, 18, resumeHourUTC, 0, 0, 0, time.UTC), // Monday
		},
		{
			name: "normalises to resume hour",
			from: time.Date(2026, time.May, 13, 23, 30, 0, 0, time.UTC),
			n:    1,
			want: time.Date(2026, time.May, 14, resumeHourUTC, 0, 0, 0, time.UTC),
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := AddBusinessDays(tc.from, tc.n)
			if !got.Equal(tc.want) {
				t.Errorf("AddBusinessDays(%s, %d) = %s, want %s",
					tc.from.Format(time.RFC3339), tc.n, got.Format(time.RFC3339), tc.want.Format(time.RFC3339))
			}
			if wd := got.Weekday(); wd == time.Saturday || wd == time.Sunday {
				t.Errorf("AddBusinessDays landed on a weekend: %s", got.Weekday())
			}
		})
	}
}
