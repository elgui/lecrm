module github.com/gbconsult/lecrm/apps/mcp

go 1.25.0

require (
	github.com/gbconsult/lecrm/apps/api v0.0.0
	github.com/google/uuid v1.6.0
	github.com/jackc/pgx/v5 v5.9.2
)

// The MCP adapter links the shared capability layer (ADR-012 §1) which
// lives in the apps/api module. The local replace keeps the separate
// single-module build (GOWORK=off, used by apps/mcp/Dockerfile) resolving
// the sibling without a published version. capability pulls in only pgx +
// google/uuid + golang.org/x/*, all already required above, so no new
// external dependency is introduced.
replace github.com/gbconsult/lecrm/apps/api => ../api

require (
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761 // indirect
	github.com/jackc/puddle/v2 v2.2.2 // indirect
	golang.org/x/sync v0.20.0 // indirect
	golang.org/x/text v0.36.0 // indirect
)
