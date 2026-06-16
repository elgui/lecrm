// Package email orchestrates transactional email sends via Brevo
// (ADR-003) within the leCRM workspace-scoped data model (ADR-001,
// ADR-009).
//
// The service layer is the single place that:
//   - pre-flight checks the per-workspace email_suppression table,
//   - emits the audit-log event with the service token's actor_type
//     (per ADR-007 §3 and ADR-009 §4.1),
//   - decides whether to send synchronously or hand off to a river job
//     (per ADR-009 §8.3),
//   - on inbound webhook events, upserts the suppression table and
//     evaluates the bounce-rate alarm.
//
// The Brevo HTTP client (apps/api/internal/email/brevo) is intentionally
// dumb — it speaks HTTP and HMAC and nothing else. All policy is here.
package email

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gbconsult/lecrm/apps/api/internal/email/brevo"
	"github.com/gbconsult/lecrm/apps/api/internal/jobs"
)

// ActorType matches the core.audit_log.actor_type CHECK constraint
// (packages/db/migrations/0001_init.sql) and the ADR-009 §4.1 service
// token claim. The email send path accepts only the two values that are
// meaningful for an outbound email request — human_api (a logged-in
// admin's session token) or internal_service (a CRM workflow firing on
// behalf of the workspace).
type ActorType string

const (
	ActorHumanAPI         ActorType = "human_api"
	ActorInternalService  ActorType = "internal_service"
)

// IsValidSendActor returns true when at is one of the two actor types
// allowed on the outbound send path. Other audit actor_types
// (mcp_agent, system) are explicitly rejected — those have different
// authorization and rate-limit rules out of scope here.
func IsValidSendActor(at ActorType) bool {
	return at == ActorHumanAPI || at == ActorInternalService
}

// BounceRateWindow is the rolling window for the complaint-rate alarm
// per ADR-003 §Mitigations item 4.
const BounceRateWindow = 7 * 24 * time.Hour

// BounceRateThreshold is the complaint-rate ceiling: above this, the
// workspace's pending sends are paused and a security event is emitted.
const BounceRateThreshold = 0.001 // 0.1%

// MinSamplesForAlarm guards against the divide-by-N noise problem: with
// only a handful of sends, a single bounce trips the alarm. ADR-003
// states "complaint-rate >0.1% over rolling 7 days" — we additionally
// require at least 100 sends in the window before alarming.
const MinSamplesForAlarm = 100

// SendRequest is the workspace-scoped view of an outbound transactional
// email. The service layer translates this into a brevo.SendRequest plus
// the audit + suppression bookkeeping.
type SendRequest struct {
	WorkspaceID uuid.UUID
	Schema      string
	ActorType   ActorType
	ActorUserID uuid.UUID // zero when ActorInternalService
	RequestID   uuid.UUID
	From        brevo.Address
	To          []brevo.Address
	Subject     string
	HTMLContent string
	TextContent string
	Tags        []string
}

// SendOutcome describes what happened. When Skipped is true the email
// was not handed to Brevo because at least one recipient was on the
// suppression list (or because the bounce-rate alarm has paused sends).
type SendOutcome struct {
	MessageID         string
	Skipped           bool
	SkippedRecipients []string
	Reason            string
}

// AuditWriter inserts an audit_log row. Wrapping this in an interface
// keeps Service unit-testable without a real Postgres handle.
type AuditWriter interface {
	WriteAudit(ctx context.Context, event AuditEvent) error
}

// AuditEvent is the minimal payload for an ADR-007 §3 entry. The
// payload field carries event-specific data (recipient counts, message
// id, suppression hit list, etc).
type AuditEvent struct {
	Event       string
	WorkspaceID uuid.UUID
	ActorType   ActorType
	ActorUserID uuid.UUID
	RequestID   uuid.UUID
	Payload     map[string]any
}

// SuppressionStore is the per-workspace suppression-list view. The Pg
// implementation lives in suppression.go; tests substitute a map-backed
// fake.
type SuppressionStore interface {
	Suppressed(ctx context.Context, schema string, emails []string) (map[string]bool, error)
	Upsert(ctx context.Context, schema, email, reason string, at time.Time) error
}

