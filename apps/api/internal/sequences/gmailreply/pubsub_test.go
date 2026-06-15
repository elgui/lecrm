package gmailreply

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"testing"
)

// encodePush builds a Pub/Sub push body wrapping the given Gmail notification
// JSON (already a string) base64-encoded in message.data, like Pub/Sub delivers.
func encodePush(t *testing.T, dataJSON string, std bool) []byte {
	t.Helper()
	var enc string
	if std {
		enc = base64.StdEncoding.EncodeToString([]byte(dataJSON))
	} else {
		enc = base64.RawURLEncoding.EncodeToString([]byte(dataJSON))
	}
	body, err := json.Marshal(map[string]any{
		"message": map[string]any{
			"data":      enc,
			"messageId": "123",
		},
		"subscription": "projects/lecrm-prod/subscriptions/gmail-inbox-push",
	})
	if err != nil {
		t.Fatalf("marshal push: %v", err)
	}
	return body
}

func TestParsePushBody_ValidStdBase64(t *testing.T) {
	body := encodePush(t, `{"emailAddress":"rep@example.com","historyId":4242}`, true)
	n, err := ParsePushBody(body)
	if err != nil {
		t.Fatalf("ParsePushBody: %v", err)
	}
	if n.EmailAddress != "rep@example.com" {
		t.Errorf("email = %q, want rep@example.com", n.EmailAddress)
	}
	if n.HistoryID != 4242 {
		t.Errorf("historyId = %d, want 4242", n.HistoryID)
	}
}

func TestParsePushBody_AcceptsRawURLBase64(t *testing.T) {
	body := encodePush(t, `{"emailAddress":"rep@example.com","historyId":7}`, false)
	n, err := ParsePushBody(body)
	if err != nil {
		t.Fatalf("ParsePushBody (rawurl): %v", err)
	}
	if n.HistoryID != 7 {
		t.Errorf("historyId = %d, want 7", n.HistoryID)
	}
}

func TestParsePushBody_Errors(t *testing.T) {
	cases := []struct {
		name string
		body []byte
	}{
		{"not json", []byte("not-json")},
		{"empty data", []byte(`{"message":{"data":""}}`)},
		{"bad base64", []byte(`{"message":{"data":"!!!not-base64!!!"}}`)},
		{"inner not json", encodePush(t, `not-json`, true)},
		{"missing email", encodePush(t, `{"historyId":9}`, true)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := ParsePushBody(tc.body); err == nil {
				t.Fatalf("expected error, got nil")
			} else if !errors.Is(err, ErrBadPushBody) {
				t.Fatalf("error %v is not ErrBadPushBody", err)
			}
		})
	}
}
