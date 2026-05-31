# syntax=docker/dockerfile:1.7

# lecrm-admin — distroless build per AC-D5 (Story 8.1). Single static Go
# binary, no shell, no package manager. The image refuses to start if
# LECRM_API_* env vars are present (defense-in-depth against same-image
# binary co-location, per safety/guard.go).

# ---- builder ----------------------------------------------------------
FROM golang:1.25 AS build
WORKDIR /src

# Copy the go.work + module sources. The whole monorepo is the build
# context so apps/admin can resolve sibling modules via go.work.
COPY go.work go.work.sum* ./
COPY apps/admin/go.mod apps/admin/go.sum* apps/admin/
COPY apps/api/go.mod apps/api/go.sum* apps/api/
COPY apps/mcp/go.mod apps/mcp/go.sum* apps/mcp/
COPY apps/migrate/go.mod apps/migrate/go.sum* apps/migrate/

# Pre-fetch admin module deps. The other modules' deps are not needed
# for the admin binary but go.work references them so we mirror the layout.
RUN cd apps/admin && go mod download

# Bring in sources (admin only — the other binaries build separately).
COPY apps/admin apps/admin

# Static linking + stripped binary. CGO disabled to keep distroless happy.
ENV CGO_ENABLED=0
RUN cd apps/admin && go build \
    -trimpath \
    -ldflags="-s -w" \
    -o /out/lecrm-admin ./cmd/lecrm-admin

# ---- runtime ----------------------------------------------------------
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/lecrm-admin /app/lecrm-admin

# AC-D3 — proxy disabled at Dokku level; no HTTP listener here.
# AC-D5  — safety/guard.go runs on every invocation, no entry shim needed.
USER nonroot:nonroot
ENTRYPOINT ["/app/lecrm-admin"]
