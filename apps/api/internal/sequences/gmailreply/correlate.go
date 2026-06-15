package gmailreply

import (
	"strings"

	"github.com/google/uuid"

	"github.com/gbconsult/lecrm/apps/api/internal/sequences"
)

// InboundMessage is one new INBOX message surfaced by a Gmail history scan,
// reduced to exactly the fields reply correlation and OOO classification need.
// The full body is never carried — only the snippet plus the headers required
// to thread the reply and to feed the classifier (ADR-009 §8.3: minimise PII on
// in-flight payloads).
type InboundMessage struct {
	GmailMessageID  string   // Gmail's opaque id (for messages.get / log correlation)
	RFC822MessageID string   // this message's own Message-ID header
	InReplyTo       string   // In-Reply-To header value (raw; may be empty)
	References      []string // References header, already split into individual ids
	From            string   // From header (for the classifier / audit)
	Subject         string   // Subject header (for the classifier)
	Snippet         string   // Gmail snippet (short preview; classifier input)
}

// ReferencedMessageIDs returns the normalised set of RFC822 Message-IDs this
// message is a reply to — the union of In-Reply-To and References, with angle
// brackets and surrounding whitespace stripped, de-duplicated, order-preserved.
// These are the keys matched against enrollment_steps.rfc_message_id.
func (m InboundMessage) ReferencedMessageIDs() []string {
	seen := make(map[string]struct{})
	out := make([]string, 0, len(m.References)+1)
	add := func(raw string) {
		for _, id := range splitMessageIDs(raw) {
			id = NormalizeMessageID(id)
			if id == "" {
				continue
			}
			if _, dup := seen[id]; dup {
				continue
			}
			seen[id] = struct{}{}
			out = append(out, id)
		}
	}
	// In-Reply-To is the most specific (the direct parent) — list it first.
	add(m.InReplyTo)
	for _, r := range m.References {
		add(r)
	}
	return out
}

// splitMessageIDs splits a raw header value that may contain several
// whitespace-separated message-ids (the References header is a space/CRLF
// separated list) into individual tokens.
func splitMessageIDs(raw string) []string {
	if raw == "" {
		return nil
	}
	return strings.Fields(raw)
}

// NormalizeMessageID canonicalises one RFC822 Message-ID for comparison:
// trims whitespace and strips a single surrounding pair of angle brackets. It
// does NOT lowercase — the local part of a Message-ID is case-sensitive per
// RFC 5322, and Gmail preserves the id verbatim, so the value stored in
// enrollment_steps.rfc_message_id at send time matches byte-for-byte.
func NormalizeMessageID(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "<")
	s = strings.TrimSuffix(s, ">")
	return strings.TrimSpace(s)
}

// MatchedStep is one row of the indexed enrollment_steps × enrollments join:
// a sent step's Message-ID together with its enrollment and that enrollment's
// current state. Produced by matchSteps (cursor.go) and consumed by
// correlateReplies.
type MatchedStep struct {
	RFCMessageID string
	EnrollmentID uuid.UUID
	StepIndex    int
	State        sequences.State
}

// ReplyMatch is a confirmed correlation: an inbound message that references a
// sent step whose enrollment is still waiting for a reply. It is the unit the
// worker turns into a Transition.
type ReplyMatch struct {
	EnrollmentID   uuid.UUID
	StepIndex      int
	RFCMessageID   string // the sent step's Message-ID that was referenced
	ReplyMessageID string // the inbound reply's own Message-ID
	Message        InboundMessage
}

// correlateReplies pairs inbound messages with the steps they reply to. For
// each inbound message it walks its referenced Message-IDs (most-specific
// first) and, on the first one that matches a step whose enrollment is in
// waiting_reply, records a ReplyMatch. At most one match is produced per
// enrollment (first reply wins) — a thread can reference many prior ids, and a
// burst of replies to the same step must not double-transition the enrollment.
//
// It is a pure function over its inputs (no I/O), which is what makes the
// worker's correlation logic exhaustively unit-testable.
func correlateReplies(msgs []InboundMessage, matched []MatchedStep) []ReplyMatch {
	byMsgID := make(map[string]MatchedStep, len(matched))
	for _, s := range matched {
		key := NormalizeMessageID(s.RFCMessageID)
		if key == "" {
			continue
		}
		// Keep the first row per Message-ID; a Message-ID is unique per send.
		if _, exists := byMsgID[key]; !exists {
			byMsgID[key] = s
		}
	}

	claimed := make(map[uuid.UUID]struct{})
	out := make([]ReplyMatch, 0, len(msgs))
	for _, msg := range msgs {
		for _, ref := range msg.ReferencedMessageIDs() {
			step, ok := byMsgID[ref]
			if !ok {
				continue
			}
			if step.State != sequences.StateWaitingReply {
				// The step exists but its enrollment already moved on (replied,
				// completed, failed…). Nothing to do — skip without claiming.
				continue
			}
			if _, dup := claimed[step.EnrollmentID]; dup {
				continue
			}
			claimed[step.EnrollmentID] = struct{}{}
			out = append(out, ReplyMatch{
				EnrollmentID:   step.EnrollmentID,
				StepIndex:      step.StepIndex,
				RFCMessageID:   step.RFCMessageID,
				ReplyMessageID: msg.RFC822MessageID,
				Message:        msg,
			})
			break // one match per inbound message
		}
	}
	return out
}

// collectReferencedIDs returns the de-duplicated union of every inbound
// message's referenced Message-IDs — the parameter list for the single indexed
// enrollment_steps lookup (matchSteps).
func collectReferencedIDs(msgs []InboundMessage) []string {
	seen := make(map[string]struct{})
	out := make([]string, 0, len(msgs))
	for _, m := range msgs {
		for _, id := range m.ReferencedMessageIDs() {
			if _, dup := seen[id]; dup {
				continue
			}
			seen[id] = struct{}{}
			out = append(out, id)
		}
	}
	return out
}
