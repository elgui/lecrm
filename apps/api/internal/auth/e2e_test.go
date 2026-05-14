//go:build e2e

// End-to-end OIDC flow test against a live Authentik + lecrm-api.
//
// The test is build-tagged `e2e` so `go test ./...` (no tag) ignores it.
// It needs the Day-2 dev stack up (Postgres on :54320, Authentik on :9000),
// the OIDC client provisioned via scripts/authentik-provision-oidc-client.py,
// the test user provisioned via scripts/authentik-provision-test-user.py,
// and the `acme` workspace seeded in core.workspaces.
//
// Run:
//
//	set -a; source deploy/.env.dev 2>/dev/null; set +a
//	~/.local/go/bin/go -C apps/api build -o /tmp/lecrm-api ./cmd/lecrm-api
//	LECRM_API_BIN=/tmp/lecrm-api \
//	  ~/.local/go/bin/go -C apps/api test -tags e2e -count 1 -v \
//	    -run TestE2EOIDCFlow ./internal/auth
//
// The test starts lecrm-api as a subprocess on :8080 (Authentik's
// redirect_uri regex is pinned to that port), drives the JSON flow
// executor with username + password, follows the 302 chain back through
// /auth/callback, and asserts the four ADR-009 §5.2 / §7.1 properties
// listed at the top of TestE2EOIDCFlow.

package auth_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	testWorkspaceSlug = "acme"
	testUsername      = "guillaume-e2e"
	testPassword      = "e2etest-changeme"
	testEmail         = "guillaume-e2e@example.com"

	// :8080 because the Authentik OIDC redirect_uri regex (set by
	// scripts/authentik-provision-oidc-client.py) is pinned to that port:
	//   ^http://[a-z0-9-]+\.lecrm\.test:8080/auth/callback$
	apiPort = ":8080"
)