// BounceRateChecker reports the rolling-window bounce-rate stats for a
// workspace. The Pg implementation sits over the suppression table (or
// a dedicated email_event_log — left as a v0 simplification using
// suppression-event timestamps + an event-count snapshot).
type BounceRateChecker interface {
	Stats(ctx context.Context, schema string, window time.Duration) (BounceStats, error)
}

// BounceStats captures the rolling window state used by the alarm.
type BounceStats struct {
	TotalSends    int64
	BounceLikeN   int64 // hardBounce + spam events in window
	Rate          float64
	PauseInEffect bool
}

// Provider is the outbound mailer interface. Production wires the brevo
// Client; tests use a stub.
type Provider interface {
	Send(ctx context.Context, req brevo.SendRequest) (brevo.SendResponse, error)
}

// Service is the high-level orchestrator.
type Service struct {
	Provider     Provider
	Suppression  SuppressionStore
	Audit        AuditWriter
	BounceRate   BounceRateChecker
	JobRunner    jobs.JobRunner
	Logger       *slog.Logger
}

// ErrPausedByAlarm is returned when the workspace has tripped the
// bounce-rate alarm and is in the paused state. Callers should surface
// this as 503 with a Retry-After hint, not 500.
var ErrPausedByAlarm = errors.New("email: workspace sends paused (bounce-rate alarm)")

// ErrInvalidActor is returned when the caller's service-token actor_type
// is not one of the two allowed for the email send path.
var ErrInvalidActor = errors.New("email: actor_type not allowed for email send")

// Send executes the full outbound flow: alarm check → suppression
// filter → provider send → audit log. The audit row is written
// unconditionally, even when every recipient was suppressed (the
// "skipped" outcome is itself an auditable event per ADR-007 §3).
func (s *Service) Send(ctx context.Context, req SendRequest) (SendOutcome, error) {
	if !IsValidSendActor(req.ActorType) {
		return SendOutcome{}, ErrInvalidActor
	}
	if s.BounceRate != nil {
		stats, err := s.BounceRate.Stats(ctx, req.Schema, BounceRateWindow)
		if err != nil {
			return SendOutcome{}, fmt.Errorf("email: bounce-rate check: %w", err)
		}
		if stats.PauseInEffect {
			_ = s.writeAudit(ctx, "email.send.skipped", req, map[string]any{
				"reason":       "paused_by_alarm",
				"recipients":   addressEmails(req.To),
				"bounce_rate":  stats.Rate,
			})
			return SendOutcome{Skipped: true, Reason: "paused_by_alarm"}, ErrPausedByAlarm
		}
	}

	emails := addressEmails(req.To)
	suppressed, err := s.Suppression.Suppressed(ctx, req.Schema, emails)
	if err != nil {
		return SendOutcome{}, fmt.Errorf("email: suppression check: %w", err)
	}

	var allowed []brevo.Address
	var skipped []string
	for _, addr := range req.To {
		if suppressed[addr.Email] {
			skipped = append(skipped, addr.Email)
			continue
		}
		allowed = append(allowed, addr)
	}

	if len(allowed) == 0 {
		_ = s.writeAudit(ctx, "email.send.skipped", req, map[string]any{
			"reason":              "all_recipients_suppressed",
			"skipped_recipients":  skipped,
		})
		return SendOutcome{Skipped: true, Reason: "all_recipients_suppressed", SkippedRecipients: skipped}, nil
	}

	brevoReq := brevo.SendRequest{
		Sender:      req.From,
		To:          allowed,
		Subject:     req.Subject,
		HTMLContent: req.HTMLContent,
		TextContent: req.TextContent,
		Tags:        req.Tags,
	}
	resp, err := s.Provider.Send(ctx, brevoReq)
	if err != nil {
		_ = s.writeAudit(ctx, "email.send.failed", req, map[string]any{
			"error":              err.Error(),
			"recipients":         addressEmails(allowed),
			"skipped_recipients": skipped,
		})
		return SendOutcome{}, err
	}

	if err := s.writeAudit(ctx, "email.send.requested", req, map[string]any{
		"message_id":          resp.MessageID,
		"recipients":          addressEmails(allowed),
		"skipped_recipients":  skipped,
	}); err != nil {
		return SendOutcome{}, fmt.Errorf("email: audit emit: %w", err)
	}

	return SendOutcome{MessageID: resp.MessageID, SkippedRecipients: skipped}, nil
}

