package metadata

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
)

func cacheKey(schema, parentType string) string {
	return schema + ":" + parentType
}

const (
	cacheTTL     = 5 * time.Minute
	cacheMaxSize = 50
)

type defEntry struct {
	propType string
	allowed  []string
	required bool
}

type cacheEntry struct {
	defs      map[string]defEntry
	fetchedAt time.Time
}

type defCache struct {
	mu      sync.Mutex
	entries map[string]cacheEntry // key: parentType
}

func newDefCache() *defCache {
	return &defCache{entries: make(map[string]cacheEntry)}
}

func (c *defCache) get(parentType string) (map[string]defEntry, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	entry, ok := c.entries[parentType]
	if !ok || time.Since(entry.fetchedAt) > cacheTTL {
		return nil, false
	}
	return entry.defs, true
}

func (c *defCache) put(parentType string, defs map[string]defEntry) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if len(c.entries) >= cacheMaxSize {
		var oldest string
		var oldestTime time.Time
		for k, v := range c.entries {
			if oldest == "" || v.fetchedAt.Before(oldestTime) {
				oldest = k
				oldestTime = v.fetchedAt
			}
		}
		if oldest != "" {
			delete(c.entries, oldest)
		}
	}

	c.entries[parentType] = cacheEntry{
		defs:      defs,
		fetchedAt: time.Now(),
	}
}

func (c *defCache) invalidate(parentType string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.entries, parentType)
}

// loadDefs fetches definitions from DB or cache for validation.
func (s *Store) loadDefs(ctx context.Context, parentType string) (map[string]defEntry, error) {
	key := cacheKey(s.schema, parentType)
	if defs, ok := s.cache.get(key); ok {
		return defs, nil
	}

	q := `SELECT property_key, property_type, allowed_values, required FROM ` +
		pgx.Identifier{s.schema, "custom_property_definitions"}.Sanitize() +
		` WHERE parent_type = $1`
	rows, err := s.pool.Query(ctx, q, parentType)
	if err != nil {
		return nil, fmt.Errorf("metadata.loadDefs: %w", err)
	}
	defer rows.Close()

	defs := make(map[string]defEntry)
	for rows.Next() {
		var key, propType string
		var allowedRaw []byte
		var required bool
		if err := rows.Scan(&key, &propType, &allowedRaw, &required); err != nil {
			return nil, fmt.Errorf("metadata.loadDefs scan: %w", err)
		}
		e := defEntry{propType: propType, required: required}
		if len(allowedRaw) > 0 {
			if err := json.Unmarshal(allowedRaw, &e.allowed); err != nil {
				return nil, fmt.Errorf("metadata.loadDefs decode: %w", err)
			}
		}
		defs[key] = e
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("metadata.loadDefs rows: %w", err)
	}

	s.cache.put(key, defs)
	return defs, nil
}

// invalidateCache removes the cached definitions for a parent type.
func (s *Store) invalidateCache(parentType string) {
	s.cache.invalidate(cacheKey(s.schema, parentType))
}

// validateStrict checks data against custom_property_definitions strictly:
// unknown keys are rejected, types are enforced, enum values are validated.
func (s *Store) validateStrict(ctx context.Context, parentType string, data map[string]any) error {
	defs, err := s.loadDefs(ctx, parentType)
	if err != nil {
		return err
	}

	// Check required fields
	for key, def := range defs {
		if def.required {
			if _, ok := data[key]; !ok {
				return &ValidationError{Msg: fmt.Sprintf("required property %q missing", key)}
			}
		}
	}

	// Validate each key in data against definitions
	for key, val := range data {
		def, ok := defs[key]
		if !ok {
			return &ValidationError{Msg: fmt.Sprintf("property %q is not defined", key)}
		}

		if err := validateValue(key, val, def); err != nil {
			return err
		}
	}
	return nil
}

func validateValue(key string, val any, def defEntry) error {
	switch def.propType {
	case "string":
		if _, ok := val.(string); !ok {
			return &ValidationError{Msg: fmt.Sprintf("property %q must be a string, got %T", key, val)}
		}
	case "number":
		switch val.(type) {
		case float64, int, int64, float32:
		default:
			return &ValidationError{Msg: fmt.Sprintf("property %q must be a number, got %T", key, val)}
		}
	case "boolean":
		if _, ok := val.(bool); !ok {
			return &ValidationError{Msg: fmt.Sprintf("property %q must be a boolean, got %T", key, val)}
		}
	case "date":
		s, ok := val.(string)
		if !ok {
			return &ValidationError{Msg: fmt.Sprintf("property %q must be a date string, got %T", key, val)}
		}
		if _, err := time.Parse("2006-01-02", s); err != nil {
			return &ValidationError{Msg: fmt.Sprintf("property %q is not a valid date (expected YYYY-MM-DD)", key)}
		}
	case "enum":
		strVal, ok := val.(string)
		if !ok {
			return &ValidationError{Msg: fmt.Sprintf("enum property %q must be a string, got %T", key, val)}
		}
		if len(def.allowed) > 0 {
			valid := false
			for _, av := range def.allowed {
				if av == strVal {
					valid = true
					break
				}
			}
			if !valid {
				return &ValidationError{Msg: fmt.Sprintf("%q is not a valid value for %q (allowed: %s)",
					strVal, key, strings.Join(def.allowed, ", "))}
			}
		}
	case "json":
		switch val.(type) {
		case map[string]any, []any:
		default:
			return &ValidationError{Msg: fmt.Sprintf("property %q must be a JSON object or array, got %T", key, val)}
		}
	}
	return nil
}
