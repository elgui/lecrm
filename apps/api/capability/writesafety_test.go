package capability

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

// fixedClock returns a deterministic Now() so the confirmation handshake is
// testable without sleeping or touching the wall clock.
func fixedClock(t time.Time) func() time.Time { return func() time.Time { return t } }

// --- scope → role mapping (ADR-012 §6) ---

func TestRoleFromScopes(t *testing.T) {
	cases := []struct {
		name   string
		scopes []string
		want   Role
	}{
		{"wildcard grants admin", []string{ScopeWildcard}, RoleAdmin},
		{"explicit write grants admin", []string{ScopeCRMWrite}, RoleAdmin},
		{"any write substring grants admin", []string{"deals:write"}, RoleAdmin},
		{"mixed read+write grants admin", []string{ScopeCRMRead, ScopeCRMWrite}, RoleAdmin},
		{"read only resolves to member", []string{ScopeCRMRead}, RoleMember},
		{"empty resolves to member", nil, RoleMember},
		{"unrecognised resolves to member", []string{"something:else"}, RoleMember},
		{"case + whitespace tolerant", []string{"  CRM:WRITE "}, RoleAdmin},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := RoleFromScopes(tc.scopes); got != tc.want {
				t.Fatalf("RoleFromScopes(%v) = %v, want %v", tc.scopes, got, tc.want)
			}
		})
	}
}

// RoleFromScopes must mirror rbac.roleFromScopes verbatim in policy so a single
// token authorizes identically against REST and MCP (ADR-012 §1, §6). We can't
// import rbac (net/http dependency; see capability.go), so we re-assert the
// policy contract here as the canonical guard.
func TestRoleFromScopes_MirrorsRBACPolicy(t *testing.T) {
	// "*" or any scope containing "write" → admin; everything else → member.
	if RoleFromScopes([]string{"*"}) != RoleAdmin {
		t.Fatal("wildcard must map to admin (matches rbac.roleFromScopes)")
	}
	if RoleFromScopes([]string{"contacts:write"}) != RoleAdmin {
		t.Fatal("write substring must map to admin (matches rbac.roleFromScopes)")
	}
	if RoleFromScopes([]string{"contacts:read"}) != RoleMember {
		t.Fatal("read-only must map to member (matches rbac.roleFromScopes)")
	}
}

// --- AuthorizeWrite: the single write gate (ADR-012 §6, §7, §8) ---

func TestAuthorizeWrite_ReadOnlyScopeDenied(t *testing.T) {
	// A token-based caller (carries scopes) with read-only scope is the
	// primary blast-radius control: it must never reach a write op (§8).
	p := Principal{
		WorkspaceID: uuid.New(),
		Role:        RoleMember,
		ActorType:   ActorTypeMCPAgent,
		Scopes:      []string{ScopeCRMRead},
	}
	err := AuthorizeWrite(p)
	if !errors.Is(err, ErrReadOnlyScope) {
		t.Fatalf("read-only scope: got %v, want ErrReadOnlyScope", err)
	}
}

func TestAuthorizeWrite_WriteScopeAllowed(t *testing.T) {
	p := Principal{
		WorkspaceID: uuid.New(),
		Role:        RoleAdmin,
		ActorType:   ActorTypeMCPAgent,
		Scopes:      []string{ScopeCRMWrite},
	}
	if err := AuthorizeWrite(p); err != nil {
		t.Fatalf("write scope should authorize, got %v", err)
	}
}

func TestAuthorizeWrite_WildcardAllowed(t *testing.T) {
	p := Principal{
		WorkspaceID: uuid.New(),
		Role:        RoleAdmin,
		ActorType:   ActorTypeMCPAgent,
		Scopes:      []string{ScopeWildcard},
	}
	if err := AuthorizeWrite(p); err != nil {
		t.Fatalf("wildcard scope should authorize, got %v", err)
	}
}

// A scope-bearing caller whose scopes grant write but whose membership Role is
// somehow below admin is still denied by the role gate — belt-and-suspenders.
func TestAuthorizeWrite_RoleGateStillApplies(t *testing.T) {
	p := Principal{
		WorkspaceID: uuid.New(),
		Role:        RoleMember, // below RoleAdmin
		ActorType:   ActorTypeMCPAgent,
		Scopes:      []string{ScopeCRMWrite},
	}
	// Scope resolves to admin, so the scope gate passes; the role gate must
	// then reject because membership Role is only RoleMember.
	if err := AuthorizeWrite(p); !errors.Is(err, ErrForbidden) {
		t.Fatalf("insufficient role: got %v, want ErrForbidden", err)
	}
}

