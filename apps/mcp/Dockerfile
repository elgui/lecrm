# syntax=docker/dockerfile:1.7
#
# Multi-stage release image for the lecrm-mcp binary — the Model Context
# Protocol adapter (ADR-011 AI-native interface seam).
#
# The MCP module depends only on pgx + google/uuid, so the build is
# self-contained (no workspace siblings, no SPA). GOWORK=off keeps the
# repo's go.work out of the single-module build.

# ----------------------------------------------------------- go build
FROM golang:1.25-alpine AS go

WORKDIR /workspace
ENV GOWORK=off

# Module manifest first for layer caching.
COPY apps/mcp/go.mod apps/mcp/go.sum ./apps/mcp/
RUN cd apps/mcp && go mod download

# Source.
COPY apps/mcp/ ./apps/mcp/

# Static binary so the distroless:static runtime can run it without libc.
RUN cd apps/mcp/cmd/lecrm-mcp \
 && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -trimpath -ldflags="-s -w" -o /out/lecrm-mcp .

# ---------------------------------------------------------------- final
FROM gcr.io/distroless/static:nonroot AS final

WORKDIR /

COPY --from=go /out/lecrm-mcp /lecrm-mcp

USER nonroot:nonroot
EXPOSE 8081

ENTRYPOINT ["/lecrm-mcp"]
