# syntax=docker/dockerfile:1.7
#
# Multi-stage release image for the lecrm-mcp binary — the Model Context
# Protocol adapter (ADR-011 AI-native interface seam).
#
# The MCP adapter links the shared capability layer (ADR-012 §1), which
# lives in the sibling apps/api module. apps/mcp/go.mod resolves it with a
# local `replace => ../api`, so the build still runs as a single module
# (GOWORK=off keeps the repo's go.work out of it) but must include the
# apps/api source tree. The compiled surface is tiny — capability +
# sqlcgen + domain pull in only pgx + google/uuid + golang.org/x/* — so the
# separate-binary topology (ADR-009 §4.2) is preserved.

# ----------------------------------------------------------- go build
FROM golang:1.25-alpine AS go

WORKDIR /workspace
ENV GOWORK=off

# Module manifests first for layer caching. apps/api is the replace target.
COPY apps/mcp/go.mod apps/mcp/go.sum ./apps/mcp/
COPY apps/api/go.mod apps/api/go.sum ./apps/api/
RUN cd apps/mcp && go mod download

# Source: the MCP module plus the capability layer it links from apps/api.
COPY apps/mcp/ ./apps/mcp/
COPY apps/api/ ./apps/api/

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
