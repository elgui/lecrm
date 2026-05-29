# Custom Caddy build for the leCRM edge.
#
# The stock Caddy binary has no DNS provider modules. The DNS-01 ACME
# challenge for the per-workspace wildcard cert needs the registrar's DNS
# API plugged in at build time via xcaddy.
#
# Provider is domain-dependent (see deploy/README.md decision log):
#   - production  *.lecrm.fr            -> caddy-dns/cloudflare
#   - staging     *.lecrm.gbconsult.me  -> caddy-dns/ovh   (gbconsult.me is on OVH DNS)
#
# This image bundles BOTH providers so the same image serves prod and
# staging; the active provider is selected by the mounted Caddyfile.
FROM caddy:2.10-builder AS builder
RUN xcaddy build \
    --with github.com/caddy-dns/ovh \
    --with github.com/caddy-dns/cloudflare

FROM caddy:2.10
COPY --from=builder /usr/bin/caddy /usr/bin/caddy
