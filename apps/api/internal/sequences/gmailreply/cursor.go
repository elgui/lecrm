package gmailreply

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/gbconsult/lecrm/apps/api/internal/sequences"
)

// gmailProviderID is the sync_connections.provider_id for a Gmail link. One
// per workspace (the table's UNIQUE(provider_id) constraint), so the cursor and
// connection helpers below address "the workspace's Gmail connection".
const gmailProviderID = "gmail"

// cursorState is the JSON shape persisted in sync_connections.sync_cursor for
// the Gmail connection: the last Gmail history id successfully scanned. Stored
// in the DB connection row (NOT the SOPS secrets manifest) per the setup
// runbook §3.
type cursorState struct {
	HistoryID uint64 `json:"history_id"`
}

// connectionSettings is the JSON shape stored in sync_connections.settings for
// the Gmail connection. It records which user (rep) owns the mailbox so the
// worker can build that user's HistoryClient, and the address for cross-checks.
type connectionSettings struct {
	UserID       uuid.UUID `json:"user_id"`
	EmailAddress string    `json:"email_address"`
}

// gmailConnection is an active Gmail connection row, reduced to what the
// watch-renewal worker needs.
type gmailConnection struct {
	ID           uuid.UUID
	UserID       uuid.UUID
	EmailAddress string
	HistoryID    uint64 // current cursor (0 if unset)
}

// loadCursor reads the Gmail connection's persisted history-id cursor within
// the workspace-scoped tx. found is false when there is no Gmail connection row
// or its cursor is unset — the caller then baselines from the push
// notification's historyId.
func loadCursor(ctx context.Context, tx pgx.Tx) (historyID uint64, found bool, err error) {
	var raw []byte
	err = tx.QueryRow(ctx,
		`SELECT sync_cursor FROM sync_connections WHERE provider_id = $1`,
		gmailProviderID,
	).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("gmailreply: load cursor: %w", err)
	}
	if len(raw) == 0 || string(raw) == "null" {
		return 0, false, nil
	}
	var c cursorState
	if err := json.Unmarshal(raw, &c); err != nil {
		return 0, false, fmt.Errorf("gmailreply: decode cursor: %w", err)
	}
	if c.HistoryID == 0 {
		return 0, false, nil
	}
	return c.HistoryID, true, nil
}

// saveCursor persists historyID as the Gmail connection's cursor within the
// workspace tx. The jsonb is passed as a string, not []byte: under the tenant
// pool's simple query protocol a []byte binds as bytea and the jsonb column
// rejects it (SQLSTATE 22P02) — the same footgun avoided in capability.EmitAudit
// and sequences.emitAudit. A zero RowsAffected (no Gmail connection row) is
// surfaced as an error so a mis-provisioned workspace fails loudly.
func saveCursor(ctx context.Context, tx pgx.Tx, historyID uint64) error {
	body, err := json.Marshal(cursorState{HistoryID: historyID})
	if err != nil {
		return fmt.Errorf("gmailreply: encode cursor: %w", err)
	}
	tag, err := tx.Exec(ctx,
		`UPDATE sync_connections
		    SET sync_cursor = $1::jsonb, last_sync_at = now(), updated_at = now()
		  WHERE provider_id = $2`,
		string(body), gmailProviderID,
	)
	if err != nil {
		return fmt.Errorf("gmailreply: save cursor: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("gmailreply: save cursor: no %q connection row", gmailProviderID)
	}
	return nil
}

// matchSteps runs the single indexed lookup at the heart of reply correlation
// (ADR-004 rev 2 §4 step 3): for the supplied set of referenced Message-IDs,
// return the sent steps that carry them together with each step's enrollment
// state. rfc_message_id is partial-indexed (idx_step_rfc_msgid, migration
// 0025), so this is one index scan regardless of how many ids are checked.
//
// The ids are passed as text and cast to text[] (ANY($1::text[])) because the
// tenant pool runs simple protocol, which cannot encode a slice parameter
// directly (it resolves to OID 0) — the established pattern in metadata.GetMany.
func matchSteps(ctx context.Context, tx pgx.Tx, messageIDs []string) ([]MatchedStep, error) {
	if len(messageIDs) == 0 {
		return nil, nil
	}
	rows, err := tx.Query(ctx,
		`SELECT es.rfc_message_id, es.enrollment_id, es.step_index, e.state::text
		   FROM enrollment_steps es
		   JOIN enrollments e ON e.id = es.enrollment_id
		  WHERE es.rfc_message_id = ANY($1::text[])`,
		messageIDs,
	)
	if err != nil {
		return nil, fmt.Errorf("gmailreply: match steps: %w", err)
	}
	defer rows.Close()

	var out []MatchedStep
	for rows.Next() {
		var (
			rfcID     string
			enrollID  uuid.UUID
			stepIndex int16 // step_index is smallint
			state     string
		)
		if err := rows.Scan(&rfcID, &enrollID, &stepIndex, &state); err != nil {
			return nil, fmt.Errorf("gmailreply: scan matched step: %w", err)
		}
		out = append(out, MatchedStep{
			RFCMessageID: rfcID,
			EnrollmentID: enrollID,
			StepIndex:    int(stepIndex),
			State:        sequences.State(state),
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("gmailreply: iterate matched steps: %w", err)
	}
	return out, nil
}

// listActiveGmailConnections returns the workspace's active Gmail connection(s)
// for watch renewal, within the workspace tx. v1 has at most one (UNIQUE
// provider_id), but the worker iterates so a future multi-mailbox model needs
// no change here.
func listActiveGmailConnections(ctx context.Context, tx pgx.Tx) ([]gmailConnection, error) {
	rows, err := tx.Query(ctx,
		`SELECT id, settings, sync_cursor
		   FROM sync_connections
		  WHERE provider_id = $1 AND status = 'active'`,
		gmailProviderID,
	)
	if err != nil {
		return nil, fmt.Errorf("gmailreply: list connections: %w", err)
	}
	defer rows.Close()

	var out []gmailConnection
	for rows.Next() {
		var (
			id          uuid.UUID
			settingsRaw []byte
			cursorRaw   []byte
		)
		if err := rows.Scan(&id, &settingsRaw, &cursorRaw); err != nil {
			return nil, fmt.Errorf("gmailreply: scan connection: %w", err)
		}
		var s connectionSettings
		if len(settingsRaw) > 0 && string(settingsRaw) != "null" {
			if err := json.Unmarshal(settingsRaw, &s); err != nil {
				return nil, fmt.Errorf("gmailreply: decode connection settings: %w", err)
			}
		}
		conn := gmailConnection{ID: id, UserID: s.UserID, EmailAddress: s.EmailAddress}
		if len(cursorRaw) > 0 && string(cursorRaw) != "null" {
			var c cursorState
			if err := json.Unmarshal(cursorRaw, &c); err == nil {
				conn.HistoryID = c.HistoryID
			}
		}
		out = append(out, conn)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("gmailreply: iterate connections: %w", err)
	}
	return out, nil
}
