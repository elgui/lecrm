package jobs_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/gbconsult/lecrm/apps/api/internal/jobs"
)

// stubResolver returns a fixed role name or a fixed error.
type stubResolver struct {
	roleName string
	err      error
}

func (s *stubResolver) WorkspaceRoleName(_ context.Context, _ uuid.UUID) (string, error) {
	return s.roleName, s.err
}

// stubCredResolver returns a fixed DSN or a fixed error.
type stubCredResolver struct {
	dsn string
	err error
}

func (s *stubCredResolver) DSNForRole(_ context.Context, _ string) (string, error) {
	return s.dsn, s.err
}

// neverCalledFn is a job func that panics if called; used to verify
// that RunWorkspaceJob short-circuits before reaching fn on early errors.
func neverCalledFn(_ context.Context, _ *pgx.Conn) (int, error) {
	panic("fn must not be called when resolver/creds fail")
}

func TestRunWorkspaceJob_ResolverError_Propagated(t *testing.T) {
	resolveErr := errors.New("no such workspace")

	_, err := jobs.RunWorkspaceJob[int](
		context.Background(),
		&stubResolver{err: resolveErr},
		&stubCredResolver{dsn: "postgres://localhost/nodb"},
		uuid.New(),
		neverCalledFn,
	)
	if err == nil {
		t.Fatal("expected error from resolver, got nil")
	}
	if !errors.Is(err, resolveErr) {
		t.Errorf("error chain does not contain resolveErr; got: %v", err)
	}
}

func TestRunWorkspaceJob_CredentialError_Propagated(t *testing.T) {
	credErr := errors.New("no credentials for role")

	_, err := jobs.RunWorkspaceJob[int](
		context.Background(),
		&stubResolver{roleName: "workspace_abc123"},
		&stubCredResolver{err: credErr},
		uuid.New(),
		neverCalledFn,
	)
	if err == nil {
		t.Fatal("expected error from cred resolver, got nil")
	}
	if !errors.Is(err, credErr) {
		t.Errorf("error chain does not contain credErr; got: %v", err)
	}
}

func TestProbeWorkspaceConnectivity_ResolverError(t *testing.T) {
	resolveErr := errors.New("workspace lookup failed")

	err := jobs.ProbeWorkspaceConnectivity(
		context.Background(),
		&stubResolver{err: resolveErr},
		&stubCredResolver{},
		uuid.New(),
	)
	if !errors.Is(err, resolveErr) {
		t.Errorf("ProbeWorkspaceConnectivity did not propagate resolver error; got %v", err)
	}
}
