# syntax=docker/dockerfile:1.7
#
# Multi-stage release image for the lecrm-api binary.
#   1. web    — Vite builds the React SPA into apps/web/dist/
#   2. go     — Go compiles the API; //go:embed pulls in apps/web/dist/
#   3. final  — gcr.io/distroless/static:nonroot — minimal runtime
#
# Per ADR-009 §5.1: the SPA is embedded; one binary serves /v1/* + /*.

# -------------------------------------------------------------- 1. web build
FROM node:20-alpine AS web

RUN corepack enable

WORKDIR /workspace/apps/web

# Install dependencies first for layer caching.
COPY apps/web/package.json apps/web/pnpm-lock.yaml* ./
RUN pnpm install --frozen-lockfile=false

# Copy the rest of the SPA source and build.
COPY apps/web/ ./
RUN pnpm build

# ----------------------------------------------------------- 2. go API build
FROM golang:1.25-alpine AS go

WORKDIR /workspace

# Module manifest first for layer caching.
COPY apps/api/go.mod apps/api/go.sum ./apps/api/
RUN cd apps/api && go mod download

# Now bring in the rest of the source.
COPY apps/api/ ./apps/api/
COPY packages/ ./packages/

# Bring in the freshly built SPA so //go:embed dist/* finds it.
COPY --from=web /workspace/apps/web/dist/ ./apps/web/dist/

# Build the static binary. CGO_ENABLED=0 keeps the binary fully static
# so the distroless:static image can run it without a libc.
RUN cd apps/api/cmd/lecrm-api \
 && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -trimpath -ldflags="-s -w" -o /out/lecrm-api .

# ---------------------------------------------------------------- 3. final
FROM gcr.io/distroless/static:nonroot AS final

WORKDIR /

COPY --from=go /out/lecrm-api /lecrm-api

USER nonroot:nonroot
EXPOSE 8080

ENTRYPOINT ["/lecrm-api"]
