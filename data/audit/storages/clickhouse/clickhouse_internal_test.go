// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package clickhouse

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/audit"
)

var errBackend = errors.New("backend failure")

// mockConn implements [Conn]. Unset hooks succeed and record nothing.
type mockConn struct {
	execFn         func(ctx context.Context, query string, args ...any) error
	prepareBatchFn func(ctx context.Context, query string, opts ...driver.PrepareBatchOption) (driver.Batch, error)
	queryFn        func(ctx context.Context, query string, args ...any) (driver.Rows, error)
	queryRowFn     func(ctx context.Context, query string, args ...any) driver.Row

	execQueries  []string
	prepareCalls int
	lastQuery    string
	lastArgs     []any
	lastCtx      context.Context //nolint:containedctx // recorded for assertions, never used to call
}

func (m *mockConn) Exec(ctx context.Context, query string, args ...any) error {
	m.execQueries = append(m.execQueries, query)
	if m.execFn != nil {
		return m.execFn(ctx, query, args...)
	}

	return nil
}

func (m *mockConn) PrepareBatch(ctx context.Context, query string, opts ...driver.PrepareBatchOption) (driver.Batch, error) {
	m.prepareCalls++
	m.lastQuery = query
	m.lastCtx = ctx
	if m.prepareBatchFn != nil {
		return m.prepareBatchFn(ctx, query, opts...)
	}

	return &mockBatch{}, nil
}

func (m *mockConn) Query(ctx context.Context, query string, args ...any) (driver.Rows, error) {
	m.lastQuery, m.lastArgs, m.lastCtx = query, args, ctx
	if m.queryFn != nil {
		return m.queryFn(ctx, query, args...)
	}

	return &mockRows{}, nil
}

func (m *mockConn) QueryRow(ctx context.Context, query string, args ...any) driver.Row {
	m.lastQuery, m.lastArgs, m.lastCtx = query, args, ctx
	if m.queryRowFn != nil {
		return m.queryRowFn(ctx, query, args...)
	}

	return &mockRow{}
}

// mockBatch records appended rows. The embedded interface covers the methods
// the storage never calls; reaching one of them panics, which is the point.
type mockBatch struct {
	driver.Batch

	// columns holds one entry per column, in schemaColumns order, each the
	// whole slice the storage appended for it.
	columns   []any
	sends     int
	closes    int
	appendErr error
	sendErr   error
}

// Column returns a handle that records what the storage appends for the
// column at index i.
func (b *mockBatch) Column(i int) driver.BatchColumn {
	return &mockBatchColumn{batch: b, index: i}
}

// rowCount reports how many rows the recorded columns carry.
func (b *mockBatch) rowCount() int {
	if len(b.columns) == 0 {
		return 0
	}

	timestamps, ok := b.columns[0].([]time.Time)
	if !ok {
		return 0
	}

	return len(timestamps)
}

type mockBatchColumn struct {
	batch *mockBatch
	index int
}

func (c *mockBatchColumn) Append(v any) error {
	if c.batch.appendErr != nil {
		return c.batch.appendErr
	}

	for len(c.batch.columns) <= c.index {
		c.batch.columns = append(c.batch.columns, nil)
	}
	c.batch.columns[c.index] = v

	return nil
}

func (c *mockBatchColumn) AppendRow(any) error { return nil }

func (b *mockBatch) Send() error {
	b.sends++

	return b.sendErr
}

func (b *mockBatch) Close() error {
	b.closes++

	return nil
}

// mockRows replays prepared rows through the same positional contract the
// driver uses.
type mockRows struct {
	driver.Rows

	rows    []*eventRow
	pos     int
	closes  int
	scanErr error
	err     error
}

func (r *mockRows) Next() bool {
	if r.pos >= len(r.rows) {
		return false
	}
	r.pos++

	return true
}

func (r *mockRows) Scan(dest ...any) error {
	if r.scanErr != nil {
		return r.scanErr
	}

	src := r.rows[r.pos-1].args()
	for i := range dest {
		reflect.ValueOf(dest[i]).Elem().Set(reflect.ValueOf(src[i]))
	}

	return nil
}

func (r *mockRows) Close() error {
	r.closes++

	return nil
}

func (r *mockRows) Err() error {
	return r.err
}

type mockRow struct {
	driver.Row

	count int64
	err   error
}

