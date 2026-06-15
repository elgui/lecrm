package gmailreply

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/gbconsult/lecrm/apps/api/internal/sequences"
)

// --- fake pgx.Tx ----------------------------------------------------------
//
// fakeTx is a pgx.Tx double driven by SQL-substring matching. It embeds the
// pgx.Tx interface (nil) so any method the code calls that the test did not
// stub nil-derefs — surfacing an unexpected DB touch as a failure.

type recordedExec struct {
	sql  string
	args []any
}

type connRow struct {
	id       uuid.UUID
	settings []byte
	cursor   []byte
}

type fakeTx struct {
	pgx.Tx

	// loadCursor (QueryRow on sync_connections)
	cursorRaw   []byte
	cursorErr   error
	cursorNoRow bool

	// matchSteps (Query on enrollment_steps)
	matched  []MatchedStep
	matchErr error

	// listActiveGmailConnections (Query on sync_connections … active)
	conns   []connRow
	connErr error

	// saveCursor (Exec UPDATE sync_connections)
	execs        []recordedExec
	saveErr      error
	saveZeroRows bool

	committed  bool
	rolledBack bool
}

func (t *fakeTx) QueryRow(_ context.Context, sql string, _ ...any) pgx.Row {
	switch {
	case strings.Contains(sql, "SELECT sync_cursor"):
		if t.cursorNoRow {
			return errRow{pgx.ErrNoRows}
		}
		return cursorRow{raw: t.cursorRaw, err: t.cursorErr}
	default:
		return errRow{fmt.Errorf("fakeTx: unexpected QueryRow: %s", sql)}
	}
}

func (t *fakeTx) Query(_ context.Context, sql string, _ ...any) (pgx.Rows, error) {
	switch {
	case strings.Contains(sql, "FROM enrollment_steps"):
		if t.matchErr != nil {
			return nil, t.matchErr
		}
		return matchedRows(t.matched), nil
	case strings.Contains(sql, "FROM sync_connections"):
		if t.connErr != nil {
			return nil, t.connErr
		}
		return connRows(t.conns), nil
	default:
		return nil, fmt.Errorf("fakeTx: unexpected Query: %s", sql)
	}
}

func (t *fakeTx) Exec(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	t.execs = append(t.execs, recordedExec{sql: sql, args: args})
	switch {
	case strings.Contains(sql, "UPDATE sync_connections"):
		if t.saveErr != nil {
			return pgconn.CommandTag{}, t.saveErr
		}
		n := 1
		if t.saveZeroRows {
			n = 0
		}
		return pgconn.NewCommandTag(fmt.Sprintf("UPDATE %d", n)), nil
	default:
		return pgconn.CommandTag{}, nil
	}
}

func (t *fakeTx) Commit(context.Context) error {
	t.committed = true
	return nil
}

func (t *fakeTx) Rollback(context.Context) error {
	if t.committed {
		return pgx.ErrTxClosed
	}
	t.rolledBack = true
	return nil
}

func (t *fakeTx) execMatching(substr string) (recordedExec, bool) {
	for _, e := range t.execs {
		if strings.Contains(e.sql, substr) {
			return e, true
		}
	}
	return recordedExec{}, false
}

// --- fake rows / row ------------------------------------------------------

type cursorRow struct {
	raw []byte
	err error
}

func (r cursorRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	*(dest[0].(*[]byte)) = r.raw
	return nil
}

type errRow struct{ err error }

func (r errRow) Scan(...any) error { return r.err }

type fakeRows struct {
	pgx.Rows
	scans []func(dest []any) error
	i     int
	err   error
}

func (r *fakeRows) Next() bool { return r.i < len(r.scans) }

func (r *fakeRows) Scan(dest ...any) error {
	if r.i >= len(r.scans) {
		return errors.New("fakeRows: scan past end")
	}
	err := r.scans[r.i](dest)
	r.i++
	return err
}

func (r *fakeRows) Err() error { return r.err }

func (r *fakeRows) Close() {}

func matchedRows(ms []MatchedStep) *fakeRows {
	fr := &fakeRows{}
	for _, m := range ms {
		m := m
		fr.scans = append(fr.scans, func(dest []any) error {
			*(dest[0].(*string)) = m.RFCMessageID
			*(dest[1].(*uuid.UUID)) = m.EnrollmentID
			*(dest[2].(*int16)) = int16(m.StepIndex)
			*(dest[3].(*string)) = string(m.State)
			return nil
		})
	}
	return fr
}

func connRows(cs []connRow) *fakeRows {
	fr := &fakeRows{}
	for _, c := range cs {
		c := c
		fr.scans = append(fr.scans, func(dest []any) error {
			*(dest[0].(*uuid.UUID)) = c.id
			*(dest[1].(*[]byte)) = c.settings
			*(dest[2].(*[]byte)) = c.cursor
			return nil
		})
	}
	return fr
}

// --- stub acquirer --------------------------------------------------------

type stubAcquirer struct {
	tx         *fakeTx
	acquireErr error
	gotWS      []uuid.UUID
	releases   int
}

func (s *stubAcquirer) AcquireTx(ctx context.Context, wsID uuid.UUID) (context.Context, pgx.Tx, func(), error) {
	if s.acquireErr != nil {
		return ctx, nil, nil, s.acquireErr
	}
	s.gotWS = append(s.gotWS, wsID)
	tx := s.tx
	if tx == nil {
		tx = &fakeTx{}
		s.tx = tx
	}
	release := func() {
		s.releases++
		_ = tx.Rollback(ctx)
	}
	return ctx, tx, release, nil
}

// --- fake Gmail client ----------------------------------------------------

type fakeClient struct {
	msgs       []InboundMessage
	newHID     uint64
	sinceErr   error
	sinceStart uint64
	sinceCalls int

	watchHID   uint64
	watchExp   time.Time
	watchErr   error
	watchCalls int
}

func (c *fakeClient) MessagesSince(_ context.Context, start uint64) ([]InboundMessage, uint64, error) {
	c.sinceStart = start
	c.sinceCalls++
	if c.sinceErr != nil {
		return nil, 0, c.sinceErr
	}
	return c.msgs, c.newHID, nil
}

func (c *fakeClient) Watch(context.Context) (uint64, time.Time, error) {
	c.watchCalls++
	return c.watchHID, c.watchExp, c.watchErr
}

type fakeClientFactory struct {
	cli     HistoryClient
	err     error
	gotWS   uuid.UUID
	gotUser uuid.UUID
}

func (f *fakeClientFactory) Client(_ context.Context, ws, user uuid.UUID) (HistoryClient, error) {
	f.gotWS, f.gotUser = ws, user
	if f.err != nil {
		return nil, f.err
	}
	return f.cli, nil
}

// --- recording transitioner / classifier ---------------------------------

type recordedTransition struct {
	enrollmentID uuid.UUID
	to           sequences.State
	reason       string
	opts         []sequences.Option
}

type recordingTransitioner struct {
	calls []recordedTransition
	err   error
}

func (r *recordingTransitioner) fn(_ context.Context, _ pgx.Tx, enrollmentID uuid.UUID, to sequences.State, reason string, opts ...sequences.Option) error {
	r.calls = append(r.calls, recordedTransition{enrollmentID: enrollmentID, to: to, reason: reason, opts: opts})
	return r.err
}

type stubClassifier struct {
	cls   Classification
	err   error
	calls int
}

func (c *stubClassifier) Classify(context.Context, InboundMessage) (Classification, error) {
	c.calls++
	return c.cls, c.err
}
