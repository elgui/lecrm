package sequences

import (
	"crypto/sha256"
	"encoding/hex"
	"strconv"

	"github.com/google/uuid"
)

// idempotencySep is the field separator in the send_step idempotency-key
// preimage. A single non-hex, non-digit byte makes the preimage
// unambiguous: it prevents (step_index=1, attempt_epoch=23) from hashing
// to the same value as (step_index=12, attempt_epoch=3), which a bare
// concatenation would allow.
const idempotencySep = ":"

// SendStepIdempotencyKey derives the application-level idempotency key for a
// sequences.send_step job, per ADR-004 rev 2 §3:
//
//	sha256(workspace_id || ':' || enrollment_id || ':' || step_index || ':' || attempt_epoch)
//
// The returned value is the lowercase hex encoding of the SHA-256 digest
// (64 chars), stored in enrollment_steps.idempotency_key.
//
// attemptEpoch semantics (ADR-004 rev 2 §3): the epoch is incremented ONLY
// when a previous attempt was explicitly marked `superseded` (template
// edited mid-flight, user manually re-queued). It is NOT incremented on
// river-internal retries — those reuse the same key so the send is
// genuinely idempotent across retries, and the partial unique index
// `uniq_enrollment_step_active` (§1) is the durable backstop that
// guarantees at most one active row per (enrollment_id, step_index).
//
// The function is pure: identical inputs always yield the identical key.
// That is the property the retry path relies on. The caller owns the epoch
// counter; this function does not read or mutate any state.
//
// UUIDs are encoded in their canonical lowercase 8-4-4-4-12 hyphenated form
// (uuid.UUID.String); the integers are base-10. Changing any of these
// encodings is a wire-format change: it would orphan in-flight send_step
// jobs whose key was computed under the old encoding, so it must not be
// done without a migration plan.
func SendStepIdempotencyKey(workspaceID, enrollmentID uuid.UUID, stepIndex, attemptEpoch int) string {
	h := sha256.New()
	h.Write([]byte(workspaceID.String()))
	h.Write([]byte(idempotencySep))
	h.Write([]byte(enrollmentID.String()))
	h.Write([]byte(idempotencySep))
	h.Write([]byte(strconv.Itoa(stepIndex)))
	h.Write([]byte(idempotencySep))
	h.Write([]byte(strconv.Itoa(attemptEpoch)))
	return hex.EncodeToString(h.Sum(nil))
}
