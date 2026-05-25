// Package tenant — structured error types shared between the SQL
// function payload contract and the CLI parser. This is the one carve-out
// from D11 (Story 8.1): a data-only file (no logic, no DB calls).
package tenant

import "fmt"

// ErrKind tags structured errors so callers (CI, tests, the Tasket-bound
// integrator workflow) can branch on them without parsing free-form text.
type ErrKind string

const (
	ErrKindSlugInvalid     ErrKind = "slug_invalid"
	ErrKindSlugConflict    ErrKind = "slug_conflict"
	ErrKindSlugReserved    ErrKind = "slug_reserved"
	ErrKindSlugTombstoned  ErrKind = "slug_tombstoned"
	ErrKindTemplateUnknown ErrKind = "template_unknown"
	ErrKindAPIEnvLeak      ErrKind = "api_env_leak"
	ErrKindDBConnect       ErrKind = "db_connect"
	ErrKindDBProvision     ErrKind = "db_provision"
)

// StructErr is the wire shape of every loud failure printed by
// lecrm-admin. The Kind discriminator lets test code and tasket
// post-processors match without regex-fragility on the message.
type StructErr struct {
	Kind    ErrKind
	Message string
}

func (e *StructErr) Error() string { return e.Message }

// New constructs a StructErr with a formatted message.
func New(kind ErrKind, format string, args ...any) *StructErr {
	return &StructErr{Kind: kind, Message: fmt.Sprintf(format, args...)}
}
