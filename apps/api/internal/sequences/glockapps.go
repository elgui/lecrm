package sequences

import (
	"errors"
	"fmt"
)

// GlockApps pre-flight content scoring (ADR-004 rev 2 §8, ADR-003 §Mitigations
// item 2). Before a sequence template is ACTIVATED, its content is scored for
// inbox-placement / spam risk on a 0–10 scale. A score below GlockAppsMinScore
// blocks activation; an admin may override the block with explicit confirmation
// (the score is surfaced as a warning, not a hard stop, so a deliberately
// edgy-but-legitimate template can still ship).
//
// INTEGRATION TIER (ADR-004 rev 2 §Q2 — OPEN). v1 obtains the score MANUALLY:
// an operator runs ops/scripts/glockapps-preflight.sh, performs the GlockApps
// inbox-placement test in the GlockApps web UI, and records the 0–10 score,
// which is fed to CheckTemplateActivation at activation time. Automated
// triggering of GlockApps from the engine requires GlockApps' API tier, which
// has not been costed/approved — this package deliberately contains NO GlockApps
// API client. Do not add one without the §Q2 decision.
//
// This file is the pure activation gate: a future template-save/activation path
// calls CheckTemplateActivation(score, adminOverride) and honours the result.
// The scoring itself is external (GlockApps); the engine only enforces the
// threshold and the override discipline.

// GlockAppsMinScore is the minimum GlockApps content score (out of GlockAppsMaxScore)
// a template needs to activate without an admin override (ADR-004 rev 2 §8).
const GlockAppsMinScore = 7

// GlockAppsMaxScore is the top of the GlockApps content score scale.
const GlockAppsMaxScore = 10

// ErrInvalidGlockAppsScore is returned when a score is outside [0, GlockAppsMaxScore].
var ErrInvalidGlockAppsScore = errors.New("sequences: glockapps score out of range")

// ActivationDecision is the outcome of the content-score gate.
type ActivationDecision struct {
	// Allowed is true when the template may activate — either the score met the
	// minimum, or it was below the minimum but an admin explicitly overrode.
	Allowed bool
	// Blocked is true when activation is refused (below minimum, no override).
	Blocked bool
	// Overridden is true when a sub-threshold score was allowed through by an
	// explicit admin override — the caller MUST record this on the audit trail.
	Overridden bool
	// Score is the evaluated content score, echoed for logging/audit.
	Score int
	// Reason is a human-readable explanation for the admin UI and audit payload.
	Reason string
}

// CheckTemplateActivation gates activation of a sequence template on its
// GlockApps content score (ADR-004 rev 2 §8). adminOverride is the explicit
// confirmation an admin gives to ship a template that scored below the minimum;
// it has effect ONLY when the score is sub-threshold (an override on an
// already-passing template is a no-op, not recorded as an override).
//
// Returns ErrInvalidGlockAppsScore when score is outside [0, GlockAppsMaxScore]
// — a malformed score must fail loudly, never silently pass the gate.
func CheckTemplateActivation(score int, adminOverride bool) (ActivationDecision, error) {
	if score < 0 || score > GlockAppsMaxScore {
		return ActivationDecision{}, fmt.Errorf("%w: %d (want 0..%d)", ErrInvalidGlockAppsScore, score, GlockAppsMaxScore)
	}

	if score >= GlockAppsMinScore {
		return ActivationDecision{
			Allowed: true,
			Score:   score,
			Reason:  fmt.Sprintf("GlockApps content score %d/%d meets the %d/%d minimum", score, GlockAppsMaxScore, GlockAppsMinScore, GlockAppsMaxScore),
		}, nil
	}

	if adminOverride {
		return ActivationDecision{
			Allowed:    true,
			Overridden: true,
			Score:      score,
			Reason:     fmt.Sprintf("admin override: GlockApps content score %d/%d below the %d/%d minimum", score, GlockAppsMaxScore, GlockAppsMinScore, GlockAppsMaxScore),
		}, nil
	}

	return ActivationDecision{
		Blocked: true,
		Score:   score,
		Reason:  fmt.Sprintf("GlockApps content score %d/%d below the %d/%d minimum; admin override required to activate", score, GlockAppsMaxScore, GlockAppsMinScore, GlockAppsMaxScore),
	}, nil
}
