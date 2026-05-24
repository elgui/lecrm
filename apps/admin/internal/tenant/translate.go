// Package tenant translates between the DB-internal vocabulary
// ("workspace") and the operator-facing vocabulary ("Tenant") per D12
// (Story 8.1). DB code paths keep the verbatim "workspaces" name; every
// CLI line that reaches Léo's terminal goes through this layer.
package tenant

// OperatorNoun is the operator-facing label that replaces "workspace"
// in CLI output. The DB schema and tables continue to use "workspace"
// — only stdout/stderr is translated.
const OperatorNoun = "Tenant"

// Invariant labels for the verify subcommand. Keeping them in one place
// keeps AC-VFY-1's "[OK] INV-05 Tenant registry row exists" format
// consistent and lets a future contributor extend the set without
// hunting through verify.go.
var invariantLabels = map[string]string{
	"INV-01": OperatorNoun + " role exists",
	"INV-02": OperatorNoun + " schema + River queue schema exist",
	"INV-03": OperatorNoun + " role has USAGE on its own schema",
	"INV-04": "Cross-tenant isolation (sibling fixture in test suite)",
	"INV-05": OperatorNoun + " registry row exists",
	"INV-06": OperatorNoun + " UUIDv7 round-trip matches provision-time ID",
	"INV-07": "Audit row exists for workspace.provisioned event",
	"INV-08": "Audit row workspace_id matches " + OperatorNoun + " ID (atomic write)",
	"INV-09": OperatorNoun + " re-create without --upsert exits non-zero (covered by create_test.go)",
	"INV-10": OperatorNoun + " re-create with --upsert is a no-op (covered by create_test.go)",
	"INV-11": OperatorNoun + " role cannot SELECT from provisioner-only tables",
	"INV-12": "Default pipeline contains exactly 5 stages of gbconsult-default",
	"INV-13": "Stdout includes [PROVISION] RBAC seeding: skipped",
	"INV-14": "Migration cold-clean against vanilla postgres:16 (deferred to sibling story)",
}

// InvariantLabel returns the operator-facing description for the given
// invariant code (e.g. "INV-05"). Unknown codes return the code itself.
func InvariantLabel(code string) string {
	if label, ok := invariantLabels[code]; ok {
		return label
	}
	return code
}
