package ooo

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// ModelHaiku is the model the stage-2 fallback runs on (ADR-005 model selection:
// the cheapest model that clears the OOO classification bar). Pinned to the
// dated snapshot so a silent model rev never shifts classifier behaviour.
const ModelHaiku = "claude-haiku-4-5-20251001"

const (
	defaultAnthropicBaseURL = "https://api.anthropic.com"
	defaultAnthropicVersion = "2023-06-01"
	defaultHaikuTimeout     = 12 * time.Second
	// haikuMaxTokens caps output. The model returns a one-line JSON verdict, so a
	// tight cap both bounds cost and fails fast on a runaway response.
	haikuMaxTokens = 64
)

// ErrNoAPIKey is returned by ClassifyOOO when the classifier was built without
// an Anthropic API key. The wiring layer should not register a keyless Haiku
// stage — running rules-only (nil LLMClassifier) is the correct keyless config.
var ErrNoAPIKey = errors.New("ooo: anthropic api key not configured")

// HaikuClassifier is the stage-2 LLMClassifier (ADR-004 rev 2 §5): a single
// Anthropic Messages call that decides OOO-vs-reply for the ambiguous tail the
// rules cannot call. It uses the standard library only — the call surface is one
// request, so it carries no SDK dependency; the seam (LLMClassifier) keeps it
// swappable for the official SDK later.
//
// Cost (ADR-004 rev 2 §5, ceiling ~$0.50/mo at 10k replies): only the ambiguous
// tail reaches Haiku (the rules absorb ~95%), each call is a few hundred input
// tokens plus a one-line JSON output. The stable system prompt carries a
// cache_control breakpoint; note Haiku 4.5's minimum cacheable prefix is ~4096
// tokens, so caching only engages once the prompt (e.g. with few-shot examples)
// crosses that floor — the breakpoint is set now so growth is free, not because
// today's short prompt caches.
type HaikuClassifier struct {
	apiKey     string
	model      string
	baseURL    string
	apiVersion string
	httpClient *http.Client
}

// HaikuOption configures a HaikuClassifier.
type HaikuOption func(*HaikuClassifier)

// WithModel overrides the model id (default ModelHaiku).
func WithModel(model string) HaikuOption {
	return func(h *HaikuClassifier) {
		if model != "" {
			h.model = model
		}
	}
}

// WithBaseURL overrides the Anthropic API base URL (tests point it at httptest).
func WithBaseURL(base string) HaikuOption {
	return func(h *HaikuClassifier) {
		if base != "" {
			h.baseURL = strings.TrimRight(base, "/")
		}
	}
}

// WithHTTPClient overrides the HTTP client (timeouts, transport, test doubles).
func WithHTTPClient(c *http.Client) HaikuOption {
	return func(h *HaikuClassifier) {
		if c != nil {
			h.httpClient = c
		}
	}
}

// NewHaikuClassifier builds the stage-2 fallback. apiKey is the Anthropic key;
// an empty key yields a classifier whose ClassifyOOO returns ErrNoAPIKey.
func NewHaikuClassifier(apiKey string, opts ...HaikuOption) *HaikuClassifier {
	h := &HaikuClassifier{
		apiKey:     apiKey,
		model:      ModelHaiku,
		baseURL:    defaultAnthropicBaseURL,
		apiVersion: defaultAnthropicVersion,
		httpClient: &http.Client{Timeout: defaultHaikuTimeout},
	}
	for _, opt := range opts {
		opt(h)
	}
	return h
}

// haikuSystemPrompt is the stable (cache-eligible) classifier instruction. It is
// frozen content — no timestamps or per-request data — so the cache_control
// breakpoint below stays valid across requests.
const haikuSystemPrompt = `You classify a single inbound email reply as either a genuine human reply or an automated out-of-office / vacation auto-responder. The email may be in French or English.

An out-of-office reply is one the recipient's mail system sent automatically because they are away (vacation, leave, travel, parental/sick leave) — typically announcing an absence and often a return date. A genuine reply is one a person wrote, even a terse one ("not interested", "please call me", "merci, je regarde").

Respond with ONLY a JSON object, no prose:
{"is_ooo": <true|false>, "confidence": <number between 0 and 1>}`

// classify request/response shapes — minimal subset of the Messages API.
type haikuReq struct {
	Model        string          `json:"model"`
	MaxTokens    int             `json:"max_tokens"`
	System       []haikuSysBlock `json:"system"`
	Messages     []haikuMessage  `json:"messages"`
	OutputConfig *haikuOutputCfg `json:"output_config,omitempty"`
}

