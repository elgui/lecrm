package gmailreply

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
)

// ErrBadPushBody is returned when a Pub/Sub push body cannot be parsed into a
// Gmail notification. Handlers map it to 400 — a malformed body is the sender's
// error, and Pub/Sub should NOT redeliver it (a redelivery would just fail the
// same way), so the handler still acks (drops) rather than 5xx-looping.
var ErrBadPushBody = errors.New("gmailreply: malformed Pub/Sub push body")

// pushEnvelope is the outer JSON Pub/Sub wraps every push delivery in
// (https://cloud.google.com/pubsub/docs/push#receive_push). The handler reads
// Message.Data (base64) and decodes it into a Notification.
type pushEnvelope struct {
	Message struct {
		// Data is the base64-encoded message payload — for Gmail watch it is a
		// JSON {emailAddress, historyId} blob (the Notification below).
		Data string `json:"data"`
		// MessageID and PublishTime are Pub/Sub envelope metadata, retained for
		// logging/debugging but not used for correlation.
		MessageID   string `json:"messageId"`
		PublishTime string `json:"publishTime"`
	} `json:"message"`
	Subscription string `json:"subscription"`
}

// Notification is the Gmail watch payload carried (base64-encoded) inside a
// Pub/Sub push (https://developers.google.com/workspace/gmail/api/guides/push).
// It identifies the mailbox and the mailbox-level history cursor — never an
// individual message — which is exactly why the worker must call history.list
// to discover what changed.
type Notification struct {
	// EmailAddress is the mailbox that changed; the push handler resolves it to
	// a (workspace, user) via ConnectionResolver.
	EmailAddress string `json:"emailAddress"`
	// HistoryID is the mailbox's current history id at publish time. It is
	// advisory: the worker scans from the *persisted* cursor (which may be
	// older, e.g. after downtime), so no events are skipped between pushes.
	HistoryID uint64 `json:"historyId"`
}

// ParsePushBody decodes a Pub/Sub push request body into the Gmail
// Notification it carries. It validates the envelope shape, the base64
// encoding, the inner JSON, and that a non-empty emailAddress is present —
// returning ErrBadPushBody (wrapped) on any failure so the caller can map a
// single sentinel to 400.
func ParsePushBody(body []byte) (Notification, error) {
	var env pushEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		return Notification{}, fmt.Errorf("%w: envelope: %v", ErrBadPushBody, err)
	}
	if env.Message.Data == "" {
		return Notification{}, fmt.Errorf("%w: empty message.data", ErrBadPushBody)
	}
	// Pub/Sub uses standard base64 (with padding) for message.data.
	raw, err := base64.StdEncoding.DecodeString(env.Message.Data)
	if err != nil {
		// Some clients emit unpadded base64url; accept it as a fallback before
		// giving up so a benign encoding choice doesn't drop real events.
		raw, err = base64.RawURLEncoding.DecodeString(env.Message.Data)
		if err != nil {
			return Notification{}, fmt.Errorf("%w: message.data not base64", ErrBadPushBody)
		}
	}

	var n Notification
	if err := json.Unmarshal(raw, &n); err != nil {
		return Notification{}, fmt.Errorf("%w: notification json: %v", ErrBadPushBody, err)
	}
	if n.EmailAddress == "" {
		return Notification{}, fmt.Errorf("%w: notification missing emailAddress", ErrBadPushBody)
	}
	return n, nil
}
