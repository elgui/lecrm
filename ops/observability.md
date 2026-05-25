# Observability — v0 posture

Decided in the council architecture review (2026-05-24). Self-hosted
LGTM (~1.1 GB RAM) is deferred; v0 ships structured logging only.

## v0: structured slog to stdout + Grafana Cloud free tier

The API binary (`lecrm-api`) emits structured JSON via Go's `log/slog`
to stdout. Every log line includes:

| Field          | Source                  | Always present |
|----------------|-------------------------|----------------|
| `request_id`   | chi middleware           | yes            |
| `method`       | HTTP request             | yes            |
| `path`         | HTTP request             | yes            |
| `host`         | HTTP request             | yes            |
| `status`       | response wrapper         | yes            |
| `bytes`        | response wrapper         | yes            |
| `ms`           | elapsed time             | yes            |
| `workspace`    | workspace middleware     | when in scope  |
| `workspace_id` | workspace middleware     | when in scope  |

Downstream handlers can obtain a workspace-enriched logger via
`logging.FromContext(ctx)`.

### Log shipping (production)

Grafana Cloud free tier: 50 GB logs/month, 10k series metrics.

Option A — Grafana Alloy (recommended, ~50 MB RAM):
```yaml
# deploy/compose/alloy.yml (create when shipping to prod)
services:
  alloy:
    image: grafana/alloy:latest
    volumes:
      - /var/log:/var/log:ro
      - ./alloy-config.alloy:/etc/alloy/config.alloy:ro
    environment:
      GRAFANA_CLOUD_API_KEY: ${GRAFANA_CLOUD_API_KEY}
      GRAFANA_CLOUD_LOKI_URL: ${GRAFANA_CLOUD_LOKI_URL}
    restart: unless-stopped
```

Option B — minimal (docker logs piped to Promtail):
```bash
docker logs -f lecrm-api | promtail \
  --client.url="${GRAFANA_CLOUD_LOKI_URL}" \
  --stdin
```

### Grafana Cloud setup

1. Create a free account at grafana.com
2. Navigate to Connections > Hosted Logs (Loki)
3. Generate an API key with `logs:write` scope
4. Set `GRAFANA_CLOUD_API_KEY` and `GRAFANA_CLOUD_LOKI_URL` in your env

### Local development

Logs go to stdout; use `jq` to filter:
```bash
./lecrm-api 2>&1 | jq 'select(.workspace == "acme")'
./lecrm-api 2>&1 | jq 'select(.status >= 400)'
```

For deep local debugging, the LGTM stack is still available:
```bash
docker compose -f deploy/compose/lgtm.yml up -d
```

## v1: self-hosted LGTM (>20 workspaces)

Reactivate `deploy/compose/lgtm.yml` when:
- Grafana Cloud free tier limits are hit (50 GB/month)
- Per-tenant anomaly detection requires Prometheus metrics
- The VPS has headroom for ~1.1 GB additional RAM

At v1 the LGTM stack provides:
- Loki for log aggregation with `workspace_id` labels
- Grafana dashboards for per-tenant metrics
- Tempo for distributed tracing
- Prometheus for metrics and alerting
- OTel Collector for telemetry pipeline

## Decision rationale

The council (2026-05-24) noted that Pennylane shipped Day-1
observability with Datadog SaaS, not self-hosted. At v0 with <5 clients,
the ~1.1 GB RAM overhead of self-hosted LGTM is unjustifiable on a
single VPS also running Postgres, Caddy, and Authentik. Structured slog
JSON + Grafana Cloud free tier provides sufficient observability until
the workspace count justifies the resource cost.
