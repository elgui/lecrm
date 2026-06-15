package sequences

import (
	"errors"
	"testing"
)

// TestCheckTemplateActivation covers the GlockApps content-score gate
// (ADR-004 rev 2 §8): >= 7/10 activates; < 7/10 is blocked unless an admin
// explicitly overrides.
func TestCheckTemplateActivation(t *testing.T) {
	cases := []struct {
		name           string
		score          int
		override       bool
		wantAllowed    bool
		wantBlocked    bool
		wantOverridden bool
	}{
		{"score above minimum activates", 8, false, true, false, false},
		{"score at minimum activates", 7, false, true, false, false},
		{"perfect score activates", 10, false, true, false, false},
		{"below minimum is blocked without override", 6, false, false, true, false},
		{"zero score is blocked without override", 0, false, false, true, false},
		{"below minimum activates with admin override", 6, true, true, false, true},
		{"override on a passing score is a no-op (not recorded as override)", 9, true, true, false, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := CheckTemplateActivation(c.score, c.override)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.Allowed != c.wantAllowed {
				t.Errorf("Allowed = %v, want %v", got.Allowed, c.wantAllowed)
			}
			if got.Blocked != c.wantBlocked {
				t.Errorf("Blocked = %v, want %v", got.Blocked, c.wantBlocked)
			}
			if got.Overridden != c.wantOverridden {
				t.Errorf("Overridden = %v, want %v", got.Overridden, c.wantOverridden)
			}
			if got.Score != c.score {
				t.Errorf("Score = %d, want %d", got.Score, c.score)
			}
			if got.Reason == "" {
				t.Error("Reason is empty")
			}
		})
	}
}

// TestCheckTemplateActivation_BlockedIsExclusive: a blocked decision is never
// also Allowed — the activation path must not accidentally ship a blocked
// template.
func TestCheckTemplateActivation_BlockedIsExclusive(t *testing.T) {
	got, err := CheckTemplateActivation(GlockAppsMinScore-1, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Allowed {
		t.Fatal("a sub-threshold score without override was Allowed")
	}
	if !got.Blocked {
		t.Fatal("a sub-threshold score without override was not Blocked")
	}
}

// TestCheckTemplateActivation_InvalidScore: a score outside [0,10] fails loudly
// rather than silently passing or blocking the gate.
func TestCheckTemplateActivation_InvalidScore(t *testing.T) {
	for _, bad := range []int{-1, 11, 100} {
		if _, err := CheckTemplateActivation(bad, false); !errors.Is(err, ErrInvalidGlockAppsScore) {
			t.Errorf("score %d: error = %v, want ErrInvalidGlockAppsScore", bad, err)
		}
		// An override must not rescue an out-of-range score.
		if _, err := CheckTemplateActivation(bad, true); !errors.Is(err, ErrInvalidGlockAppsScore) {
			t.Errorf("score %d (override): error = %v, want ErrInvalidGlockAppsScore", bad, err)
		}
	}
}

// TestGlockAppsThreshold pins the §8 constants so a change is deliberate.
func TestGlockAppsThreshold(t *testing.T) {
	if GlockAppsMinScore != 7 || GlockAppsMaxScore != 10 {
		t.Fatalf("GlockApps thresholds = %d/%d, want 7/10 (ADR-004 rev 2 §8)", GlockAppsMinScore, GlockAppsMaxScore)
	}
}
