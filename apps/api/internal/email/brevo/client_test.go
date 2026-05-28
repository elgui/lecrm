package brevo

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSend_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v3/smtp/email" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		if r.Header.Get("api-key") != "k_test" {
			t.Errorf("missing api-key header, got %q", r.Header.Get("api-key"))
		}
		body, _ := io.ReadAll(r.Body)
		var got SendRequest
		if err := json.Unmarshal(body, &got); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if got.Subject != "hello" || got.To[0].Email != "a@example.com" {
			t.Errorf("payload mismatch: %+v", got)
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"messageId":"<msg-1@brevo>"}`))
	}))
	defer srv.Close()

	c := New("k_test", srv.URL, nil)
	resp, err := c.Send(context.Background(), SendRequest{
		Sender:  Address{Email: "from@lecrm.eu"},
		To:      []Address{{Email: "a@example.com"}},
		Subject: "hello",
		TextContent: "world",
	})
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	if resp.MessageID != "<msg-1@brevo>" {
		t.Errorf("messageId: got %q want <msg-1@brevo>", resp.MessageID)
	}
}

func TestSend_EmptyAPIKey(t *testing.T) {
	c := New("", "", nil)
	_, err := c.Send(context.Background(), SendRequest{
		Sender: Address{Email: "x@y"},
		To:     []Address{{Email: "a@b"}}, Subject: "s",
	})
	if !errors.Is(err, ErrEmptyAPIKey) {
		t.Fatalf("want ErrEmptyAPIKey, got %v", err)
	}
}

func TestSend_RequiresRecipient(t *testing.T) {
	c := New("k", "https://example.invalid", nil)
	_, err := c.Send(context.Background(), SendRequest{
		Sender: Address{Email: "from@x"}, Subject: "s",
	})
	if err == nil || !strings.Contains(err.Error(), "To recipient") {
		t.Fatalf("want recipient error, got %v", err)
	}
}

func TestSend_APIError_TransientOn429(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"code":"too_many_requests","message":"slow down"}`))
	}))
	defer srv.Close()
	c := New("k", srv.URL, nil)
	_, err := c.Send(context.Background(), SendRequest{
		Sender: Address{Email: "x@y"}, To: []Address{{Email: "a@b"}}, Subject: "s",
	})
	var ae *APIError
	if !errors.As(err, &ae) {
		t.Fatalf("want *APIError, got %T %v", err, err)
	}
	if ae.Status != http.StatusTooManyRequests {
		t.Errorf("status: %d", ae.Status)
	}
	if !ae.IsTransient() {
		t.Errorf("429 should be transient")
	}
}

func TestSend_APIError_NonTransientOn400(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"code":"bad_request","message":"invalid sender"}`))
	}))
	defer srv.Close()
	c := New("k", srv.URL, nil)
	_, err := c.Send(context.Background(), SendRequest{
		Sender: Address{Email: "x@y"}, To: []Address{{Email: "a@b"}}, Subject: "s",
	})
	var ae *APIError
	if !errors.As(err, &ae) {
		t.Fatalf("want *APIError, got %v", err)
	}
	if ae.IsTransient() {
		t.Errorf("400 should NOT be transient")
	}
}

func TestVerifySignature_OK(t *testing.T) {
	secret := []byte("s3cr3t")
	body := []byte(`{"event":"delivered","email":"a@b","message-id":"<x>"}`)
	mac := hmac.New(sha256.New, secret)
	mac.Write(body)
	sig := hex.EncodeToString(mac.Sum(nil))
	if err := VerifySignature(secret, body, sig); err != nil {
		t.Fatalf("VerifySignature: %v", err)
	}
}

func TestVerifySignature_Tampered(t *testing.T) {
	secret := []byte("s3cr3t")
	body := []byte(`{"event":"delivered"}`)
	mac := hmac.New(sha256.New, secret)
	mac.Write(body)
	sig := hex.EncodeToString(mac.Sum(nil))
	tampered := append([]byte{}, body...)
	tampered[len(tampered)-1] = '!'
	if err := VerifySignature(secret, tampered, sig); !errors.Is(err, ErrInvalidSignature) {
		t.Fatalf("want ErrInvalidSignature, got %v", err)
	}
}

func TestVerifySignature_NotHex(t *testing.T) {
	if err := VerifySignature([]byte("k"), []byte("body"), "not-hex-zzz"); !errors.Is(err, ErrInvalidSignature) {
		t.Fatalf("want ErrInvalidSignature, got %v", err)
	}
}

func TestVerifySignature_EmptySecret(t *testing.T) {
	if err := VerifySignature(nil, []byte("body"), "00"); err == nil {
		t.Fatalf("want error for empty secret")
	}
}

func TestParseEvent_Known(t *testing.T) {
	raw := []byte(`{"event":"hardBounce","email":"x@y","message-id":"<m1>","date":"2026-05-28 10:00:00","reason":"550 5.1.1"}`)
	ev, err := ParseEvent(raw)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if ev.Event != EventHardBounce {
		t.Errorf("event: %q", ev.Event)
	}
	if !ev.IsKnown() {
		t.Errorf("hardBounce should be known")
	}
	if reason, ok := ev.SuppressionReason(); !ok || reason != "hard_bounce" {
		t.Errorf("suppression: %q %v", reason, ok)
	}
	if !ev.IsBounceLike() {
		t.Errorf("hardBounce IsBounceLike should be true")
	}
	if ev.Date.IsZero() {
		t.Errorf("date should be parsed")
	}
}

func TestParseEvent_Unknown(t *testing.T) {
	raw := []byte(`{"event":"some_new_event","email":"x@y"}`)
	ev, err := ParseEvent(raw)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if ev.IsKnown() {
		t.Errorf("some_new_event should NOT be known")
	}
	if _, ok := ev.SuppressionReason(); ok {
		t.Errorf("unknown event should not produce suppression reason")
	}
}

func TestParseEvent_DeliveredDoesNotSuppress(t *testing.T) {
	ev, err := ParseEvent([]byte(`{"event":"delivered","email":"x@y"}`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if _, ok := ev.SuppressionReason(); ok {
		t.Errorf("delivered should not suppress")
	}
	if ev.IsBounceLike() {
		t.Errorf("delivered is not bounce-like")
	}
}

func TestParseEvent_MissingEventField(t *testing.T) {
	_, err := ParseEvent([]byte(`{"email":"x@y"}`))
	if err == nil {
		t.Fatalf("want error on missing event")
	}
}
