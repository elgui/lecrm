# apps/

Per ADR-009 §8.1. Three Go binaries + one frontend, all sharing one
Go module workspace (`go.work` at repo root, added when the first
`go mod init` lands).

| Path | Role | Runs as | Notes |
|---|---|---|---|
| `api/cmd/lecrm-api/` | Main HTTP server: REST under `/v1/*`, embedded SPA under `/*` via `//go:embed dist/*`. | Application role (no DDL). | Sprint 2 |
| `mcp/cmd/lecrm-mcp/` | MCP adapter. Separate Compose service; same Go module so `CrmAdapter` interface and sqlc types are shared. | Constrained role (read-only by default). | Sprint 9 skeleton, Sprint 13 wire format |
| `migrate/cmd/lecrm-migrate/` | Atlas runner invoked as Compose pre-deploy job. | `lecrm_provisioner` (Tier-0). | Sprint 3 |
| `web/` | React 19 + Vite + TanStack Router/Query + shadcn/ui + Radix. Builds to `apps/web/dist/`, embedded by `lecrm-api`. | n/a | Sprint 2 init, Sprint 4 first slice |

**Day-1 status:** directories created; no source yet. Each cmd ships a
`main.go` in Sprint 1 final / Sprint 2.
