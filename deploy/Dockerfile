# syntax=docker/dockerfile:1.7
#
# Multi-stage release image for the lecrm-api binary.
#   1. web    — Vite builds the React SPA into apps/web/dist/
#   2. go     — Go compiles the API; //go:embed pulls in internal/spa/dist/
#   3. final  — gcr.io/distroless/static:nonroot — minimal runtime
#
# Per ADR-009 §5.1: the SPA is embedded; one binary serves /v1/* + /*.

# -------------------------------------------------------------- 1. web build
FROM oven/bun:1-alpine AS web

WORKDIR /workspace/apps/web

# Install dependencies first for layer caching.
COPY apps/web/package.json apps/web/bun.lock* ./
RUN bun install --frozen-lockfile

# Copy the rest of the SPA source and build.
COPY apps/web/ ./
RUN bun run build

# ----------------------------------------------------------- 2. go API build
FROM golang:1.25-alpine AS go

# buildkit-provided target arch (e.g. amd64, arm64). BuildKit always sets this
# from the build host (or --platform), so a plain build on the arm64 Netcup box
# produces an arm64 binary and an amd64 host produces amd64. It is only ever
# empty under the legacy builder (DOCKER_BUILDKIT=0), which silently ignores the
# `# syntax=` directive above — the build fails loudly there (see GOARCH below)
# rather than guessing an arch and shipping an unrunnable binary.
ARG TARGETARCH

WORKDIR /workspace

# Module manifest first for layer caching.
COPY apps/api/go.mod apps/api/go.sum ./apps/api/
RUN cd apps/api && go mod download

# Now bring in the rest of the source.
COPY apps/api/ ./apps/api/
COPY packages/ ./packages/

# Bring in the freshly built SPA so `//go:embed all:dist` (in
# internal/spa) pulls it into the binary.
COPY --from=web /workspace/apps/web/dist/ ./apps/api/internal/spa/dist/

# Build the static binary. CGO_ENABLED=0 keeps the binary fully static
# so the distroless:static image can run it without a libc.
RUN cd apps/api/cmd/lecrm-api \
 && CGO_ENABLED=0 GOOS=linux GOARCH=${TARGETARCH:?TARGETARCH is empty — build with BuildKit (DOCKER_BUILDKIT=1 / compose v2); the legacy builder cannot supply the target arch} \
    go build -trimpath -ldflags="-s -w" -o /out/lecrm-api .

# ---------------------------------------------------------------- 3. final
FROM gcr.io/distroless/static:nonroot AS final

WORKDIR /

COPY --from=go /out/lecrm-api /lecrm-api

USER nonroot:nonroot
EXPOSE 8080

ENTRYPOINT ["/lecrm-api"]
