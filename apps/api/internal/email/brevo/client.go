// Package brevo is a hand-rolled HTTP client for the subset of the Brevo
// (ex-Sendinblue) Transactional Email API that leCRM v0 consumes.
//
// Scope (per ADR-003 and the Sprint-11 Track B tasket):
//   - POST /v3/smtp/email — send a single transactional email.
//   - Webhook signature verification — HMAC-SHA256 over the raw body
//     using the per-workspace webhook signing secret.
//
// Out of scope for this package: sequences, contact-list management,
// domain-authentication API. Those land in v1 work.
//
// The package is intentionally dependency-free at the API surface: it
// returns plain Go types and concrete error values, so the email service
// layer can wrap it behind an EmailProvider interface (ADR-003
// "Consequences > Neutral") without leaking Brevo-specific types.
package brevo

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

// DefaultBaseURL is the production Brevo API endpoint. The client accepts
// a different value for tests (httptest.Server).
const DefaultBaseURL = "https://api.brevo.com"

// Client is a Brevo HTTP client. Construct with New.
type Client struct {
	apiKey  string
	baseURL string
	http    *http.Client
}

// New returns a Client. apiKey is the workspace's Brevo API key (carried
// in the `api-key` header per Brevo docs). baseURL defaults to
// DefaultBaseURL when empty. httpClient defaults to one with a 10 s
// timeout when nil.
func New(apiKey, baseURL string, httpClient *http.Client) *Client {
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 10 * time.Second}
	}
	return &Client{apiKey: apiKey, baseURL: baseURL, http: httpClient}
}

// Address is a single recipient or sender. Name is optional.
type Address struct {
	Email string `json:"email"`
	Name  string `json:"name,omitempty"`
}

// SendRequest is the body for POST /v3/smtp/email. Only the fields leCRM
// v0 needs are modelled; the rest of Brevo's payload (attachments,
// templates, replyTo, headers) can be added later without breaking
// callers.
type SendRequest struct {
	Sender      Address           `json:"sender"`
	To          []Address         `json:"to"`
	Cc          []Address         `json:"cc,omitempty"`
	Bcc         []Address         `json:"bcc,omitempty"`
	Subject     string            `json:"subject"`
	HTMLContent string            `json:"htmlContent,omitempty"`
	TextContent string            `json:"textContent,omitempty"`
	ReplyTo     *Address          `json:"replyTo,omitempty"`
	Headers     map[string]string `json:"headers,omitempty"`
	Tags        []string          `json:"tags,omitempty"`
}

// SendResponse is the parsed response from a successful send.
type SendResponse struct {
	MessageID string `json:"messageId"`
}

// APIError is the shape of Brevo's JSON error responses (4xx and 5xx).
type APIError struct {
	Status  int    `json:"-"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *APIError) Error() string {
	return fmt.Sprintf("brevo: %d %s: %s", e.Status, e.Code, e.Message)
}

// IsTransient returns true when the API error is plausibly worth retrying.
// 429 (rate limit) and 5xx are transient; 4xx other than 429 are not.
func (e *APIError) IsTransient() bool {
	return e.Status == http.StatusTooManyRequests || e.Status >= 500
}

// ErrEmptyAPIKey is returned by Send when no API key was configured. We
// surface this as an explicit error rather than letting Brevo return 401,
// because the v0 wiring may legitimately run with an empty key in dev.
var ErrEmptyAPIKey = errors.New("brevo: api key not configured")

// Send posts req to /v3/smtp/email and returns the parsed messageId.
// On non-2xx the returned error is *APIError; callers can type-assert to
// inspect IsTransient() for retry decisions.
func (c *Client) Send(ctx context.Context, req SendRequest) (SendResponse, error) {
	if c.apiKey == "" {
		return SendResponse{}, ErrEmptyAPIKey
	}
	if len(req.To) == 0 {
		return SendResponse{}, fmt.Errorf("brevo: send: at least one To recipient required")
	}
	if req.Sender.Email == "" {
		return SendResponse{}, fmt.Errorf("brevo: send: sender email required")
	}

	body, err := json.Marshal(req)
	if err != nil {
		return SendResponse{}, fmt.Errorf("brevo: marshal send request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.baseURL+"/v3/smtp/email", bytes.NewReader(body))
	if err != nil {
		return SendResponse{}, fmt.Errorf("brevo: build send request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")
	httpReq.Header.Set("api-key", c.apiKey)

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return SendResponse{}, fmt.Errorf("brevo: send http: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return SendResponse{}, fmt.Errorf("brevo: read send response: %w", err)
	}

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		var out SendResponse
		if err := json.Unmarshal(raw, &out); err != nil {
			return SendResponse{}, fmt.Errorf("brevo: parse send response: %w", err)
		}
		return out, nil
	}

	apiErr := &APIError{Status: resp.StatusCode}
	_ = json.Unmarshal(raw, apiErr)
	if apiErr.Message == "" {
		apiErr.Message = string(raw)
	}
	return SendResponse{}, apiErr
}
