package config

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const ObjectType = "methodology_config"

type MethodologyConfig struct {
	VersionSeq          int                  `json:"version_seq"`
	TemplateName        string               `json:"template_name"`
	AcquisitionChannels []AcquisitionChannel `json:"acquisition_channels"`
	PipelineStages      []PipelineStage      `json:"pipeline_stages"`
	Automations         []Automation         `json:"automations"`
	ColorCoding         map[string]string    `json:"color_coding"`
}

type AcquisitionChannel struct {
	Key         string            `json:"key"`
	Label       string            `json:"label"`
	Attribution map[string]string `json:"attribution"`
}

type PipelineStage struct {
	Key             string          `json:"key"`
	Label           string          `json:"label"`
	OrderIndex      int             `json:"order_index"`
	EntryConditions []string        `json:"entry_conditions"`
	ExitConditions  []string        `json:"exit_conditions"`
	StageProperties []StageProperty `json:"stage_properties"`
}

type StageProperty struct {
	Key           string   `json:"key"`
	Type          string   `json:"type"`
	Required      bool     `json:"required"`
	AllowedValues []string `json:"allowed_values,omitempty"`
}

type Automation struct {
	Key     string   `json:"key"`
	Trigger Trigger  `json:"trigger"`
	Actions []Action `json:"actions"`
}

type Trigger struct {
	Type string `json:"type"`
	From string `json:"from,omitempty"`
	To   string `json:"to,omitempty"`
}

type Action struct {
	Type     string `json:"type"`
	Channel  string `json:"channel,omitempty"`
	Template string `json:"template,omitempty"`
	Field    string `json:"field,omitempty"`
	Value    string `json:"value,omitempty"`
}

type WorkspaceRef struct {
	ID       uuid.UUID
	Slug     string
	RoleName string
}

func ResolveSlug(ctx context.Context, conn *pgx.Conn, slug string) (WorkspaceRef, error) {
	var roleName string
	var id uuid.UUID
	err := conn.QueryRow(ctx,
		`SELECT id, role_name FROM core.workspaces WHERE slug = $1 AND tombstoned_at IS NULL`,
		slug).Scan(&id, &roleName)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return WorkspaceRef{}, fmt.Errorf("tenant %q not found", slug)
	case err != nil:
		return WorkspaceRef{}, fmt.Errorf("lookup tenant %q: %w", slug, err)
	}
	return WorkspaceRef{ID: id, Slug: slug, RoleName: roleName}, nil
}

func safeIdent(name string) string {
	for i := 0; i < len(name); i++ {
		c := name[i]
		isLower := c >= 'a' && c <= 'z'
		isDigit := c >= '0' && c <= '9'
		if !isLower && !isDigit && c != '_' {
			return `"invalid_identifier"`
		}
	}
	return `"` + name + `"`
}