// TestE2EOIDCFlow drives /auth/login on lecrm-api through Authentik's
// default-authentication-flow, follows the redirect to /auth/callback,
// and asserts:
//
//  1. cookie lecrm_session is set with Domain=acme.lecrm.test
//     (workspace-scoped — ADR-009 §5.2 binding) and is NOT visible at
//     the parent domain lecrm.test;
//  2. core.users has exactly one row for the test user, with
//     issuer ending in `/o/lecrm/` and a non-empty subject;
//  3. core.workspace_members has exactly one row binding that user to
//     the acme workspace;
//  4. GET /auth/me with the session cookie returns 200 with both
//     user_id and workspace_id populated.
func TestE2EOIDCFlow(t *testing.T) {
	requireEnv(t,
		"LECRM_DATABASE_URL",
		"LECRM_OIDC_ISSUER",
		"LECRM_OIDC_CLIENT_ID",
		"LECRM_OIDC_CLIENT_SECRET",
		"LECRM_SESSION_SECRET",
	)
	tld := os.Getenv("LECRM_COOKIE_DOMAIN_TLD")
	if tld == "" {
		tld = "lecrm.test"
	}
	if !strings.HasSuffix(tld, ".test") {
		t.Skipf("e2e test refuses to run against non-.test TLD (got %q) to avoid touching real DNS", tld)
	}

	binPath := os.Getenv("LECRM_API_BIN")
	if binPath == "" {
		t.Skip("LECRM_API_BIN not set; pre-build the binary and re-export (see file header)")
	}

	apiCancel := startAPI(t, binPath)
	defer apiCancel()

	// HTTP client: cookie jar accepts the .test TLD; custom DialContext
	// maps <workspace>.<tld>:<port> to 127.0.0.1:<port> so the Set-Cookie
	// Domain (which excludes port) matches the request URL host.
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("cookiejar: %v", err)
	}
	dialer := &net.Dialer{Timeout: 5 * time.Second}
	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, port, splitErr := net.SplitHostPort(addr)
			if splitErr == nil && strings.HasSuffix(host, "."+tld) {
				return dialer.DialContext(ctx, network, net.JoinHostPort("127.0.0.1", port))
			}
			return dialer.DialContext(ctx, network, addr)
		},
	}
	noRedir := &http.Client{
		Jar:       jar,
		Transport: transport,
		Timeout:   10 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	follow := &http.Client{
		Jar:       jar,
		Transport: transport,
		Timeout:   10 * time.Second,
	}

	workspaceHost := testWorkspaceSlug + "." + tld
	base := "http://" + workspaceHost + apiPort
	issuerURL, err := url.Parse(os.Getenv("LECRM_OIDC_ISSUER"))
	if err != nil {
		t.Fatalf("parse issuer: %v", err)
	}
	authentikBase := issuerURL.Scheme + "://" + issuerURL.Host

	// 1. /auth/login -> 302 to Authentik authorize endpoint, with
	//    Set-Cookie lecrm_oauth_state (Domain=acme.lecrm.test).
	resp := mustDo(t, noRedir, "GET", base+"/auth/login", nil, nil)
	requireStatus(t, "/auth/login", resp, http.StatusFound)
	authzURL := resp.Header.Get("Location")
	resp.Body.Close()
	if !strings.HasPrefix(authzURL, authentikBase+"/application/o/authorize/") {
		t.Fatalf("/auth/login Location not authorize endpoint: %q", authzURL)
	}

	// 2. Hit authz: 302 to /flows/-/default/authentication/?next=…,
	//    Set-Cookie authentik_session (anonymous, server-side state init).
	resp = mustDo(t, noRedir, "GET", authzURL, nil, nil)
	requireStatus(t, "authz seed", resp, http.StatusFound)
	resp.Body.Close()

	// 3. Drive the JSON flow executor. The `?query=` param round-trips the
	//    authz query so the post-auth re-entry knows where to land.
	authzQuery := authzURL[strings.Index(authzURL, "?")+1:]
	execURL := authentikBase + "/api/v3/flows/executor/default-authentication-flow/" +
		"?query=" + url.QueryEscape("?"+authzQuery)
	jsonHdr := http.Header{
		"Accept":       []string{"application/json"},
		"Content-Type": []string{"application/json"},
	}

	resp = mustDo(t, noRedir, "GET", execURL, nil, jsonHdr)
	requireComponent(t, "identification GET", resp, "ak-stage-identification")

	resp = mustDo(t, noRedir, "POST", execURL, []byte(`{"uid_field":"`+testUsername+`"}`), jsonHdr)
	requireStatus(t, "uid_field POST", resp, http.StatusFound)
	resp.Body.Close()

	resp = mustDo(t, noRedir, "GET", execURL, nil, jsonHdr)
	requireComponent(t, "password GET", resp, "ak-stage-password")

	resp = mustDo(t, noRedir, "POST", execURL, []byte(`{"password":"`+testPassword+`"}`), jsonHdr)
	requireStatus(t, "password POST", resp, http.StatusFound)
	resp.Body.Close()

	// 4. The MFA-validate stage skips (not_configured_action=skip, no
	//    devices enrolled) and chains into UserLoginStage. Following the
	//    302 chain bottoms out at xak-flow-redirect.
	resp = mustDo(t, follow, "GET", execURL, nil, jsonHdr)
	requireStatus(t, "flow completion", resp, http.StatusOK)
	var redir struct {
		Component string `json:"component"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&redir); err != nil {
		t.Fatalf("decode flow completion: %v", err)
	}
	resp.Body.Close()
	if redir.Component != "xak-flow-redirect" {
		t.Fatalf("expected xak-flow-redirect after auth, got %q", redir.Component)
	}

	// 5. Revisit authz_url: the user is now authenticated server-side,
	//    so Authentik mints the authorization code and 302s back to
	//    redirect_uri (http://acme.lecrm.test:8080/auth/callback?code=…).
	resp = mustDo(t, noRedir, "GET", authzURL, nil, nil)
	requireStatus(t, "post-auth authz", resp, http.StatusFound)
	callbackURL := resp.Header.Get("Location")
	resp.Body.Close()
	if !strings.HasPrefix(callbackURL, base+"/auth/callback?code=") {
		t.Fatalf("post-auth authz Location not callback: %q", callbackURL)
	}

	// 6. /auth/callback exchanges code, upserts the user, ensures
	//    membership, and Set-Cookie's lecrm_session.
	resp = mustDo(t, noRedir, "GET", callbackURL, nil, nil)
	requireStatus(t, "/auth/callback", resp, http.StatusFound)
	if got := resp.Header.Get("Location"); got != "/" {
		t.Fatalf("/auth/callback Location: want '/', got %q", got)
	}
	resp.Body.Close()

	// --- Assertion 1: lecrm_session set on workspace subdomain, NOT on parent.
	workspaceURL := &url.URL{Scheme: "http", Host: workspaceHost, Path: "/"}
	parentURL := &url.URL{Scheme: "http", Host: tld, Path: "/"}
	var sessionCookie *http.Cookie
	for _, c := range jar.Cookies(workspaceURL) {
		if c.Name == "lecrm_session" {
			sessionCookie = c
		}
	}
	if sessionCookie == nil {
		t.Fatalf("assertion #1 FAIL: lecrm_session not set at %s", workspaceHost)
	}
	for _, c := range jar.Cookies(parentURL) {
		if c.Name == "lecrm_session" {
			t.Fatalf("assertion #1 FAIL: lecrm_session leaked to parent domain %s (ADR-009 §5.2)", tld)
		}
	}

	// --- Assertion 2 + 3: db state — exactly one user, one membership.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, os.Getenv("LECRM_DATABASE_URL"))
	if err != nil {
		t.Fatalf("dial db: %v", err)
	}
	defer pool.Close()

	var (
		userCount int
		dbIssuer  string
		dbSubject string
		dbUserID  string
	)
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM core.users WHERE email = $1`, testEmail,
	).Scan(&userCount); err != nil {
		t.Fatalf("count users: %v", err)
	}
	if userCount != 1 {
		t.Fatalf("assertion #2 FAIL: want exactly 1 core.users row for %s, got %d", testEmail, userCount)
	}
	if err := pool.QueryRow(ctx,
		`SELECT id::text, issuer, subject FROM core.users WHERE email = $1`, testEmail,
	).Scan(&dbUserID, &dbIssuer, &dbSubject); err != nil {
		t.Fatalf("read user: %v", err)
	}
	if !strings.HasSuffix(dbIssuer, "/o/lecrm/") {
		t.Fatalf("assertion #2 FAIL: user issuer suffix mismatch — got %q", dbIssuer)
	}
	if dbSubject == "" {
		t.Fatalf("assertion #2 FAIL: user subject is empty (ADR-009 §7.1 requires non-empty sub)")
	}

	var memberCount int
	if err := pool.QueryRow(ctx, `
		SELECT count(*)
		  FROM core.workspace_members m
		  JOIN core.workspaces w ON w.id = m.workspace_id
		 WHERE m.user_id = $1::uuid AND w.slug = $2
	`, dbUserID, testWorkspaceSlug).Scan(&memberCount); err != nil {
		t.Fatalf("count members: %v", err)
	}
	if memberCount != 1 {
		t.Fatalf("assertion #3 FAIL: want exactly 1 core.workspace_members row, got %d", memberCount)
	}

	// --- Assertion 4: /auth/me round-trip.
	resp = mustDo(t, noRedir, "GET", base+"/auth/me", nil, nil)
	requireStatus(t, "/auth/me", resp, http.StatusOK)
	var me struct {
		UserID      string `json:"user_id"`
		WorkspaceID string `json:"workspace_id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&me); err != nil {
		t.Fatalf("decode /auth/me: %v", err)
	}
	resp.Body.Close()
	if me.UserID == "" || me.WorkspaceID == "" {
		t.Fatalf("assertion #4 FAIL: /auth/me missing fields: %+v", me)
	}
	if me.UserID != dbUserID {
		t.Fatalf("assertion #4 FAIL: /auth/me user_id %q != db user_id %q", me.UserID, dbUserID)
	}

	subjectTail := dbSubject
	if len(subjectTail) > 8 {
		subjectTail = subjectTail[:8] + "..."
	}
	t.Logf("e2e PASS: user=%s issuer=%s subject=%s workspace_id=%s cookie_domain=%s",
		me.UserID, dbIssuer, subjectTail, me.WorkspaceID, workspaceHost)
}

// startAPI spawns the lecrm-api binary on apiPort and waits for /healthz.
// The subprocess inherits the env (including LECRM_* config) plus an
// override that pins LECRM_HTTP_ADDR to the e2e port.
func startAPI(t *testing.T, binPath string) func() {
	t.Helper()

	logFile, err := os.CreateTemp("", "lecrm-api-e2e-*.log")
	if err != nil {
		t.Fatalf("create log: %v", err)
	}
	cmd := exec.Command(binPath)
	cmd.Env = append(os.Environ(), "LECRM_HTTP_ADDR="+apiPort)
	cmd.Stdout = logFile
	cmd.Stderr = logFile

	if err := cmd.Start(); err != nil {
		logFile.Close()
		t.Fatalf("start lecrm-api: %v", err)
	}

	cancel := func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}
		_ = logFile.Close()
	}

	healthURL := "http://127.0.0.1" + apiPort + "/healthz"
	deadline := time.Now().Add(15 * time.Second)
	var lastErr error
	for time.Now().Before(deadline) {
		time.Sleep(150 * time.Millisecond)
		resp, err := http.Get(healthURL)
		if err != nil {
			lastErr = err
			continue
		}
		_ = resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			t.Logf("lecrm-api ready at %s (log: %s)", apiPort, logFile.Name())
			return cancel
		}
		lastErr = fmt.Errorf("status %d", resp.StatusCode)
	}
	cancel()
	out, _ := os.ReadFile(logFile.Name())
	t.Fatalf("lecrm-api never became healthy on %s: %v\nlog:\n%s", apiPort, lastErr, out)
	return func() {}
}

func mustDo(t *testing.T, c *http.Client, method, urlStr string, body []byte, headers http.Header) *http.Response {
	t.Helper()
	var rdr io.Reader
	if body != nil {
		rdr = strings.NewReader(string(body))
	}
	req, err := http.NewRequest(method, urlStr, rdr)
	if err != nil {
		t.Fatalf("new request %s %s: %v", method, urlStr, err)
	}
	for k, vs := range headers {
		req.Header[k] = vs
	}
	resp, err := c.Do(req)
	if err != nil {
		t.Fatalf("do %s %s: %v", method, urlStr, err)
	}
	return resp
}

func requireStatus(t *testing.T, label string, resp *http.Response, want int) {
	t.Helper()
	if resp.StatusCode != want {
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		t.Fatalf("%s: want status %d, got %d. body=%q", label, want, resp.StatusCode, body)
	}
}

func requireComponent(t *testing.T, label string, resp *http.Response, want string) {
	t.Helper()
	requireStatus(t, label, resp, http.StatusOK)
	body, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if err != nil {
		t.Fatalf("%s: read body: %v", label, err)
	}
	var j struct {
		Component string `json:"component"`
	}
	if err := json.Unmarshal(body, &j); err != nil {
		t.Fatalf("%s: decode json: %v\nbody=%s", label, err, body)
	}
	if j.Component != want {
		t.Fatalf("%s: want component %q, got %q\nbody=%s", label, want, j.Component, body)
	}
}

func requireEnv(t *testing.T, keys ...string) {
	t.Helper()
	var missing []string
	for _, k := range keys {
		if os.Getenv(k) == "" {
			missing = append(missing, k)
		}
	}
	if len(missing) > 0 {
		t.Skipf("missing env vars: %s (source deploy/.env.dev first)", strings.Join(missing, ", "))
	}
}