type haikuSysBlock struct {
	Type         string            `json:"type"`
	Text         string            `json:"text"`
	CacheControl *haikuCacheCtrl   `json:"cache_control,omitempty"`
}

type haikuCacheCtrl struct {
	Type string `json:"type"`
}

type haikuMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type haikuOutputCfg struct {
	Format haikuFormat `json:"format"`
}

type haikuFormat struct {
	Type   string         `json:"type"`
	Schema map[string]any `json:"schema"`
}

type haikuResp struct {
	Content    []haikuContentBlock `json:"content"`
	StopReason string              `json:"stop_reason"`
}

type haikuContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// verdictPayload is the JSON the model is constrained to return.
type verdictPayload struct {
	IsOOO      bool    `json:"is_ooo"`
	Confidence float64 `json:"confidence"`
}

// ClassifyOOO asks Haiku whether the reply is an out-of-office auto-responder.
func (h *HaikuClassifier) ClassifyOOO(ctx context.Context, body ReplyBody) (bool, Confidence, error) {
	if h.apiKey == "" {
		return false, 0, ErrNoAPIKey
	}

	reqBody := haikuReq{
		Model:     h.model,
		MaxTokens: haikuMaxTokens,
		System: []haikuSysBlock{{
			Type:         "text",
			Text:         haikuSystemPrompt,
			CacheControl: &haikuCacheCtrl{Type: "ephemeral"},
		}},
		Messages: []haikuMessage{{Role: "user", Content: renderReply(body)}},
		OutputConfig: &haikuOutputCfg{Format: haikuFormat{
			Type: "json_schema",
			Schema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"is_ooo":     map[string]any{"type": "boolean"},
					"confidence": map[string]any{"type": "number"},
				},
				"required":             []string{"is_ooo", "confidence"},
				"additionalProperties": false,
			},
		}},
	}

	raw, err := json.Marshal(reqBody)
	if err != nil {
		return false, 0, fmt.Errorf("ooo: marshal haiku request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, h.baseURL+"/v1/messages", bytes.NewReader(raw))
	if err != nil {
		return false, 0, fmt.Errorf("ooo: build haiku request: %w", err)
	}
	req.Header.Set("content-type", "application/json")
	req.Header.Set("x-api-key", h.apiKey)
	req.Header.Set("anthropic-version", h.apiVersion)

	resp, err := h.httpClient.Do(req)
	if err != nil {
		return false, 0, fmt.Errorf("ooo: haiku request: %w", err)
	}
	defer resp.Body.Close()

	respRaw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	if err != nil {
		return false, 0, fmt.Errorf("ooo: read haiku response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return false, 0, fmt.Errorf("ooo: haiku status %d: %s", resp.StatusCode, strings.TrimSpace(string(respRaw)))
	}

	var parsed haikuResp
	if err := json.Unmarshal(respRaw, &parsed); err != nil {
		return false, 0, fmt.Errorf("ooo: decode haiku response: %w", err)
	}

	text := firstText(parsed.Content)
	if text == "" {
		return false, 0, fmt.Errorf("ooo: haiku returned no text block (stop_reason=%q)", parsed.StopReason)
	}

	var v verdictPayload
	if err := json.Unmarshal([]byte(text), &v); err != nil {
		return false, 0, fmt.Errorf("ooo: haiku verdict not json: %w (got %q)", err, truncate(text, 120))
	}
	return v.IsOOO, clampConfidence(v.Confidence), nil
}

// renderReply formats the minimised reply for the model. Only From/Subject/the
// snippet are sent (ADR-009 §8.3 — never the full body).
func renderReply(body ReplyBody) string {
	var b strings.Builder
	b.WriteString("From: ")
	b.WriteString(body.From)
	b.WriteString("\nSubject: ")
	b.WriteString(body.Subject)
	b.WriteString("\n\n")
	b.WriteString(body.Body)
	return b.String()
}

func firstText(blocks []haikuContentBlock) string {
	for _, b := range blocks {
		if b.Type == "text" && strings.TrimSpace(b.Text) != "" {
			return strings.TrimSpace(b.Text)
		}
	}
	return ""
}

func clampConfidence(c float64) Confidence {
	switch {
	case c < 0:
		return 0
	case c > 1:
		return 1
	default:
		return Confidence(c)
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

// Compile-time proof the Haiku classifier satisfies the stage-2 seam.
var _ LLMClassifier = (*HaikuClassifier)(nil)