// IngestEvent processes one parsed Brevo webhook event for one workspace:
//  1. If the event suppresses (hard bounce, spam, blocked, unsubscribed),
//     upsert email_suppression.
//  2. Always emit `email.event.received` audit row.
//  3. Re-check bounce-rate and emit `security.email_bounce_rate_high`
//     when threshold crossed (the rate-check helper persists the
//     "paused" flag).
func (s *Service) IngestEvent(ctx context.Context, workspaceID uuid.UUID, schema string, ev brevo.Event) error {
	if !ev.IsKnown() {
		return fmt.Errorf("email: unknown brevo event type %q", ev.Event)
	}

	if reason, ok := ev.SuppressionReason(); ok {
		at := ev.Date
		if at.IsZero() {
			at = time.Now().UTC()
		}
		if err := s.Suppression.Upsert(ctx, schema, ev.Email, reason, at); err != nil {
			return fmt.Errorf("email: suppression upsert: %w", err)
		}
	}

	if err := s.Audit.WriteAudit(ctx, AuditEvent{
		Event:       "email.event.received",
		WorkspaceID: workspaceID,
		ActorType:   ActorInternalService,
		Payload: map[string]any{
			"event":      string(ev.Event),
			"email":      ev.Email,
			"message_id": ev.MessageID,
			"reason":     ev.Reason,
		},
	}); err != nil {
		return fmt.Errorf("email: audit event: %w", err)
	}

	if ev.IsBounceLike() && s.BounceRate != nil {
		stats, err := s.BounceRate.Stats(ctx, schema, BounceRateWindow)
		if err != nil {
			return fmt.Errorf("email: bounce-rate post-event check: %w", err)
		}
		if ShouldTripAlarm(stats) {
			if err := s.Audit.WriteAudit(ctx, AuditEvent{
				Event:       "security.email_bounce_rate_high",
				WorkspaceID: workspaceID,
				ActorType:   ActorInternalService,
				Payload: map[string]any{
					"total_sends":   stats.TotalSends,
					"bounce_count":  stats.BounceLikeN,
					"rate":          stats.Rate,
					"threshold":     BounceRateThreshold,
					"window_hours":  int(BounceRateWindow.Hours()),
				},
			}); err != nil {
				return fmt.Errorf("email: alarm audit: %w", err)
			}
		}
	}

	return nil
}

// ShouldTripAlarm encodes the alarm policy in a pure function so it can
// be tested without any DB. The rule: window must hold at least
// MinSamplesForAlarm sends, and the bounce-like rate must exceed
// BounceRateThreshold. Both halves are required — a 100% rate over 3
// sends is NOT alarm-worthy (noise floor).
func ShouldTripAlarm(s BounceStats) bool {
	if s.TotalSends < MinSamplesForAlarm {
		return false
	}
	return s.Rate > BounceRateThreshold
}

func (s *Service) writeAudit(ctx context.Context, event string, req SendRequest, payload map[string]any) error {
	return s.Audit.WriteAudit(ctx, AuditEvent{
		Event:       event,
		WorkspaceID: req.WorkspaceID,
		ActorType:   req.ActorType,
		ActorUserID: req.ActorUserID,
		RequestID:   req.RequestID,
		Payload:     payload,
	})
}

func addressEmails(addrs []brevo.Address) []string {
	out := make([]string, len(addrs))
	for i, a := range addrs {
		out[i] = a.Email
	}
	return out
}