func TestAuthorizeWrite_NoPrincipalUnauthenticated(t *testing.T) {
	if err := AuthorizeWrite(Principal{}); !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("empty principal: got %v, want ErrUnauthenticated", err)
	}
}

// A scope-less Principal (a human admin session authorizing by membership role)
// skips the scope gate and authorizes purely on Role — mechanism-agnostic (§7).
func TestAuthorizeWrite_ScopelessHumanAdminAllowed(t *testing.T) {
	p := Principal{
		WorkspaceID: uuid.New(),
		Role:        RoleAdmin,
		ActorType:   ActorTypeHumanAPI,
		// no Scopes
	}
	if err := AuthorizeWrite(p); err != nil {
		t.Fatalf("scope-less human admin should authorize, got %v", err)
	}
}

// --- confirmation-token handshake (ADR-012 §6, TO RESOLVE 4) ---

func newTestConfirmer(t *testing.T) *Confirmer {
	t.Helper()
	c, err := NewConfirmer([]byte("test-signing-secret-not-for-prod"))
	if err != nil {
		t.Fatalf("NewConfirmer: %v", err)
	}
	return c
}

func TestNewConfirmer_RejectsEmptySecret(t *testing.T) {
	if _, err := NewConfirmer(nil); err == nil {
		t.Fatal("empty secret must be rejected (fail closed)")
	}
}

func TestConfirmer_IssueVerifyRoundTrip(t *testing.T) {
	c := newTestConfirmer(t)
	ws := uuid.New()
	now := time.Unix(1_700_000_000, 0)
	digest := ComputeEffectDigest("advance_deal", []Effect{{Action: "stage_change", EntityType: "deal", EntityID: "d1"}})

	tok := c.Issue(ws, "advance_deal", digest, now)
	if err := c.Verify(tok, ws, "advance_deal", digest, now); err != nil {
		t.Fatalf("fresh token should verify, got %v", err)
	}
}

func TestConfirmer_VerifyRejectsExpired(t *testing.T) {
	c := newTestConfirmer(t)
	ws := uuid.New()
	now := time.Unix(1_700_000_000, 0)
	digest := ComputeEffectDigest("op", nil)
	tok := c.Issue(ws, "op", digest, now)

	later := now.Add(confirmTokenTTL + time.Second)
	if err := c.Verify(tok, ws, "op", digest, later); !errors.Is(err, ErrConfirmationInvalid) {
		t.Fatalf("expired token: got %v, want ErrConfirmationInvalid", err)
	}
}

func TestConfirmer_VerifyRejectsWrongWorkspace(t *testing.T) {
	c := newTestConfirmer(t)
	now := time.Unix(1_700_000_000, 0)
	digest := ComputeEffectDigest("op", nil)
	tok := c.Issue(uuid.New(), "op", digest, now)

	if err := c.Verify(tok, uuid.New(), "op", digest, now); !errors.Is(err, ErrConfirmationInvalid) {
		t.Fatalf("wrong workspace: got %v, want ErrConfirmationInvalid", err)
	}
}

func TestConfirmer_VerifyRejectsWrongOperation(t *testing.T) {
	c := newTestConfirmer(t)
	ws := uuid.New()
	now := time.Unix(1_700_000_000, 0)
	digest := ComputeEffectDigest("op-a", nil)
	tok := c.Issue(ws, "op-a", digest, now)

	if err := c.Verify(tok, ws, "op-b", digest, now); !errors.Is(err, ErrConfirmationInvalid) {
		t.Fatalf("wrong operation: got %v, want ErrConfirmationInvalid", err)
	}
}

// The token binds to the planned effect: if the would-be effect changed between
// dry-run and confirm (underlying rows mutated), the digest differs and the
// confirm fails, forcing a fresh dry-run.
func TestConfirmer_VerifyRejectsMutatedEffect(t *testing.T) {
	c := newTestConfirmer(t)
	ws := uuid.New()
	now := time.Unix(1_700_000_000, 0)
	d1 := ComputeEffectDigest("op", []Effect{{Action: "update", EntityType: "deal", After: map[string]any{"stage": "won"}}})
	d2 := ComputeEffectDigest("op", []Effect{{Action: "update", EntityType: "deal", After: map[string]any{"stage": "lost"}}})
	tok := c.Issue(ws, "op", d1, now)

	if err := c.Verify(tok, ws, "op", d2, now); !errors.Is(err, ErrConfirmationInvalid) {
		t.Fatalf("mutated effect: got %v, want ErrConfirmationInvalid", err)
	}
}

