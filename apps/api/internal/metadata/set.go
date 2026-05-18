// Package metadata implements the custom-property metadata engine (ADR-010 §5).
//
// All mutations go through Set, which wraps the JSONB write and the
// metadata.property.upsert audit event in a single Postgres transaction.
// If the audit INSERT fails the transaction rolls back — the metadata write
// is rejected (ADR-009 §7.2 fail-closed invariant).
package metadata

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Store provides typed access to custom properties for one workspace.
// pool must hold INSERT on core.audit_log and full access to the workspace schema.
type Store struct {
	pool        *pgxpool.Pool
	schema      string    // workspace schema name (= workspace role name)
	workspaceID uuid.UUID // for audit log workspace_id column
}

// New returns a Store bound to the given pool, workspace schema, and workspace ID.
func New(pool *pgxpool.Pool, schema string, workspaceID uuid.UUID) *Store {
	return &Store{pool: pool, schema: schema, workspaceID: workspaceID}
}

// Object is a row from the objects table.
type Object struct {
	ID         uuid.UUID
	ObjectType string
	ParentType string
	ParentID   uuid.UUID
	Data       map[string]any
}

// Get returns all custom properties for one parent record.
// Returns an empty map when no properties have been set.
func (s *Store) Get(ctx context.Context, parentType string, parentID uuid.UUID) (map[string]any, error) {
	q := `SELECT data FROM ` + pgx.Identifier{s.schema, "objects"}.Sanitize() +
		` WHERE object_type = 'custom_properties' AND parent_type = $1 AND parent_id = $2`
	var rawJSON []byte
	if err := s.pool.QueryRow(ctx, q, parentType, parentID).Scan(&rawJSON); err != nil {
		if err == pgx.ErrNoRows {
			return map[string]any{}, nil
		}
		return nil, fmt.Errorf("metadata.Get: %w", err)
	}
	var out map[string]any
	if err := json.Unmarshal(rawJSON, &out); err != nil {
		return nil, fmt.Errorf("metadata.Get decode: %w", err)
	}
	return out, nil
}

