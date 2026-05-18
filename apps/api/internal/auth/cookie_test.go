package auth

import (
	"net/http"
	"strings"
	"testing"
)

// The single most important property: cookies issued by the auth package
// MUST carry a per-workspace Domain (e.g. "acme.lecrm.fr"), never the
// parent domain ("lecrm.fr"). A wildcard parent-domain cookie would let
// a logged-in user of workspace A authenticate as themselves on
// workspace B — the canonical multi-tenant cookie-leak. ADR-009 §5.2.

func TestBuildSessionCookie_ScopesToWorkspaceSubdomain(t *testing.T) {
	c, err := BuildSessionCookie(CookieScope{
		WorkspaceSubdomain: "acme",
		DomainTLD:          "lecrm.fr",
		Secure:             true,
	}, "signed.value")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.Domain != "acme.lecrm.fr" {
		t.Fatalf("Domain = %q, want %q", c.Domain, "acme.lecrm.fr")
	}
	if !c.HttpOnly {
		t.Fatal("HttpOnly must be true")
	}
	if !c.Secure {
		t.Fatal("Secure must be true when CookieScope.Secure=true")
	}
	if c.SameSite != http.SameSiteStrictMode {
		t.Fatalf("SameSite = %v, want Strict", c.SameSite)
	}
	if c.Path != "/" {
		t.Fatalf("Path = %q, want %q", c.Path, "/")
	}
}

func TestBuildSessionCookie_RejectsParentDomainWildcards(t *testing.T) {
	cases := []struct {
		name string
		s    CookieScope
	}{
		{"empty subdomain", CookieScope{WorkspaceSubdomain: "", DomainTLD: "lecrm.fr"}},
		{"subdomain contains dot", CookieScope{WorkspaceSubdomain: "acme.evil", DomainTLD: "lecrm.fr"}},
		{"subdomain contains star", CookieScope{WorkspaceSubdomain: "*", DomainTLD: "lecrm.fr"}},
		{"TLD with leading dot", CookieScope{WorkspaceSubdomain: "acme", DomainTLD: ".lecrm.fr"}},
		{"empty TLD", CookieScope{WorkspaceSubdomain: "acme", DomainTLD: ""}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := BuildSessionCookie(tc.s, "v")
			if err == nil {
				t.Fatalf("expected error for %#v, got nil", tc.s)
			}
		})
	}
}

func TestBuildSessionCookie_ProducedDomainNeverEqualsTLD(t *testing.T) {
	// Regression: even with valid inputs, the produced Domain must
	// always include the subdomain — Domain == DomainTLD would be a
	// silent parent-domain leak.
	c, err := BuildSessionCookie(CookieScope{
		WorkspaceSubdomain: "anything",
		DomainTLD:          "lecrm.fr",
		Secure:             false,
	}, "v")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.Domain == "lecrm.fr" || !strings.HasPrefix(c.Domain, "anything.") {
		t.Fatalf("cookie Domain %q must be a workspace subdomain of lecrm.fr", c.Domain)
	}
}

func TestSubdomainFromHost(t *testing.T) {
	cases := []struct {
		host    string
		tld     string
		want    string
		wantErr bool
	}{
		{"acme.lecrm.fr", "lecrm.fr", "acme", false},
		{"ACME.lecrm.fr", "lecrm.fr", "acme", false},
		{"acme.lecrm.fr:8080", "lecrm.fr", "acme", false},
		{"acme.lecrm.test", "lecrm.test", "acme", false},
		{"lecrm.fr", "lecrm.fr", "", true},                   // no subdomain
		{"a.b.lecrm.fr", "lecrm.fr", "", true},               // two-level subdomain — workspaces are exactly one level
		{"acme.evil.example", "lecrm.fr", "", true},          // not our domain
		{"", "lecrm.fr", "", true},
	}
	for _, tc := range cases {
		t.Run(tc.host, func(t *testing.T) {
			got, err := SubdomainFromHost(tc.host, tc.tld)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got %q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestClearSessionCookie_MatchesScopedSetCookie(t *testing.T) {
	// The clearing cookie's Domain MUST match the set cookie or the
	// browser ignores the deletion.
	s := CookieScope{WorkspaceSubdomain: "acme", DomainTLD: "lecrm.fr", Secure: true}
	set, _ := BuildSessionCookie(s, "v")
	clr, err := ClearSessionCookie(s)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if clr.Domain != set.Domain {
		t.Fatalf("clear Domain %q != set Domain %q", clr.Domain, set.Domain)
	}
	if clr.MaxAge >= 0 {
		t.Fatalf("clear MaxAge = %d, want < 0", clr.MaxAge)
	}
}

// TestCrossWorkspaceCookieLeakRejected is the ADR-009 §5.2 cross-tenant
// isolation proof: a session cookie issued for workspace "acme" carries
// Domain=acme.lecrm.fr; a cookie issued for workspace "other" carries
// Domain=other.lecrm.fr. These are distinct domains — the browser will
// never send "acme"'s cookie on a request to "other.lecrm.fr" (and
// vice versa). Any attempt to hand-roll a parent-domain cookie is rejected
// by BuildSessionCookie before any value is set.
func TestCrossWorkspaceCookieLeakRejected(t *testing.T) {
	const tld = "lecrm.fr"

	acmeCookie, err := BuildSessionCookie(CookieScope{
		WorkspaceSubdomain: "acme",
		DomainTLD:          tld,
		Secure:             true,
	}, "acme-session-value")
	if err != nil {
		t.Fatalf("build acme cookie: %v", err)
	}

	otherCookie, err := BuildSessionCookie(CookieScope{
		WorkspaceSubdomain: "other",
		DomainTLD:          tld,
		Secure:             true,
	}, "other-session-value")
	if err != nil {
		t.Fatalf("build other cookie: %v", err)
	}

	if acmeCookie.Domain == otherCookie.Domain {
		t.Fatalf("cross-workspace leak: both workspaces issued the same cookie Domain %q", acmeCookie.Domain)
	}
	if acmeCookie.Domain != "acme.lecrm.fr" {
		t.Fatalf("acme cookie Domain = %q, want %q", acmeCookie.Domain, "acme.lecrm.fr")
	}
	if otherCookie.Domain != "other.lecrm.fr" {
		t.Fatalf("other cookie Domain = %q, want %q", otherCookie.Domain, "other.lecrm.fr")
	}

	// Confirm neither domain equals the parent TLD (wildcard leak guard).
	if acmeCookie.Domain == tld {
		t.Fatalf("acme cookie Domain leaked to parent %q", tld)
	}
	if otherCookie.Domain == tld {
		t.Fatalf("other cookie Domain leaked to parent %q", tld)
	}
}