func TestConfirmer_VerifyRejectsMalformed(t *testing.T) {
	c := newTestConfirmer(t)
	for _, bad := range []string{"", "garbage", "v1.notanumber.sig", "v2.123.sig", "a.b.c.d"} {
		if err := c.Verify(bad, uuid.New(), "op", "digest", time.Unix(1, 0)); !errors.Is(err, ErrConfirmationInvalid) {
			t.Fatalf("malformed token %q: got %v, want ErrConfirmationInvalid", bad, err)
		}
	}
}

func TestComputeEffectDigest_DeterministicRegardlessOfMapOrder(t *testing.T) {
	// JSON marshals map keys in sorted order, so two logically-equal effects
	// built with differently-ordered map literals produce the same digest.
	a := []Effect{{Action: "update", EntityType: "deal", After: map[string]any{"b": 2, "a": 1, "c": 3}}}
	b := []Effect{{Action: "update", EntityType: "deal", After: map[string]any{"c": 3, "a": 1, "b": 2}}}
	if ComputeEffectDigest("op", a) != ComputeEffectDigest("op", b) {
		t.Fatal("digest must be independent of map literal ordering")
	}
}

// --- GuardedWrite orchestration: deny / dry-run / confirm / replay ---

// guardCalls tracks which closures GuardedWrite invoked, so a test can assert
// e.g. that a dry-run never reached apply().
type guardCalls struct{ replayed, planned, applied int }

func TestGuardedWrite_ReadOnlyScopeNeverPlansOrApplies(t *testing.T) {
	var calls guardCalls
	g := GuardedWrite{Operation: "advance_deal"}
	p := Principal{WorkspaceID: uuid.New(), Role: RoleMember, Scopes: []string{ScopeCRMRead}}

	_, err := g.Run(p,
		func() (int, []byte, bool, error) { calls.replayed++; return 0, nil, false, nil },
		func() (Preview, error) { calls.planned++; return Preview{}, nil },
		func() (int, []byte, error) { calls.applied++; return 200, nil, nil },
	)
	if !errors.Is(err, ErrReadOnlyScope) {
		t.Fatalf("got %v, want ErrReadOnlyScope", err)
	}
	if calls.planned != 0 || calls.applied != 0 || calls.replayed != 0 {
		t.Fatalf("read-only denial must short-circuit before any closure: %+v", calls)
	}
}

func TestGuardedWrite_DryRunMutatesNothing(t *testing.T) {
	var calls guardCalls
	g := GuardedWrite{
		Operation: "advance_deal",
		Options:   WriteOptions{DryRun: true},
	}
	p := Principal{WorkspaceID: uuid.New(), Role: RoleAdmin, Scopes: []string{ScopeCRMWrite}}
	wantPreview := Preview{Summary: "would advance deal", Effects: []Effect{{Action: "stage_change", EntityType: "deal"}}}

	res, err := g.Run(p,
		func() (int, []byte, bool, error) { calls.replayed++; return 0, nil, false, nil },
		func() (Preview, error) { calls.planned++; return wantPreview, nil },
		func() (int, []byte, error) { calls.applied++; return 200, nil, nil },
	)
	if err != nil {
		t.Fatalf("dry-run error: %v", err)
	}
	if calls.applied != 0 {
		t.Fatal("dry-run must never apply")
	}
	if res.Preview == nil || !res.Preview.DryRun {
		t.Fatal("dry-run must return a Preview with DryRun=true")
	}
	if res.Preview.Operation != "advance_deal" {
		t.Fatalf("preview operation = %q", res.Preview.Operation)
	}
	// Non-destructive op: no confirmation token issued.
	if res.Preview.ConfirmationRequired || res.Preview.ConfirmationToken != "" {
		t.Fatal("non-destructive dry-run must not require/issue confirmation")
	}
}

