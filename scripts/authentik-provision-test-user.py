"""Provision an Authentik internal user for the OIDC e2e test.

Idempotent: re-running finds the existing user, resets its password to
the well-known constant below, and prints the username. The password is
hard-coded because Authentik is bound to 127.0.0.1 in dev and this user
exists solely to drive scripts/authentik-provision-test-user.py-paired
end-to-end coverage of the /auth/login -> /auth/callback flow.

Run from the host:
    docker cp scripts/authentik-provision-test-user.py \\
        lecrm-authentik-worker:/tmp/provision-user.py
    docker exec lecrm-authentik-worker \\
        ak shell -c "exec(open('/tmp/provision-user.py').read())"
"""
from authentik.core.models import User, UserTypes

USERNAME = "guillaume-e2e"
EMAIL = "guillaume-e2e@example.com"
NAME = "Guillaume E2E"
PASSWORD = "e2etest-changeme"  # dev-only fixture; Authentik is bound to 127.0.0.1

user, created = User.objects.get_or_create(
    username=USERNAME,
    defaults={
        "email": EMAIL,
        "name": NAME,
        "type": UserTypes.INTERNAL,
        "is_active": True,
    },
)
user.email = EMAIL
user.name = NAME
user.type = UserTypes.INTERNAL
user.is_active = True
user.set_password(PASSWORD)
user.save()

print("OK")
print(f"USERNAME={user.username}")
print(f"EMAIL={user.email}")
print(f"CREATED={created}")
print(f"PASSWORD={PASSWORD}")
