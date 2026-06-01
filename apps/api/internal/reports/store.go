package reports

// Persistence + execution for native reports.
//
// Saved report definitions reuse the ADR-010 metadata JSONB pattern: they are
// rows in the workspace `objects` table with object_type='saved_report' and
// the Definition marshalled into `data`. No new per-tenant table / migration is
// needed — the objects table already exists in every workspace schema and is
// reached through the same search_path-scoped transactions as custom
// properties. All reads/writes go through capability.{Read,Write}Tx, so they
// are workspace-isolated and (writes) fail-closed-audited exactly like the rest
// of the CRM.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gbconsult/lecrm/apps/api/capability"
)

const savedReportObjectType = "saved_report"

// Store provides report persistence + execution for one workspace.
type Store struct {
	pool        *pgxpool.Pool
	schema      string
	workspaceID uuid.UUID
}

// NewStore binds a Store to a workspace schema + id.
func NewStore(pool *pgxpool.Pool, schema string, workspaceID uuid.UUID) *Store {
	return &Store{pool: pool, schema: schema, workspaceID: workspaceID}
}

// SavedReport is a persisted definition with its identity + timestamps.
type SavedReport struct {
	ID         uuid.UUID  `json:"id"`
	Definition Definition `json:"definition"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}

// ListSaved returns all saved reports for the workspace, newest first.
func (s *Store) ListSaved(ctx context.Context) ([]SavedReport, error) {
	out := []SavedReport{}
	err := capability.ReadTx(ctx, s.pool, s.schema, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx,
			`SELECT id, data, created_at, updated_at FROM objects
			 WHERE object_type = $1 ORDER BY created_at DESC, id DESC`,
			savedReportObjectType)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			sr, err := scanSaved(rows)
			if err != nil {
				return err
			}
			out = append(out, sr)
		}
		return rows.Err()
	})
	return out, err
}

// GetSaved returns one saved report, or pgx.ErrNoRows when absent.
func (s *Store) GetSaved(ctx context.Context, id uuid.UUID) (SavedReport, error) {
	var sr SavedReport
	err := capability.ReadTx(ctx, s.pool, s.schema, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx,
			`SELECT id, data, created_at, updated_at FROM objects
			 WHERE object_type = $1 AND id = $2`,
			savedReportObjectType, id)
		var e error
		sr, e = scanSaved(row)
		return e
	})
	return sr, err
}

// CreateSaved persists a new definition and returns it with its assigned id.
// Validates the definition first (400 path) and audits the write (fail-closed).
func (s *Store) CreateSaved(ctx context.Context, def Definition) (SavedReport, error) {
	if err := def.normalizeAndValidate(); err != nil {
		return SavedReport{}, err
	}
	data, err := json.Marshal(def)
	if err != nil {
		return SavedReport{}, fmt.Errorf("reports.CreateSaved marshal: %w", err)
	}
	var sr SavedReport
	err = capability.WriteTx(ctx, s.pool, s.schema, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx,
			`INSERT INTO objects (object_type, data) VALUES ($1, $2)
			 RETURNING id, data, created_at, updated_at`,
			savedReportObjectType, data)
		var e error
		sr, e = scanSaved(row)
		if e != nil {
			return e
		}
		return capability.EmitAudit(ctx, tx, "report.definition.created", s.workspaceID,
			capability.ActorTypeHumanAPI, map[string]any{"report_id": sr.ID.String(), "name": def.Name})
	})
	return sr, err
}

// UpdateSaved overwrites an existing definition. Returns pgx.ErrNoRows if the
// id is unknown (so the handler can 404).
func (s *Store) UpdateSaved(ctx context.Context, id uuid.UUID, def Definition) (SavedReport, error) {
	if err := def.normalizeAndValidate(); err != nil {
		return SavedReport{}, err
	}
	data, err := json.Marshal(def)
	if err != nil {
		return SavedReport{}, fmt.Errorf("reports.UpdateSaved marshal: %w", err)
	}
	var sr SavedReport
	err = capability.WriteTx(ctx, s.pool, s.schema, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx,
			`UPDATE objects SET data = $2, updated_at = now()
			 WHERE object_type = $1 AND id = $3
			 RETURNING id, data, created_at, updated_at`,
			savedReportObjectType, data, id)
		var e error
		sr, e = scanSaved(row)
		if e != nil {
			return e
		}
		return capability.EmitAudit(ctx, tx, "report.definition.updated", s.workspaceID,
			capability.ActorTypeHumanAPI, map[string]any{"report_id": id.String(), "name": def.Name})
	})
	return sr, err
}

// DeleteSaved removes a definition. Returns pgx.ErrNoRows if the id is unknown.
func (s *Store) DeleteSaved(ctx context.Context, id uuid.UUID) error {
	return capability.WriteTx(ctx, s.pool, s.schema, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx,
			`DELETE FROM objects WHERE object_type = $1 AND id = $2`,
			savedReportObjectType, id)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return pgx.ErrNoRows
		}
		return capability.EmitAudit(ctx, tx, "report.definition.deleted", s.workspaceID,
			capability.ActorTypeHumanAPI, map[string]any{"report_id": id.String()})
	})
}

// RunRow is one aggregated bucket.
type RunRow struct {
	Label   string   `json:"label"`
	Current float64  `json:"current"`
	Prior   *float64 `json:"prior,omitempty"`
}

// RunResult is the full result of executing a definition.
type RunResult struct {
	Metric      string    `json:"metric"`
	MetricLabel string    `json:"metric_label"`
	Dimension   string    `json:"dimension"`
	Period      string    `json:"period"`
	CompareYoY  bool      `json:"compare_yoy"`
	CurrentLabel string   `json:"current_label"`
	PriorLabel  string    `json:"prior_label,omitempty"`
	Rows        []RunRow  `json:"rows"`
	GeneratedAt time.Time `json:"generated_at"`
}

// Run executes a definition against the workspace schema and returns the
// aggregated rows. `now` is injected (the handler passes time.Now()) so the
// period windows are deterministic in tests.
func (s *Store) Run(ctx context.Context, def Definition, now time.Time) (RunResult, error) {
	sql, args, plan, err := BuildRunQuery(def, now)
	if err != nil {
		return RunResult{}, err
	}
	res := RunResult{
		Metric:       def.Metric,
		MetricLabel:  metricLabel(def.Metric),
		Dimension:    def.Dimension,
		Period:       def.Period,
		CompareYoY:   plan.HasPrior,
		CurrentLabel: plan.CurLabel,
		PriorLabel:   plan.PriorLabel,
		Rows:         []RunRow{},
		GeneratedAt:  now,
	}
	err = capability.ReadTx(ctx, s.pool, s.schema, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, sql, args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var row RunRow
			var dimOrder int64
			if plan.HasPrior {
				var prior float64
				if err := rows.Scan(&row.Label, &row.Current, &prior, &dimOrder); err != nil {
					return err
				}
				row.Prior = &prior
			} else {
				if err := rows.Scan(&row.Label, &row.Current, &dimOrder); err != nil {
					return err
				}
			}
			res.Rows = append(res.Rows, row)
		}
		return rows.Err()
	})
	if err != nil {
		return RunResult{}, err
	}
	return res, nil
}

// rowScanner unifies *pgx.Row and pgx.Rows for scanSaved.
type rowScanner interface {
	Scan(dest ...any) error
}

func scanSaved(rs rowScanner) (SavedReport, error) {
	var sr SavedReport
	var data []byte
	if err := rs.Scan(&sr.ID, &data, &sr.CreatedAt, &sr.UpdatedAt); err != nil {
		return SavedReport{}, err
	}
	if err := json.Unmarshal(data, &sr.Definition); err != nil {
		return SavedReport{}, fmt.Errorf("reports.scanSaved decode: %w", err)
	}
	return sr, nil
}

// IsNotFound reports whether err is the "no such report" sentinel.
func IsNotFound(err error) bool { return errors.Is(err, pgx.ErrNoRows) }
