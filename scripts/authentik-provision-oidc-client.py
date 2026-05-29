"""Provision the lecrm OIDC client in Authentik.

Idempotent: re-running finds the existing provider by name and prints
its client_id/client_secret. RedirectURI rows are persisted as JSON in
the private _redirect_uris field per Authentik 2025.10 schema.
"""
import os
import secrets

from authentik.core.models import Application, Token, TokenIntents, User
from authentik.crypto.models import CertificateKeyPair
from authentik.flows.models import Flow
from authentik.providers.oauth2.models import (
    ClientTypes,
    OAuth2Provider,
    RedirectURI,
    RedirectURIMatchingMode,
    ScopeMapping,
)

CLIENT_ID = "lecrm-api"

# 1. Admin API token for the akadmin user (optional dev convenience).
akadmin = User.objects.get(username="akadmin")
api_token, _ = Token.objects.get_or_create(
    identifier="lecrm-dev-admin",
    defaults={
        "user": akadmin,
        "intent": TokenIntents.INTENT_API,
        "expiring": False,
    },
)
api_token.user = akadmin
api_token.intent = TokenIntents.INTENT_API
api_token.expiring = False
api_token.save()

# 2. OAuth2 provider.
auth_flow = Flow.objects.get(slug="default-authentication-flow")
authz_flow = Flow.objects.get(slug="default-provider-authorization-implicit-consent")
invalidation_flow = Flow.objects.get(slug="default-provider-invalidation-flow")
signing_key = CertificateKeyPair.objects.filter(
    name__startswith="authentik Self-signed"
).first()

provider, _ = OAuth2Provider.objects.get_or_create(
    name="lecrm",
    defaults={
        "client_type": ClientTypes.CONFIDENTIAL,
        "client_id": CLIENT_ID,
        "client_secret": secrets.token_urlsafe(48),
        "authorization_flow": authz_flow,
        "authentication_flow": auth_flow,
        "invalidation_flow": invalidation_flow,
        "signing_key": signing_key,
    },
)
provider.authorization_flow = authz_flow
provider.authentication_flow = auth_flow
provider.invalidation_flow = invalidation_flow
provider.signing_key = signing_key
provider.client_id = CLIENT_ID

# redirect_uris is a property: getter dataclass-decodes _redirect_uris,
# setter dataclass-encodes it. Use the dataclass.
#
# The regex matches any workspace subdomain's /auth/callback. Override via
# LECRM_OIDC_REDIRECT_URI_REGEX for other environments — e.g. staging:
#   ^https://[a-z0-9-]+\.lecrm\.gbconsult\.me/auth/callback$
# Default is local dev (lecrm.test:8080). Covers demo + every wildcard
# workspace subdomain in one regex row.
redirect_regex = os.environ.get(
    "LECRM_OIDC_REDIRECT_URI_REGEX",
    r"^http://[a-z0-9-]+\.lecrm\.test:8080/auth/callback$",
)
provider.redirect_uris = [
    RedirectURI(
        matching_mode=RedirectURIMatchingMode.REGEX,
        url=redirect_regex,
    )
]
provider.save()

# Attach the standard openid/profile/email scope mappings so the ID
# token carries `sub`, `name`, and `email` claims. Without these, the
# provider mints tokens with empty claims and our (issuer, subject)
# tuple is the only identifier the relying party sees.
scope_names = ["openid", "email", "profile"]
mappings = ScopeMapping.objects.filter(scope_name__in=scope_names)
if mappings.count() != len(scope_names):
    found = set(mappings.values_list("scope_name", flat=True))
    missing = set(scope_names) - found
    raise SystemExit(f"missing default ScopeMappings: {missing}")
provider.property_mappings.set(mappings)
provider.save()

# 3. Application bound to the provider.
app, _ = Application.objects.get_or_create(
    slug="lecrm",
    defaults={"name": "leCRM", "provider": provider},
)
app.name = "leCRM"
app.provider = provider
app.save()

print("OK")
print(f"CLIENT_ID={provider.client_id}")
print(f"CLIENT_SECRET={provider.client_secret}")
print(f"API_TOKEN_KEY={api_token.key}")
