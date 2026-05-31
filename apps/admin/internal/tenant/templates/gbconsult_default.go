// Package templates holds the v0 hardcoded provisioning templates.
//
// Sprint 9 / ADR-010 replaces this with a metadata-engine-backed registry.
// Until then, callers select by name (gbconsult-default) and the SQL
// wrapper (core.lecrm_provision_workspace_with_registry) seeds the same
// stages literally. Keeping a Go-side copy lets verify.go assert the
// pipeline_stages table matches without re-reading the migration source.
package templates

// GBConsultDefaultStages is the 5-stage default pipeline that ships with
// every tenant created by the integrator (per Story 8.1 AC-F5). The order
// matches order_index 1..5 in the seeded pipeline_stages table.
//
// Labels are French to match the French-SMB ICP (deal titles, custom fields
// and the ICP are all French). The combined "Gagné / Perdu" stage keeps the
// legacy single-closed-stage shape; splitting Won vs Lost into two stages is
// flagged for Guillaume (changes the stage count — see tasket 0077).
var GBConsultDefaultStages = []string{
	"Découverte",
	"Qualifié",
	"Proposition envoyée",
	"Négociation",
	"Gagné / Perdu",
}

// GBConsultDefaultName is the template identifier the CLI passes to the
// SECURITY DEFINER wrapper. Must match the literal compared in
// core.lecrm_provision_workspace_with_registry.
const GBConsultDefaultName = "gbconsult-default"
