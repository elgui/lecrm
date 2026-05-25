package metadata

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Definition is a row from custom_property_definitions.
type Definition struct {
	ID            uuid.UUID `json:"id"`
	ParentType    string    `json:"parent_type"`
	PropertyKey   string    `json:"property_key"`
	PropertyType  string    `json:"property_type"`
	AllowedValues []string  `json:"allowed_values,omitempty"`
	Required      bool      `json:"required"`
}

var validPropertyTypes = map[string]bool{
	"string":  true,
	"number":  true,
	"boolean": true,
	"enum":    true,
	"date":    true,
	"json":    true,
}

var validParentTypes = map[string]bool{
	"contact": true,
	"deal":    true,
}

// ListDefinitions returns all custom property definitions for a parent type.
func (s *Store) ListDefinitions(ctx context.Context, parentType string) ([]Definition, error) {
	q := `SELECT id, parent_type, property_key, property_type, allowed_values, required FROM ` +
		pgx.Identifier{s.schema, "custom_property_definitions"}.Sanitize() +
		` WHERE parent_type = $1 ORDER BY property_key`
	rows, err := s.pool.Query(ctx, q, parentType)
	if err != nil {
		return nil, fmt.Errorf("metadata.ListDefinitions: %w", err)
	}
	defer rows.Close()

	var out []Definition
	for rows.Next() {
		var d Definition
		var allowedRaw []byte
		if err := rows.Scan(&d.ID, &d.ParentType, &d.PropertyKey, &d.PropertyType, &allowedRaw, &d.Required); err != nil {
			return nil, fmt.Errorf("metadata.ListDefinitions scan: %w", err)
		}
		if len(allowedRaw) > 0 {
			if err := json.Unmarshal(allowedRaw, &d.AllowedValues); err != nil {
				return nil, fmt.Errorf("metadata.ListDefinitions decode: %w", err)
			}
		}
		out = append(out, d)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("metadata.ListDefinitions rows: %w", err)
	}
	if out == nil {
		out = []Definition{}
	}
	return out, nil
}

// CreateDefinitionInput is the input for creating a new property definition.
type CreateDefinitionInput struct {
	ParentType    string   `json:"parent_type"`
	PropertyKey   string   `json:"property_key"`
	PropertyType  string   `json:"property_type"`
	AllowedValues []string `json:"allowed_values,omitempty"`
	Required      bool     `json:"required"`
}

// CreateDefinition inserts a new custom property definition.
func (s *Store) CreateDefinition(ctx context.Context, input CreateDefinitionInput) (Definition, error) {
	if !validParentTypes[input.ParentType] {
		return Definition{}, fmt.Errorf("metadata.CreateDefinition: invalid parent_type %q", input.ParentType)
	}
	if !validPropertyTypes[input.PropertyType] {
		return Definition{}, fmt.Errorf("metadata.CreateDefinition: invalid property_type %q", input.PropertyType)
	}
	if input.PropertyKey == "" {
		return Definition{}, fmt.Errorf("metadata.CreateDefinition: property_key is required")
	}
	if input.PropertyType == "enum" && len(input.AllowedValues) == 0 {
		return Definition{}, fmt.Errorf("metadata.CreateDefinition: enum type requires allowed_values")
	}

	var allowedRaw []byte
	if len(input.AllowedValues) > 0 {
		var err error
		allowedRaw, err = json.Marshal(input.AllowedValues)
		if err != nil {
			return Definition{}, fmt.Errorf("metadata.CreateDefinition marshal: %w", err)
		}
	}

	q := `INSERT INTO ` + pgx.Identifier{s.schema, "custom_property_definitions"}.Sanitize() +
		` (parent_type, property_key, property_type, allowed_values, required) VALUES ($1, $2, $3, $4, $5) RETURNING id`
	var id uuid.UUID
	if err := s.pool.QueryRow(ctx, q, input.ParentType, input.PropertyKey, input.PropertyType, allowedRaw, input.Required).Scan(&id); err != nil {
		return Definition{}, fmt.Errorf("metadata.CreateDefinition: %w", err)
	}

	s.invalidateCache(input.ParentType)

	return Definition{
		ID:            id,
		ParentType:    input.ParentType,
		PropertyKey:   input.PropertyKey,
		PropertyType:  input.PropertyType,
		AllowedValues: input.AllowedValues,
		Required:      input.Required,
	}, nil
}

// DeleteDefinition removes a property definition by ID and cleans up
// matching data from objects.
func (s *Store) DeleteDefinition(ctx context.Context, id uuid.UUID) error {
	// Look up the definition first so we know the parent_type for cache invalidation.
	q := `SELECT parent_type, property_key FROM ` +
		pgx.Identifier{s.schema, "custom_property_definitions"}.Sanitize() +
		` WHERE id = $1`
	var parentType, propertyKey string
	if err := s.pool.QueryRow(ctx, q, id).Scan(&parentType, &propertyKey); err != nil {
		return fmt.Errorf("metadata.DeleteDefinition lookup: %w", err)
	}

	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("metadata.DeleteDefinition begin: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	delQ := `DELETE FROM ` + pgx.Identifier{s.schema, "custom_property_definitions"}.Sanitize() +
		` WHERE id = $1`
	if _, err := tx.Exec(ctx, delQ, id); err != nil {
		return fmt.Errorf("metadata.DeleteDefinition: %w", err)
	}

	// Strip the deleted key from all object data blobs for this parent type.
	objTable := pgx.Identifier{s.schema, "objects"}.Sanitize()
	stripQ := `UPDATE ` + objTable +
		` SET data = data - $1, updated_at = now() WHERE object_type = 'custom_properties' AND parent_type = $2 AND data ? $1`
	if _, err := tx.Exec(ctx, stripQ, propertyKey, parentType); err != nil {
		return fmt.Errorf("metadata.DeleteDefinition strip: %w", err)
	}

	auditPayload, _ := json.Marshal(map[string]any{
		"definition_id": id.String(),
		"parent_type":   parentType,
		"property_key":  propertyKey,
	})
	if _, err := tx.Exec(ctx,
		`INSERT INTO core.audit_log (event, workspace_id, payload) VALUES ('metadata.definition.delete', $1, $2)`,
		uuid.NullUUID{UUID: s.workspaceID, Valid: s.workspaceID != uuid.Nil},
		auditPayload,
	); err != nil {
		return fmt.Errorf("metadata.DeleteDefinition audit: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("metadata.DeleteDefinition commit: %w", err)
	}

	s.invalidateCache(parentType)
	return nil
}