// PgAuditWriter writes audit_log rows to core.audit_log via a pool.
type PgAuditWriter struct {
	Pool *pgxpool.Pool
}

// WriteAudit implements AuditWriter against core.audit_log.
func (w *PgAuditWriter) WriteAudit(ctx context.Context, ev AuditEvent) error {
	if w.Pool == nil {
		return errors.New("email: PgAuditWriter has nil pool")
	}
	payloadJSON, err := jsonMarshalOrEmpty(ev.Payload)
	if err != nil {
		return err
	}
	// core.audit_log.actor_type is NOT NULL DEFAULT 'human_api' (migration
	// 0025); a bare nil here would violate the constraint and roll back the
	// fail-closed write. Default an unset actor to 'human_api', mirroring
	// capability.EmitAudit. All current callers already set a valid actor.
	actorType := string(ev.ActorType)
	if actorType == "" {
		actorType = string(ActorHumanAPI)
	}
	var actorUserID any
	if ev.ActorUserID != uuid.Nil {
		actorUserID = ev.ActorUserID
	}
	var requestID any
	if ev.RequestID != uuid.Nil {
		requestID = ev.RequestID
	}
	var wsID any
	if ev.WorkspaceID != uuid.Nil {
		wsID = ev.WorkspaceID
	}
	_, err = w.Pool.Exec(ctx,
		`INSERT INTO core.audit_log
		   (event, workspace_id, actor_type, actor_user_id, request_id, payload)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		// string(payloadJSON), not payloadJSON: under pgx's simple query protocol
		// a []byte is sent as a bytea literal and rejected by the jsonb column (22P02).
		ev.Event, wsID, actorType, actorUserID, requestID, string(payloadJSON),
	)
	if err != nil {
		return fmt.Errorf("email: insert audit row: %w", err)
	}
	return nil
}

// jsonMarshalOrEmpty marshals payload to JSON; nil payload becomes "{}".
func jsonMarshalOrEmpty(payload map[string]any) ([]byte, error) {
	if payload == nil {
		return []byte("{}"), nil
	}
	return json.Marshal(payload)
}

// PgSuppressionStore implements SuppressionStore against the per-tenant
// schema's email_suppression table. Caller-provided schema is wrapped in
// pgx.Identifier{}.Sanitize() so SQL injection is impossible even with
// hostile input (which can't happen — schema comes from the workspace
// resolver — but defence-in-depth).
type PgSuppressionStore struct {
	Pool *pgxpool.Pool
}

// Suppressed returns a map[email]bool for the supplied addresses. Emails
// not in the suppression table are absent from the result.
func (s *PgSuppressionStore) Suppressed(ctx context.Context, schema string, emails []string) (map[string]bool, error) {
	out := make(map[string]bool, len(emails))
	if len(emails) == 0 {
		return out, nil
	}
	q := `SELECT email FROM ` + pgx.Identifier{schema, "email_suppression"}.Sanitize() +
		` WHERE email = ANY($1)`
	rows, err := s.Pool.Query(ctx, q, emails)
	if err != nil {
		return nil, fmt.Errorf("email: suppression query: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var e string
		if err := rows.Scan(&e); err != nil {
			return nil, fmt.Errorf("email: suppression scan: %w", err)
		}
		out[e] = true
	}
	return out, rows.Err()
}

// Upsert inserts or updates a suppression row. The (email) UNIQUE
// constraint on the table guarantees idempotency.
func (s *PgSuppressionStore) Upsert(ctx context.Context, schema, email, reason string, at time.Time) error {
	q := `INSERT INTO ` + pgx.Identifier{schema, "email_suppression"}.Sanitize() +
		` (email, reason, suppressed_at) VALUES ($1, $2, $3)
		  ON CONFLICT (email) DO UPDATE
		    SET reason = EXCLUDED.reason, suppressed_at = EXCLUDED.suppressed_at`
	_, err := s.Pool.Exec(ctx, q, email, reason, at)
	if err != nil {
		return fmt.Errorf("email: suppression upsert: %w", err)
	}
	return nil
}