func (r *mockRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}

	ptr, ok := dest[0].(*int64)
	if !ok {
		return errors.New("unexpected scan destination")
	}
	*ptr = r.count

	return nil
}

func TestNewRequiresConn(t *testing.T) {
	t.Parallel()

	s, err := New(nil)

	require.Nil(t, s)
	require.ErrorIs(t, err, ErrNoConn)
}

func TestNewSkipsTableCreationByDefault(t *testing.T) {
	t.Parallel()

	conn := &mockConn{}

	_, err := New(conn)

	require.NoError(t, err)
	require.Empty(t, conn.execQueries)
}

func TestNewCreatesTableOnDemand(t *testing.T) {
	t.Parallel()

	conn := &mockConn{}

	_, err := New(conn, WithAutoCreateTable(), WithTableName("events"), WithTTL(time.Hour))

	require.NoError(t, err)
	require.Equal(t, []string{mustDDL(t, "events", DefaultEngine, "", time.Hour)}, conn.execQueries)
}

func TestNewBoundsTableCreation(t *testing.T) {
	t.Parallel()

	var remaining time.Duration
	conn := &mockConn{execFn: func(ctx context.Context, _ string, _ ...any) error {
		deadline, ok := ctx.Deadline()
		require.True(t, ok, "the CREATE TABLE runs under a deadline")
		remaining = time.Until(deadline)
		return nil
	}}

	_, err := New(conn, WithAutoCreateTable(), WithDDLTimeout(time.Minute))

	require.NoError(t, err)
	require.InDelta(t, time.Minute, remaining, float64(5*time.Second))
}

func TestNewPropagatesTableCreationFailure(t *testing.T) {
	t.Parallel()

	conn := &mockConn{execFn: func(context.Context, string, ...any) error { return errBackend }}

	s, err := New(conn, WithAutoCreateTable())

	require.Nil(t, s)
	require.ErrorIs(t, err, errBackend)
}

func TestStoreAppendsSingleRow(t *testing.T) {
	t.Parallel()

	batch := &mockBatch{}
	conn := &mockConn{
		prepareBatchFn: func(context.Context, string, ...driver.PrepareBatchOption) (driver.Batch, error) {
			return batch, nil
		},
	}

	s, err := New(conn)
	require.NoError(t, err)

	ctx := t.Context()
	require.NoError(t, s.Store(ctx, fullEvent()))

	require.Equal(t, insertStatement(DefaultTableName), conn.lastQuery)
	require.Equal(t, 1, batch.rowCount())
	require.Equal(t, 1, batch.sends)
	require.Equal(t, 1, batch.closes)

	// The recorded columns must match what a buffer built from the same
	// event holds, which also pins the column order the storage appends in.
	row, err := toRow(fullEvent())
	require.NoError(t, err)

	want := newColumnBuffer(1)
	want.appendRow(row)
	require.Equal(t, want.values(), batch.columns)

	// The single-row path must decorate the context with the async-insert
	// settings; the driver keeps them in an unexported value, so the only
	// observable effect is that the context is no longer the caller's.
	require.NotEqual(t, ctx, conn.lastCtx)
}

func TestStoreBatchSplitsIntoChunks(t *testing.T) {
	t.Parallel()

	var batches []*mockBatch
	conn := &mockConn{
		prepareBatchFn: func(context.Context, string, ...driver.PrepareBatchOption) (driver.Batch, error) {
			b := &mockBatch{}
			batches = append(batches, b)

			return b, nil
		},
	}

	s, err := New(conn, WithMaxBatchSize(2))
	require.NoError(t, err)

	events := make([]*audit.Event, 5)
	for i := range events {
		events[i] = fullEvent()
	}

	require.NoError(t, s.StoreBatch(t.Context(), events))

	require.Len(t, batches, 3)
	require.Equal(t, 2, batches[0].rowCount())
	require.Equal(t, 2, batches[1].rowCount())
	require.Equal(t, 1, batches[2].rowCount())

	for i, b := range batches {
		require.Equal(t, 1, b.sends, "batch %d", i)
		require.Equal(t, 1, b.closes, "batch %d", i)
	}
}

func TestStoreBatchEmptyIsNoOp(t *testing.T) {
	t.Parallel()

	conn := &mockConn{}

	s, err := New(conn)
	require.NoError(t, err)

	require.NoError(t, s.StoreBatch(t.Context(), nil))
	require.Zero(t, conn.prepareCalls)
}

