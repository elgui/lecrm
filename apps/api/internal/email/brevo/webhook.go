package brevo

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// EventType is a Brevo transactional webhook event name. The set leCRM
// consumes is fixed per ADR-003 §Decision (webhook event handling).
type EventType string

const (
	EventDelivered    EventType = "delivered"
	EventHardBounce   EventType = "hardBounce"
	EventSoftBounce   EventType = "softBounce"
	EventBlocked      EventType = "blocked"
	EventSpam         EventType = "spam"
	EventUnsubscribed EventType = "unsubscribed"
)

// AllEventTypes returns the canonical inbound event list. Webhook
// payloads carrying any other event type are rejected upstream — the
// service treats unknown events as a misconfiguration, not as silently
// ignored.
func AllEventTypes() []EventType {
	return []EventType{
		EventDelivered, EventHardBounce, EventSoftBounce,
		EventBlocked, EventSpam, EventUnsubscribed,
	}
}

// Event is the parsed inbound webhook payload. Brevo sends one JSON
// object per event (not an array). Field names match Brevo's documented
// schema; everything is captured as RawExtras for forward-compat.
type Event struct {
	Event       EventType       `json:"event"`
	Email       string          `json:"email"`
	MessageID   string          `json:"message-id"`
	Date        time.Time       `json:"-"`
	DateRaw     string          `json:"date"`
	Subject     string          `json:"subject,omitempty"`
	Tag         string          `json:"tag,omitempty"`
	Reason      string          `json:"reason,omitempty"`
	RawExtras   json.RawMessage `json:"-"`
	rawDateUsed bool
}

// ParseEvent unmarshals raw into Event and parses Date from DateRaw using
// the RFC3339-with-zone format Brevo sends.
func ParseEvent(raw []byte) (Event, error) {
	var ev Event
	if err := json.Unmarshal(raw, &ev); err != nil {
		return Event{}, fmt.Errorf("brevo: parse event: %w", err)
	}
	if ev.Event == "" {
		return Event{}, errors.New("brevo: event payload missing 'event' field")
	}
	if ev.DateRaw != "" {
		// Brevo uses "2024-05-28 10:00:00" in UTC by default, with some
		// payloads including a zone offset. Try both.
		for _, layout := range []string{time.RFC3339, "2006-01-02 15:04:05"} {
			if t, err := time.Parse(layout, ev.DateRaw); err == nil {
				ev.Date = t
				ev.rawDateUsed = true
				break
			}
		}
	}
	ev.RawExtras = append([]byte(nil), raw...)
	return ev, nil
}

// IsKnown returns true if e.Event is one of the canonical event types.
// The service rejects unknown events to surface schema drift.
func (e Event) IsKnown() bool {
	for _, k := range AllEventTypes() {
		if e.Event == k {
			return true
		}
	}
	return false
}

// SuppressionReason maps a Brevo event to the suppression-table reason
// string. Returns ("", false) for events that should NOT suppress (e.g.
// `delivered`, `softBounce` — see Brevo + RFC 5321 best practice).
func (e Event) SuppressionReason() (string, bool) {
	switch e.Event {
	case EventHardBounce:
		return "hard_bounce", true
	case EventBlocked:
		return "blocked", true
	case EventSpam:
		return "complaint", true
	case EventUnsubscribed:
		return "unsubscribed", true
	default:
		return "", false
	}
}

// IsBounceLike reports whether the event counts toward the rolling
// bounce-rate alarm (ADR-003 §Mitigations item 4). Hard bounces and spam
// complaints count; soft bounces and blocked don't.
func (e Event) IsBounceLike() bool {
	return e.Event == EventHardBounce || e.Event == EventSpam
}

// ErrInvalidSignature is returned by VerifySignature when the HMAC check
// fails. Handlers MUST surface 401 on this error — not 403 — so the
// caller distinguishes config error (wrong secret) from authorization
// error (caller not allowed).
var ErrInvalidSignature = errors.New("brevo: invalid webhook signature")

// VerifySignature checks that `signature` is a valid hex-encoded
// HMAC-SHA256 of `body` under `secret`. Brevo signs the raw request body
// and delivers the digest in the `X-Sib-Webhook-Signature` header
// (header name fetched via SignatureHeader()).
//
// Uses hmac.Equal for constant-time comparison; the function is safe to
// call from request handlers without leaking timing information about
// the secret.
func VerifySignature(secret, body []byte, signatureHex string) error {
	if len(secret) == 0 {
		return errors.New("brevo: webhook secret not configured")
	}
	want, err := hex.DecodeString(signatureHex)
	if err != nil {
		return fmt.Errorf("%w: not hex", ErrInvalidSignature)
	}
	mac := hmac.New(sha256.New, secret)
	mac.Write(body)
	got := mac.Sum(nil)
	if !hmac.Equal(want, got) {
		return ErrInvalidSignature
	}
	return nil
}

// SignatureHeader is the HTTP header Brevo uses to deliver the webhook
// HMAC digest. Centralised so the handler and any tests cannot drift.
const SignatureHeader = "X-Sib-Webhook-Signature"
