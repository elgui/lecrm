"""Brand the Authentik login screen for leCRM.

Out of the box Authentik shows its own identity on the login page: the
orange "authentik" wordmark (the `branding_logo`), the title "Welcome to
authentik!" (the authentication flow's `title`), and a stock aerial-road
photo background. For an outward-facing demo that is an off-brand first
touch — the literal first screen a user sees should say leCRM, not
authentik.

This script rewrites the visible identity on every Brand plus the default
authentication flow:

  * branding_logo                  -> leCRM wordmark (SVG data URI)
  * branding_favicon               -> leCRM square mark (SVG data URI)
  * branding_title                 -> "leCRM"
  * branding_default_flow_background-> branded blue gradient (SVG data URI)
  * default-authentication-flow.title -> "Bienvenue sur leCRM"

The assets are inline SVGs base64-encoded into `data:` URIs at runtime, so
the script is fully self-contained — no media-volume file copy, nothing to
lose. It is idempotent (re-running just re-applies the same values) and
version-defensive (only fields the running Authentik actually has are
written), so it carries cleanly to the Hetzner migration on a fresh
Authentik.

Colours mirror the leCRM web app (`apps/web/src/index.css`):
  primary  #2563EB   secondary text #33475B   canvas #F5F8FA

Run from the host (container `lecrm-authentik-worker`):

    docker cp scripts/authentik-brand-lecrm.py \\
        lecrm-authentik-worker:/tmp/brand.py
    docker exec lecrm-authentik-worker \\
        ak shell -c "exec(open('/tmp/brand.py').read())"
"""
import base64

from authentik.brands.models import Brand
from authentik.flows.models import Flow

PRIMARY = "#2563EB"  # leCRM primary (apps/web/src/index.css --primary)
SLATE = "#33475B"  # leCRM secondary text
DEEP = "#1E3A8A"  # blue-900, gradient foot

BRANDING_TITLE = "leCRM"
FLOW_TITLE = "Bienvenue sur leCRM"
AUTH_FLOW_SLUG = "default-authentication-flow"

FONT = "Inter,Segoe UI,Helvetica,Arial,sans-serif"

# leCRM wordmark: blue rounded tile + "le", then slate "CRM".
LOGO_SVG = f"""<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 210 56" \
width="210" height="56" role="img" aria-label="leCRM">
  <rect x="0" y="0" width="56" height="56" rx="13" fill="{PRIMARY}"/>
  <text x="28" y="39" font-family="{FONT}" font-size="29" font-weight="700" \
fill="#ffffff" text-anchor="middle">le</text>
  <text x="70" y="39" font-family="{FONT}" font-size="29" font-weight="700" \
fill="{SLATE}">CRM</text>
</svg>"""

# Square favicon / app mark.
FAVICON_SVG = f"""<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 64 64" \
width="64" height="64" role="img" aria-label="leCRM">
  <rect width="64" height="64" rx="15" fill="{PRIMARY}"/>
  <text x="32" y="45" font-family="{FONT}" font-size="34" font-weight="700" \
fill="#ffffff" text-anchor="middle">le</text>
</svg>"""

# Calm branded gradient that replaces the stock aerial-road photo.
BACKGROUND_SVG = f"""<svg xmlns="http://www.w3.org/2000/svg" \
viewBox="0 0 1600 900" width="1600" height="900" \
preserveAspectRatio="xMidYMid slice">
  <defs>
    <linearGradient id="g" x1="0" y1="0" x2="1" y2="1">
      <stop offset="0" stop-color="{PRIMARY}"/>
      <stop offset="1" stop-color="{DEEP}"/>
    </linearGradient>
  </defs>
  <rect width="1600" height="900" fill="url(#g)"/>
</svg>"""


def svg_data_uri(svg: str) -> str:
    """base64 `data:` URI — renders both as <img src> and CSS url()."""
    encoded = base64.b64encode(svg.strip().encode("utf-8")).decode("ascii")
    return f"data:image/svg+xml;base64,{encoded}"


def has_field(obj, name: str) -> bool:
    return any(f.name == name for f in obj._meta.get_fields())


LOGO_URI = svg_data_uri(LOGO_SVG)
FAVICON_URI = svg_data_uri(FAVICON_SVG)
BACKGROUND_URI = svg_data_uri(BACKGROUND_SVG)

# Field -> value, applied only when the running Authentik has the field.
BRAND_FIELDS = {
    "branding_title": BRANDING_TITLE,
    "branding_logo": LOGO_URI,
    "branding_favicon": FAVICON_URI,
    "branding_default_flow_background": BACKGROUND_URI,
}

# 1. Rebrand every Brand. A fresh Authentik ships one default brand
#    (domain "authentik-default"); create it if somehow absent so the
#    script is safe on a bare install.
brands = list(Brand.objects.all())
if not brands:
    brands = [Brand.objects.create(domain="authentik-default", default=True)]

for brand in brands:
    applied = []
    for field, value in BRAND_FIELDS.items():
        if has_field(brand, field):
            setattr(brand, field, value)
            applied.append(field)
    brand.save()
    print(f"BRAND domain={brand.domain!r} default={brand.default} "
          f"applied={applied}")

# 2. Drop "Welcome to authentik!" from the authentication flow title.
try:
    flow = Flow.objects.get(slug=AUTH_FLOW_SLUG)
    flow.title = FLOW_TITLE
    flow.save()
    print(f"FLOW slug={AUTH_FLOW_SLUG!r} title={flow.title!r}")
except Flow.DoesNotExist:
    # Non-fatal: the brand rebrand above already removes the visible
    # authentik wordmark; the flow title is a secondary touch.
    print(f"WARN flow {AUTH_FLOW_SLUG!r} not found — skipped title rebrand")

print("OK")
print(f"BRANDS_UPDATED={len(brands)}")
print(f"LOGO_BYTES={len(LOGO_URI)}")