func TestGuardedWrite_DestructiveDryRunIssuesToken_ThenConfirmApplies(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	conf := newTestConfirmer(t)
	ws := uuid.New()
	p := Principal{WorkspaceID: ws, Role: RoleAdmin, Scopes: []string{ScopeCRMWrite}}
	effects := []Effect{{Action: "delete", EntityType: "contact", EntityID: "c1"}}
	plan := func() (Preview, error) { return Preview{Summary: "would delete contact", Effects: effects}, nil }

	// 1. Dry-run a destructive op → returns a confirmation token.
	dry := GuardedWrite{
		Operation:   "delete_contact",
		Destructive: true,
		Confirmer:   conf,
		Now:         fixedClock(now),
		Options:     WriteOptions{DryRun: true},
	}
	res, err := dry.Run(p, nil, plan, func() (int, []byte, error) { t.Fatal("dry-run applied"); return 0, nil, nil })
	if err != nil {
		t.Fatalf("destructive dry-run: %v", err)
	}
	if !res.Preview.ConfirmationRequired || res.Preview.ConfirmationToken == "" {
		t.Fatal("destructive dry-run must require + issue a confirmation token")
	}
	token := res.Preview.ConfirmationToken

	// 2. Real call WITHOUT the token → ErrConfirmationRequired.
	applied := 0
	missing := GuardedWrite{Operation: "delete_contact", Destructive: true, Confirmer: conf, Now: fixedClock(now)}
	if _, err := missing.Run(p, nil, plan, func() (int, []byte, error) { applied++; return 200, nil, nil }); !errors.Is(err, ErrConfirmationRequired) {
		t.Fatalf("missing token: got %v, want ErrConfirmationRequired", err)
	}
	if applied != 0 {
		t.Fatal("must not apply without confirmation")
	}

	// 3. Real call WITH the token → applies exactly once.
	good := GuardedWrite{
		Operation:   "delete_contact",
		Destructive: true,
		Confirmer:   conf,
		Now:         fixedClock(now),
		Options:     WriteOptions{ConfirmationToken: token},
	}
	res, err = good.Run(p, nil, plan, func() (int, []byte, error) { applied++; return 204, []byte(`{"deleted":true}`), nil })
	if err != nil {
		t.Fatalf("confirmed apply: %v", err)
	}
	if applied != 1 || res.Status != 204 {
		t.Fatalf("confirmed call must apply once with real status; applied=%d status=%d", applied, res.Status)
	}
}

func TestGuardedWrite_DestructiveWithoutConfirmerErrors(t *testing.T) {
	g := GuardedWrite{Operation: "delete_contact", Destructive: true} // Confirmer nil
	p := Principal{WorkspaceID: uuid.New(), Role: RoleAdmin, Scopes: []string{ScopeCRMWrite}}
	_, err := g.Run(p, nil,
		func() (Preview, error) { return Preview{}, nil },
		func() (int, []byte, error) { return 200, nil, nil },
	)
	if err == nil {
		t.Fatal("destructive op without a Confirmer must error (misconfiguration)")
	}
}

func TestGuardedWrite_IdempotentReplayReturnsCachedResult(t *testing.T) {
	var applied int
	g := GuardedWrite{
		Operation: "log_interaction",
		Options:   WriteOptions{IdempotencyKey: "key-123"},
	}
	p := Principal{WorkspaceID: uuid.New(), Role: RoleAdmin, Scopes: []string{ScopeCRMWrite}}

	res, err := g.Run(p,
		func() (int, []byte, bool, error) { return 201, []byte(`{"cached":true}`), true, nil },
		func() (Preview, error) { return Preview{}, nil },
		func() (int, []byte, error) { applied++; return 201, nil, nil },
	)
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if !res.Replayed || res.Status != 201 || string(res.Body) != `{"cached":true}` {
		t.Fatalf("replay must return cached result, got %+v", res)
	}
	if applied != 0 {
		t.Fatal("idempotent replay must not re-apply the mutation")
	}
}

func TestGuardedWrite_IdempotencyMissFallsThroughToApply(t *testing.T) {
	var applied int
	g := GuardedWrite{
		Operation: "log_interaction",
		Options:   WriteOptions{IdempotencyKey: "key-new"},
	}
	p := Principal{WorkspaceID: uuid.New(), Role: RoleAdmin, Scopes: []string{ScopeCRMWrite}}

	res, err := g.Run(p,
		func() (int, []byte, bool, error) { return 0, nil, false, nil }, // cache miss
		func() (Preview, error) { return Preview{}, nil },
		func() (int, []byte, error) { applied++; return 201, []byte(`{"created":true}`), nil },
	)
	if err != nil {
		t.Fatalf("apply after miss: %v", err)
	}
	if res.Replayed {
		t.Fatal("cache miss must not be marked Replayed")
	}
	if applied != 1 || res.Status != 201 {
		t.Fatalf("cache miss must apply once, got applied=%d status=%d", applied, res.Status)
	}
}