// Set upserts the entire custom-property payload for one parent record.
// Validates the payload against custom_property_definitions before writing.
//
// Fail-closed (ADR-009 §7.2): the JSONB write and the metadata.property.upsert
// audit event share one Postgres transaction. An audit INSERT failure rolls back
// the objects write — the caller receives an error and no data is persisted.
func (s *Store) Set(ctx context.Context, parentType string, parentID uuid.UUID, data map[string]any) error {
	if err := s.validate(ctx, parentType, data); err != nil {
		return err
	}

	rawJSON, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("metadata.Set marshal: %w", err)
	}

	keys := make([]string, 0, len(data))
	for k := range data {
		keys = append(keys, k)
	}
	auditPayload, _ := json.Marshal(map[string]any{
		"parent_type":   parentType,
		"parent_id":     parentID.String(),
		"property_keys": keys,
	})

	objTable := pgx.Identifier{s.schema, "objects"}.Sanitize()

	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("metadata.Set begin: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	// Replace the full property bag atomically (DELETE + INSERT avoids a
	// UNIQUE-constraint dependency that is not yet part of the schema).
	if _, err := tx.Exec(ctx,
		`DELETE FROM `+objTable+` WHERE object_type = 'custom_properties' AND parent_type = $1 AND parent_id = $2`,
		parentType, parentID,
	); err != nil {
		return fmt.Errorf("metadata.Set delete: %w", err)
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO `+objTable+` (object_type, parent_type, parent_id, data) VALUES ('custom_properties', $1, $2, $3)`,
		parentType, parentID, rawJSON,
	); err != nil {
		return fmt.Errorf("metadata.Set insert: %w", err)
	}

	// Emit audit event — must succeed or the entire transaction rolls back.
	if _, err := tx.Exec(ctx,
		`INSERT INTO core.audit_log (event, workspace_id, payload) VALUES ('metadata.property.upsert', $1, $2)`,
		uuid.NullUUID{UUID: s.workspaceID, Valid: s.workspaceID != uuid.Nil},
		auditPayload,
	); err != nil {
		return fmt.Errorf("metadata.Set audit: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("metadata.Set commit: %w", err)
	}
	return nil
}

// Find queries objects by JSONB containment predicate using the GIN index.
func (s *Store) Find(ctx context.Context, parentType string, query map[string]any) ([]Object, error) {
	rawQuery, err := json.Marshal(query)
	if err != nil {
		return nil, fmt.Errorf("metadata.Find marshal: %w", err)
	}
	q := `SELECT id, object_type, parent_type, parent_id, data FROM ` +
		pgx.Identifier{s.schema, "objects"}.Sanitize() +
		` WHERE parent_type = $1 AND data @> $2`
	rows, err := s.pool.Query(ctx, q, parentType, rawQuery)
	if err != nil {
		return nil, fmt.Errorf("metadata.Find query: %w", err)
	}
	defer rows.Close()

	var out []Object
	for rows.Next() {
		var obj Object
		var dataRaw []byte
		var pt pgtype.Text
		var pid uuid.NullUUID
		if err := rows.Scan(&obj.ID, &obj.ObjectType, &pt, &pid, &dataRaw); err != nil {
			return nil, fmt.Errorf("metadata.Find scan: %w", err)
		}
		obj.ParentType = pt.String
		obj.ParentID = pid.UUID
		if err := json.Unmarshal(dataRaw, &obj.Data); err != nil {
			return nil, fmt.Errorf("metadata.Find decode: %w", err)
		}
		out = append(out, obj)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("metadata.Find rows: %w", err)
	}
	return out, nil
}

// validate checks data against custom_property_definitions for the workspace.
// Unknown keys are allowed (v0 lenient mode); known enum properties must match allowed_values.
func (s *Store) validate(ctx context.Context, parentType string, data map[string]any) error {
	q := `SELECT property_key, property_type, allowed_values, required FROM ` +
		pgx.Identifier{s.schema, "custom_property_definitions"}.Sanitize() +
		` WHERE parent_type = $1`
	rows, err := s.pool.Query(ctx, q, parentType)
	if err != nil {
		return fmt.Errorf("metadata.validate: %w", err)
	}
	defer rows.Close()

	type defEntry struct {
		propType string
		allowed  []string
		required bool
	}
	defs := make(map[string]defEntry)
	for rows.Next() {
		var key, propType string
		var allowedRaw []byte
		var required bool
		if err := rows.Scan(&key, &propType, &allowedRaw, &required); err != nil {
			return fmt.Errorf("metadata.validate scan: %w", err)
		}
		e := defEntry{propType: propType, required: required}
		if len(allowedRaw) > 0 {
			if err := json.Unmarshal(allowedRaw, &e.allowed); err != nil {
				return fmt.Errorf("metadata.validate decode allowed_values: %w", err)
			}
		}
		defs[key] = e
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("metadata.validate rows: %w", err)
	}

	for key, def := range defs {
		if def.required {
			if _, ok := data[key]; !ok {
				return fmt.Errorf("metadata.Set: required property %q missing", key)
			}
		}
	}

	for key, val := range data {
		def, ok := defs[key]
		if !ok {
			continue
		}
		if def.propType == "enum" && len(def.allowed) > 0 {
			strVal, ok := val.(string)
			if !ok {
				return fmt.Errorf("metadata.Set: enum property %q must be a string, got %T", key, val)
			}
			valid := false
			for _, av := range def.allowed {
				if av == strVal {
					valid = true
					break
				}
			}
			if !valid {
				return fmt.Errorf("metadata.Set: %q is not a valid value for %q (allowed: %s)",
					strVal, key, strings.Join(def.allowed, ", "))
			}
		}
	}
	return nil
}
