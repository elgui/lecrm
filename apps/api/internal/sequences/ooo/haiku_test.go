package ooo

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHaikuClassify_NoAPIKey(t *testing.T) {
	h := NewHaikuClassifier("")
	_, _, err := h.ClassifyOOO(context.Background(), ReplyBody{Subject: "x", Body: "y"})
	if !errors.Is(err, ErrNoAPIKey) {
		t.Fatalf("want ErrNoAPIKey, got %v", err)
	}
}

func TestHaikuClassify_Success(t *testing.T) {
	var gotBody []byte
	var gotHeaders http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/messages" {
			t.Errorf("path = %q, want /v1/messages", r.URL.Path)
		}
		gotHeaders = r.Header.Clone()
		gotBody, _ = io.ReadAll(r.Body)
		w.Header().Set("content-type", "application/json")
		_, _ = io.WriteString(w, `{"content":[{"type":"text","text":"{\"is_ooo\":true,\"confidence\":0.92}"}],"stop_reason":"end_turn"}`)
	}))
	defer srv.Close()

	h := NewHaikuClassifier("sk-test-key", WithBaseURL(srv.URL))
	isOOO, conf, err := h.ClassifyOOO(context.Background(), ReplyBody{
		From:    "Jane <jane@acme.fr>",
		Subject: "Re: Proposal",
		Body:    "Maybe — give me a few days.",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !isOOO {
		t.Error("isOOO = false, want true")
	}
	if conf != 0.92 {
		t.Errorf("confidence = %v, want 0.92", conf)
	}

	// Request-shape assertions: model pinned, output cap set, cache breakpoint on
	// the system block, auth headers present, and the rendered reply carried.
	if got := gotHeaders.Get("x-api-key"); got != "sk-test-key" {
		t.Errorf("x-api-key = %q, want sk-test-key", got)
	}
	if got := gotHeaders.Get("anthropic-version"); got != defaultAnthropicVersion {
		t.Errorf("anthropic-version = %q, want %q", got, defaultAnthropicVersion)
	}
	var req haikuReq
	if err := json.Unmarshal(gotBody, &req); err != nil {
		t.Fatalf("decode captured request: %v", err)
	}
	if req.Model != ModelHaiku {
		t.Errorf("model = %q, want %q", req.Model, ModelHaiku)
	}
	if req.MaxTokens != haikuMaxTokens {
		t.Errorf("max_tokens = %d, want %d", req.MaxTokens, haikuMaxTokens)
	}
	if len(req.System) != 1 || req.System[0].CacheControl == nil || req.System[0].CacheControl.Type != "ephemeral" {
		t.Errorf("system block must carry an ephemeral cache_control breakpoint, got %+v", req.System)
	}
	if len(req.Messages) != 1 || !strings.Contains(req.Messages[0].Content, "jane@acme.fr") {
		t.Errorf("user message must carry the rendered reply, got %+v", req.Messages)
	}
}

func TestHaikuClassify_ClampsConfidence(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"content":[{"type":"text","text":"{\"is_ooo\":false,\"confidence\":1.7}"}]}`)
	}))
	defer srv.Close()

	h := NewHaikuClassifier("sk-test-key", WithBaseURL(srv.URL))
	_, conf, err := h.ClassifyOOO(context.Background(), ReplyBody{Subject: "x", Body: "y"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if conf != 1.0 {
		t.Errorf("confidence = %v, want clamp to 1.0", conf)
	}
}

func TestHaikuClassify_Non200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = io.WriteString(w, `{"type":"error","error":{"type":"rate_limit_error"}}`)
	}))
	defer srv.Close()

	h := NewHaikuClassifier("sk-test-key", WithBaseURL(srv.URL))
	_, _, err := h.ClassifyOOO(context.Background(), ReplyBody{Subject: "x", Body: "y"})
	if err == nil {
		t.Fatal("expected an error on a 429 response")
	}
	if !strings.Contains(err.Error(), "429") {
		t.Errorf("error should mention the status code, got %v", err)
	}
}

func TestHaikuClassify_MalformedVerdict(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"content":[{"type":"text","text":"not json"}]}`)
	}))
	defer srv.Close()

	h := NewHaikuClassifier("sk-test-key", WithBaseURL(srv.URL))
	_, _, err := h.ClassifyOOO(context.Background(), ReplyBody{Subject: "x", Body: "y"})
	if err == nil {
		t.Fatal("expected an error when the model returns non-JSON")
	}
}

func TestHaikuClassify_SatisfiesSeam(t *testing.T) {
	var _ LLMClassifier = NewHaikuClassifier("k")
}
