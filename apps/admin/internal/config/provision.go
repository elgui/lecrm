package config

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/jackc/pgx/v5"
)

type PropertyProvisionResult struct {
	Created int
	Updated int
}

func provisionCustomProperties(ctx context.Context, conn *pgx.Conn, ref WorkspaceRef, cfg *MethodologyConfig, stdout io.Writer) (*PropertyProvisionResult, error) {
	props := collectDealProperties(cfg)
	if len(props) == 0 {
		return &PropertyProvisionResult{}, nil
	}

	q := fmt.Sprintf(
		`INSERT INTO %s.custom_property_definitions
		   (parent_type, property_key, property_type, allowed_values, required)
		 VALUES ($1, $2, $3, $4, $5)
		 ON CONFLICT (parent_type, property_key) DO UPDATE SET
		   property_type  = EXCLUDED.property_type,
		   allowed_values = EXCLUDED.allowed_values,
		   required       = EXCLUDED.required
		 RETURNING (xmax = 0)`, // xmax=0 means INSERT, otherwise UPDATE
		safeIdent(ref.RoleName))

	var result PropertyProvisionResult
	for _, p := range props {
		var allowedJSON []byte
		if len(p.AllowedValues) > 0 {
			var err error
			allowedJSON, err = json.Marshal(p.AllowedValues)
			if err != nil {
				return nil, fmt.Errorf("marshal allowed_values for %q: %w", p.Key, err)
			}
		}

		var wasInsert bool
		err := conn.QueryRow(ctx, q, "deal", p.Key, p.Type, allowedJSON, p.Required).Scan(&wasInsert)
		if err != nil {
			return nil, fmt.Errorf("upsert property %q: %w", p.Key, err)
		}
		if wasInsert {
			result.Created++
		} else {
			result.Updated++
		}
	}

	_, _ = fmt.Fprintf(stdout, "Custom properties: %d created, %d updated\n", result.Created, result.Updated)
	return &result, nil
}

func collectDealProperties(cfg *MethodologyConfig) []StageProperty {
	seen := make(map[string]bool)
	var props []StageProperty
	for _, stage := range cfg.PipelineStages {
		for _, p := range stage.StageProperties {
			if seen[p.Key] {
				continue
			}
			seen[p.Key] = true
			props = append(props, p)
		}
	}
	return props
}
