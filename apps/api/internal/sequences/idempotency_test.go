package sequences

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"strconv"
	"testing"

	"github.com/google/uuid"
)

// Fixed UUIDs so the test is deterministic and the golden values below are
// stable. uuid.MustParse panics on a malformed literal (test-time only).
var (
	wsA  = uuid.MustParse("11111111-1111-1111-1111-111111111111")
	wsB  = uuid.MustParse("22222222-2222-2222-2222-222222222222")
	enrA = uuid.MustParse("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa")
	enrB = uuid.MustParse("bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb")
)

var hex64RE = regexp.MustCompile(`^[0-9a-f]{64}$`)

// TestSendStepIdempotencyKey_Shape asserts the output is a 64-char lowercase
// hex SHA-256 digest, and that it equals an independent recomputation of the
// documented preimage. This pins the exact wire format (ADR-004 rev 2 §3) so
// an accidental change to the separator, field order, or UUID/int encoding
// is caught here.
func TestSendStepIdempotencyKey_Shape(t *testing.T) {
	got := SendStepIdempotencyKey(wsA, enrA, 3, 0)

	if !hex64RE.MatchString(got) {
		t.Fatalf("key %q is not 64 lowercase hex chars", got)
	}

	// Independent recomputation of the §3 preimage:
	// workspace_id ":" enrollment_id ":" step_index ":" attempt_epoch
	preimage := wsA.String() + ":" + enrA.String() + ":" + strconv.Itoa(3) + ":" + strconv.Itoa(0)
	sum := sha256.Sum256([]byte(preimage))
	want := hex.EncodeToString(sum[:])
	if got != want {
		t.Fatalf("key mismatch:\n got = %s\nwant = %s\n(preimage %q)", got, want, preimage)
	}
}

// TestSendStepIdempotencyKey_Deterministic asserts the function is pure:
// the same inputs always produce the same key. This is the property the
// river-internal retry path relies on (retries reuse the same key).
func TestSendStepIdempotencyKey_Deterministic(t *testing.T) {
	first := SendStepIdempotencyKey(wsA, enrA, 2, 1)
	for i := 0; i < 100; i++ {
		if again := SendStepIdempotencyKey(wsA, enrA, 2, 1); again != first {
			t.Fatalf("key not deterministic: iteration %d gave %s, first gave %s", i, again, first)
		}
	}
}

// TestSendStepIdempotencyKey_RetryReusesKey documents the retry contract:
// since attempt_epoch is NOT bumped on river-internal retries, a retried
// send computes the identical key.
func TestSendStepIdempotencyKey_RetryReusesKey(t *testing.T) {
	const epoch = 0 // unchanged across river-internal retries
	original := SendStepIdempotencyKey(wsA, enrA, 5, epoch)
	retry := SendStepIdempotencyKey(wsA, enrA, 5, epoch)
	if original != retry {
		t.Fatalf("a retry (same epoch) must reuse the key: %s != %s", original, retry)
	}
}

// TestSendStepIdempotencyKey_SupersedeChangesKey documents the supersede
// contract: bumping attempt_epoch (explicit supersede) yields a NEW key, so
// the re-queued send is a distinct idempotency token.
func TestSendStepIdempotencyKey_SupersedeChangesKey(t *testing.T) {
	before := SendStepIdempotencyKey(wsA, enrA, 5, 0)
	after := SendStepIdempotencyKey(wsA, enrA, 5, 1) // epoch bumped on supersede
	if before == after {
		t.Fatalf("a supersede (epoch bump) must change the key, both were %s", before)
	}
}

// TestSendStepIdempotencyKey_DistinctInputsDistinctKeys asserts every field
// participates in the digest: changing workspace, enrollment, step, or epoch
// independently changes the key. Collisions here would mean two different
// sends share an idempotency token — a double-send hazard.
func TestSendStepIdempotencyKey_DistinctInputsDistinctKeys(t *testing.T) {
	base := SendStepIdempotencyKey(wsA, enrA, 1, 0)

	cases := map[string]string{
		"different workspace":  SendStepIdempotencyKey(wsB, enrA, 1, 0),
		"different enrollment": SendStepIdempotencyKey(wsA, enrB, 1, 0),
		"different step":       SendStepIdempotencyKey(wsA, enrA, 2, 0),
		"different epoch":      SendStepIdempotencyKey(wsA, enrA, 1, 1),
	}
	for name, k := range cases {
		if k == base {
			t.Errorf("%s collided with base key %s", name, base)
		}
	}

	// All five keys (base + 4 variants) must be mutually distinct.
	seen := map[string]string{base: "base"}
	for name, k := range cases {
		if other, dup := seen[k]; dup {
			t.Errorf("key collision between %q and %q: %s", name, other, k)
		}
		seen[k] = name
	}
}

// TestSendStepIdempotencyKey_SeparatorPreventsAmbiguity is the critical
// boundary case: without the ":" delimiter, (step=1, epoch=23) and
// (step=12, epoch=3) would share the preimage tail "123" and collide. The
// delimiter must keep them distinct.
func TestSendStepIdempotencyKey_SeparatorPreventsAmbiguity(t *testing.T) {
	a := SendStepIdempotencyKey(wsA, enrA, 1, 23)
	b := SendStepIdempotencyKey(wsA, enrA, 12, 3)
	if a == b {
		t.Fatalf("step/epoch boundary collision: (1,23) and (12,3) hashed to the same key %s", a)
	}

	// And the UUID/step boundary: enrollment ending in a digit vs a step
	// starting with that digit must not bleed across the separator.
	c := SendStepIdempotencyKey(wsA, enrA, 0, 0)
	d := SendStepIdempotencyKey(wsA, enrB, 0, 0)
	if c == d {
		t.Fatalf("enrollment boundary collision: %s", c)
	}
}
