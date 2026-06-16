package sequences

import (
	"strings"
	"testing"
)

func TestAllJobKinds(t *testing.T) {
	want := map[string]bool{
		"sequences.enroll":     true,
		"sequences.send_step":  true,
		"sequences.poll_reply": true,
		"sequences.finalize":   true,
	}
	got := AllJobKinds()
	if len(got) != len(want) {
		t.Fatalf("AllJobKinds len = %d, want %d", len(got), len(want))
	}
	seen := map[string]bool{}
	for _, k := range got {
		if !want[k] {
			t.Errorf("unexpected job kind %q", k)
		}
		if seen[k] {
			t.Errorf("duplicate job kind %q", k)
		}
		seen[k] = true
		if !strings.HasPrefix(k, "sequences.") {
			t.Errorf("job kind %q is not under the sequences. namespace", k)
		}
	}
}

func TestAllJobKindsIsDefensiveCopy(t *testing.T) {
	out := AllJobKinds()
	out[0] = "mutated"
	if allJobKinds[0] == "mutated" {
		t.Fatal("AllJobKinds returned a slice aliasing the internal allJobKinds")
	}
}

func TestAuditEventsAreNamespacedAndUnique(t *testing.T) {
	got := AuditEvents()
	if len(got) != 8 {
		t.Fatalf("AuditEvents len = %d, want 8 (ADR-004 rev 2 §6)", len(got))
	}
	seen := map[string]bool{}
	for _, e := range got {
		if !strings.HasPrefix(e, "sequences.") {
			t.Errorf("audit event %q is not under the sequences. namespace", e)
		}
		if seen[e] {
			t.Errorf("duplicate audit event %q", e)
		}
		seen[e] = true
	}
	// The §6 catalogue, spelled out so a rename is caught here.
	for _, want := range []string{
		"sequences.enrolled", "sequences.step_sent", "sequences.reply_received",
		"sequences.ooo_detected", "sequences.failed", "sequences.bounced",
		"sequences.unsubscribed", "sequences.transition.invalid",
	} {
		if !seen[want] {
			t.Errorf("missing ADR-004 rev 2 §6 audit event %q", want)
		}
	}
}

func TestAuditEventsIsDefensiveCopy(t *testing.T) {
	out := AuditEvents()
	out[0] = "mutated"
	if auditEvents[0] == "mutated" {
		t.Fatal("AuditEvents returned a slice aliasing the internal auditEvents")
	}
}
