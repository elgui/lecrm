package sequences

import (
	"context"
	"testing"
)

// TestEvaluateBounce covers the pure bounce policy (ADR-004 rev 2 §8): hard and
// complaint suppress on a single event; soft suppresses only at the consecutive
// threshold.
func TestEvaluateBounce(t *testing.T) {
	cases := []struct {
		name         string
		kind         BounceKind
		consecutive  int
		limits       Limits
		wantSuppress bool
		wantReason   string
	}{
		{"hard bounce suppresses immediately", BounceHard, 0, Limits{}, true, SuppressionReasonHardBounce},
		{"complaint suppresses immediately", BounceComplaint, 0, Limits{}, true, SuppressionReasonComplaint},
		{"one soft bounce does not suppress", BounceSoft, 1, Limits{}, false, ""},
		{"two soft bounces do not suppress", BounceSoft, 2, Limits{}, false, ""},
		{"three consecutive soft bounces suppress (default threshold)", BounceSoft, 3, Limits{}, true, SuppressionReasonHardBounce},
		{"four soft bounces still suppress", BounceSoft, 4, Limits{}, true, SuppressionReasonHardBounce},
		{"custom threshold of 2 suppresses at 2", BounceSoft, 2, Limits{SoftBounceSuppressThreshold: 2}, true, SuppressionReasonHardBounce},
		{"unknown kind never suppresses", BounceKind("delivered"), 99, Limits{}, false, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := EvaluateBounce(c.kind, c.consecutive, c.limits)
			if got.Suppress != c.wantSuppress {
				t.Errorf("Suppress = %v, want %v", got.Suppress, c.wantSuppress)
			}
			if got.Reason != c.wantReason {
				t.Errorf("Reason = %q, want %q", got.Reason, c.wantReason)
			}
		})
	}
}

// TestEvaluateBounce_DefaultThreshold pins the documented default (3) so a
// change to DefaultSoftBounceSuppressThreshold is caught.
func TestEvaluateBounce_DefaultThreshold(t *testing.T) {
	if DefaultSoftBounceSuppressThreshold != 3 {
		t.Fatalf("DefaultSoftBounceSuppressThreshold = %d, want 3 (ADR-004 rev 2 §8)", DefaultSoftBounceSuppressThreshold)
	}
	if EvaluateBounce(BounceSoft, 2, Limits{}).Suppress {
		t.Error("2 soft bounces suppressed under the default threshold of 3")
	}
	if !EvaluateBounce(BounceSoft, 3, Limits{}).Suppress {
		t.Error("3 soft bounces did not suppress under the default threshold")
	}
}

// TestConsecutiveSoftBounces reads the trailing soft-bounce run for an address.
func TestConsecutiveSoftBounces(t *testing.T) {
	tx := &preflightTx{softRun: 3}
	n, err := ConsecutiveSoftBounces(context.Background(), tx, "x@example.fr")
	if err != nil {
		t.Fatalf("ConsecutiveSoftBounces error: %v", err)
	}
	if n != 3 {
		t.Fatalf("count = %d, want 3", n)
	}
}

// TestApplyBounce_HardImmediateSuppression: a hard bounce writes the suppression
// row immediately and does NOT count soft bounces.
func TestApplyBounce_HardImmediateSuppression(t *testing.T) {
	tx := &preflightTx{}
	action, err := ApplyBounce(context.Background(), tx, "hard@example.fr", BounceHard, DefaultLimits())
	if err != nil {
		t.Fatalf("ApplyBounce error: %v", err)
	}
	if !action.Suppress || action.Reason != SuppressionReasonHardBounce {
		t.Fatalf("action = %+v, want suppress/hard_bounce", action)
	}
	if tx.issued("is_soft") {
		t.Error("hard bounce counted soft bounces (unnecessary)")
	}
	ex, ok := tx.execMatching("INSERT INTO email_suppression")
	if !ok {
		t.Fatal("no email_suppression upsert for a hard bounce")
	}
	if ex.args[0] != "hard@example.fr" || ex.args[1] != SuppressionReasonHardBounce {
		t.Errorf("upsert args = %v, want [hard@example.fr hard_bounce]", ex.args)
	}
}

// TestApplyBounce_SoftBelowThreshold_NoSuppress: a soft bounce under the
// threshold records nothing.
func TestApplyBounce_SoftBelowThreshold_NoSuppress(t *testing.T) {
	tx := &preflightTx{softRun: 2}
	action, err := ApplyBounce(context.Background(), tx, "soft@example.fr", BounceSoft, DefaultLimits())
	if err != nil {
		t.Fatalf("ApplyBounce error: %v", err)
	}
	if action.Suppress {
		t.Fatalf("action = %+v, want no suppression at 2 soft bounces", action)
	}
	if _, ok := tx.execMatching("INSERT INTO email_suppression"); ok {
		t.Error("suppression row written below the soft-bounce threshold")
	}
}

// TestApplyBounce_SoftThreeConsecutive_Suppresses: the 3rd consecutive soft
// bounce suppresses the recipient (recorded as hard_bounce).
func TestApplyBounce_SoftThreeConsecutive_Suppresses(t *testing.T) {
	tx := &preflightTx{softRun: 3}
	action, err := ApplyBounce(context.Background(), tx, "soft3@example.fr", BounceSoft, DefaultLimits())
	if err != nil {
		t.Fatalf("ApplyBounce error: %v", err)
	}
	if !action.Suppress || action.Reason != SuppressionReasonHardBounce {
		t.Fatalf("action = %+v, want suppress/hard_bounce at 3 consecutive soft bounces", action)
	}
	ex, ok := tx.execMatching("INSERT INTO email_suppression")
	if !ok {
		t.Fatal("no suppression upsert after 3 consecutive soft bounces")
	}
	if ex.args[0] != "soft3@example.fr" {
		t.Errorf("upsert email = %v, want soft3@example.fr", ex.args[0])
	}
}