func TestStoreBatchFailures(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		batch  *mockBatch
		events []*audit.Event
		want   error
	}{
		{
			name:   "append fails",
			batch:  &mockBatch{appendErr: errBackend},
			events: []*audit.Event{fullEvent()},
			want:   errBackend,
		},
		{
			name:   "send fails",
			batch:  &mockBatch{sendErr: errBackend},
			events: []*audit.Event{fullEvent()},
			want:   errBackend,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			conn := &mockConn{
				prepareBatchFn: func(context.Context, string, ...driver.PrepareBatchOption) (driver.Batch, error) {
					return tt.batch, nil
				},
			}

			s, err := New(conn)
			require.NoError(t, err)

			require.ErrorIs(t, s.StoreBatch(t.Context(), tt.events), tt.want)
			require.Equal(t, 1, tt.batch.closes)
		})
	}
}

func TestStoreBatchRejectsUnencodableEvent(t *testing.T) {
	t.Parallel()

	batch := &mockBatch{}
	conn := &mockConn{
		prepareBatchFn: func(context.Context, string, ...driver.PrepareBatchOption) (driver.Batch, error) {
			return batch, nil
		},
	}

	s, err := New(conn)
	require.NoError(t, err)

	bad := &audit.Event{Metadata: map[string]any{"ch": make(chan int)}}

	require.Error(t, s.StoreBatch(t.Context(), []*audit.Event{bad}))
	require.Empty(t, batch.columns)
	require.Zero(t, batch.sends)
}

func TestStoreBatchPropagatesPrepareFailure(t *testing.T) {
	t.Parallel()

	conn := &mockConn{
		prepareBatchFn: func(context.Context, string, ...driver.PrepareBatchOption) (driver.Batch, error) {
			return nil, errBackend
		},
	}

	s, err := New(conn)
	require.NoError(t, err)

	require.ErrorIs(t, s.StoreBatch(t.Context(), []*audit.Event{fullEvent()}), errBackend)
}

func TestQueryIteratesEvents(t *testing.T) {
	t.Parallel()

	row, err := toRow(fullEvent())
	require.NoError(t, err)

	rows := &mockRows{rows: []*eventRow{row, row}}
	conn := &mockConn{
		queryFn: func(context.Context, string, ...any) (driver.Rows, error) { return rows, nil },
	}

	s, err := New(conn, WithTimeRangeMode(TimeRangeModeDisabled))
	require.NoError(t, err)

	var got []*audit.Event
	for event, err := range s.Query(t.Context(), &audit.Query{ActorID: "user-42"}) {
		require.NoError(t, err)
		got = append(got, event)
	}

	require.Len(t, got, 2)
	for _, event := range got {
		RequireEventEqual(t, fullEvent(), event)
	}
	require.Equal(t, 1, rows.closes)
	require.Equal(t, []any{"user-42"}, conn.lastArgs)
}

func TestQueryStopsWhenConsumerBreaks(t *testing.T) {
	t.Parallel()

	row, err := toRow(fullEvent())
	require.NoError(t, err)

	rows := &mockRows{rows: []*eventRow{row, row, row}}
	conn := &mockConn{
		queryFn: func(context.Context, string, ...any) (driver.Rows, error) { return rows, nil },
	}

	s, err := New(conn, WithTimeRangeMode(TimeRangeModeDisabled))
	require.NoError(t, err)

	seen := 0
	for range s.Query(t.Context(), nil) {
		seen++

		break
	}

	require.Equal(t, 1, seen)
	require.Equal(t, 1, rows.pos)
	require.Equal(t, 1, rows.closes)
}

func TestQueryYieldsFailures(t *testing.T) {
	t.Parallel()

	row, err := toRow(fullEvent())
	require.NoError(t, err)

	tests := []struct {
		name string
		conn *mockConn
	}{
		{
			name: "query fails",
			conn: &mockConn{
				queryFn: func(context.Context, string, ...any) (driver.Rows, error) { return nil, errBackend },
			},
		},
		{
			name: "scan fails",
			conn: &mockConn{
				queryFn: func(context.Context, string, ...any) (driver.Rows, error) {
					return &mockRows{rows: []*eventRow{row}, scanErr: errBackend}, nil
				},
			},
		},
		{
			name: "iteration fails",
			conn: &mockConn{
				queryFn: func(context.Context, string, ...any) (driver.Rows, error) {
					return &mockRows{err: errBackend}, nil
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			s, err := New(tt.conn, WithTimeRangeMode(TimeRangeModeDisabled))
			require.NoError(t, err)

			var errs []error
			for event, err := range s.Query(t.Context(), nil) {
				require.Nil(t, event)
				errs = append(errs, err)
			}

			require.Len(t, errs, 1)
			require.ErrorIs(t, errs[0], errBackend)
		})
	}
}

// A malformed JSON column fails the query with a conversion error.
func TestQueryFailsOnMalformedColumn(t *testing.T) {
	t.Parallel()

	conn := &mockConn{
		queryFn: func(context.Context, string, ...any) (driver.Rows, error) {
			return &mockRows{rows: []*eventRow{{Metadata: "{"}}}, nil
		},
	}

	s, err := New(conn, WithTimeRangeMode(TimeRangeModeDisabled))
	require.NoError(t, err)

	var errs []error
	for _, err := range s.Query(t.Context(), &audit.Query{}) {
		errs = append(errs, err)
	}

	require.Len(t, errs, 1)
	var syntaxErr *json.SyntaxError
	require.ErrorAs(t, errs[0], &syntaxErr)
}

func TestCount(t *testing.T) {
	t.Parallel()

	conn := &mockConn{
		queryRowFn: func(context.Context, string, ...any) driver.Row { return &mockRow{count: 17} },
	}

	s, err := New(conn, WithTimeRangeMode(TimeRangeModeDisabled))
	require.NoError(t, err)

	got, err := s.Count(t.Context(), &audit.Query{ActorID: "user-42"})

	require.NoError(t, err)
	require.Equal(t, int64(17), got)
	require.Equal(t, "SELECT toInt64(count()) FROM `audit_events` WHERE `actor_id` = ?", conn.lastQuery)
}

func TestCountPropagatesFailure(t *testing.T) {
	t.Parallel()

	conn := &mockConn{
		queryRowFn: func(context.Context, string, ...any) driver.Row { return &mockRow{err: errBackend} },
	}

	s, err := New(conn, WithTimeRangeMode(TimeRangeModeDisabled))
	require.NoError(t, err)

	got, err := s.Count(t.Context(), nil)

	require.Zero(t, got)
	require.ErrorIs(t, err, errBackend)
}

func TestTimeRangeModeEnforceRejectsUnboundedReads(t *testing.T) {
	t.Parallel()

	conn := &mockConn{}

	s, err := New(conn, WithTimeRangeMode(TimeRangeModeEnforce))
	require.NoError(t, err)

	var errs []error
	for _, err := range s.Query(t.Context(), &audit.Query{ActorID: "user-42"}) {
		errs = append(errs, err)
	}
	require.Len(t, errs, 1)
	require.ErrorIs(t, errs[0], ErrTimeRangeRequired)

	_, err = s.Count(t.Context(), nil)
	require.ErrorIs(t, err, ErrTimeRangeRequired)

	require.Empty(t, conn.lastQuery, "no statement may reach the server")
}

func TestTimeRangeModeEnforceAllowsBoundedReads(t *testing.T) {
	t.Parallel()

	start := time.Date(2026, time.March, 1, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name  string
		query *audit.Query
	}{
		{name: "start bound", query: &audit.Query{StartTime: &start}},
		{name: "end bound", query: &audit.Query{EndTime: &start}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			conn := &mockConn{
				queryRowFn: func(context.Context, string, ...any) driver.Row { return &mockRow{count: 3} },
			}

			s, err := New(conn, WithTimeRangeMode(TimeRangeModeEnforce))
			require.NoError(t, err)

			got, err := s.Count(t.Context(), tt.query)

			require.NoError(t, err)
			require.Equal(t, int64(3), got)
		})
	}
}

func TestWithTimeRangeModeIgnoresUnknownMode(t *testing.T) {
	t.Parallel()

	o := newOptions(WithTimeRangeMode(TimeRangeModeEnforce), WithTimeRangeMode("bogus"))

	require.Equal(t, TimeRangeModeEnforce, o.timeRangeMode)
}

func TestCloseIsNoOp(t *testing.T) {
	t.Parallel()

	s, err := New(&mockConn{})
	require.NoError(t, err)

	require.NoError(t, s.Close(t.Context()))
}
