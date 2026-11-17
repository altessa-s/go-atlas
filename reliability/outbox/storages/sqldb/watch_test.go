// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package sqldb_test

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/testhelpers"
	"github.com/altessa-s/go-atlas/reliability/outbox"
	"github.com/altessa-s/go-atlas/reliability/outbox/storages/sqldb"
)

// fakeListener serves one scripted subscription per Listen call: a nil entry
// fails the call, a channel is handed out as the subscription.
type fakeListener struct {
	mu       sync.Mutex
	subs     []chan struct{}
	channels []string
}

func (l *fakeListener) Listen(_ context.Context, channel string) (<-chan struct{}, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.channels = append(l.channels, channel)
	if len(l.subs) == 0 {
		return make(chan struct{}), nil // Stays open until the test ends.
	}
	sub := l.subs[0]
	l.subs = l.subs[1:]
	if sub == nil {
		return nil, errBoom
	}
	return sub, nil
}

func (l *fakeListener) calls() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]string(nil), l.channels...)
}

func TestNewNotifyValidation(t *testing.T) {
	t.Parallel()
	db, _ := testhelpers.NewFakeSQL(t, ok)
	var typedNil *fakeListener

	tests := []struct {
		name    string
		dialect sqldb.Dialect
		opts    []sqldb.Option
		wantErr error
		channel string
	}{
		{"mysql notify", sqldb.DialectMySQL, []sqldb.Option{sqldb.WithNotify()}, sqldb.ErrNotifyUnsupported, ""},
		{"mysql channel", sqldb.DialectMySQL, []sqldb.Option{sqldb.WithNotifyChannel("ch")}, sqldb.ErrNotifyUnsupported, ""},
		{"mysql listener", sqldb.DialectMySQL, []sqldb.Option{sqldb.WithListener(&fakeListener{})}, sqldb.ErrNotifyUnsupported, ""},
		{"mysql typed nil listener", sqldb.DialectMySQL, []sqldb.Option{sqldb.WithListener(typedNil)}, nil, ""},
		{"derived channel", sqldb.DialectPostgres, []sqldb.Option{sqldb.WithNotify(), sqldb.WithTableName("App.Events")}, nil, "app_events"},
		{"explicit channel", sqldb.DialectPostgres, []sqldb.Option{sqldb.WithNotifyChannel("outbox_wake")}, nil, "outbox_wake"},
		{"invalid channel", sqldb.DialectPostgres, []sqldb.Option{sqldb.WithNotifyChannel("outbox-wake")}, sqldb.ErrInvalidNotifyChannel, ""},
		{"qualified channel", sqldb.DialectPostgres, []sqldb.Option{sqldb.WithNotifyChannel("a.b")}, sqldb.ErrInvalidNotifyChannel, ""},
		{
			"derived channel too long", sqldb.DialectPostgres,
			[]sqldb.Option{sqldb.WithNotify(), sqldb.WithTableName(strings.Repeat("s", 40) + "." + strings.Repeat("t", 40))},
			sqldb.ErrInvalidNotifyChannel, "",
		},
		{
			"long table without notifications", sqldb.DialectPostgres,
			[]sqldb.Option{sqldb.WithTableName(strings.Repeat("s", 40) + "." + strings.Repeat("t", 40))}, nil, "",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			store, err := sqldb.New(db, tc.dialect, tc.opts...)
			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			if tc.channel != "" {
				require.Equal(t, tc.channel, store.NotifyChannel())
			}
		})
	}
}

func TestSaveEventsNotifies(t *testing.T) {
	t.Parallel()

	t.Run("own transaction", func(t *testing.T) {
		t.Parallel()
		db, fake := testhelpers.NewFakeSQL(t, ok)
		store, err := sqldb.New(db, sqldb.DialectPostgres, sqldb.WithNotify())
		require.NoError(t, err)

		require.NoError(t, store.SaveEvents(t.Context(), pending("e1")))

		calls := fake.Calls()
		require.Len(t, calls, 2)
		require.True(t, strings.HasPrefix(calls[0].Query, "INSERT"))
		require.Equal(t, "SELECT pg_notify($1, '')", calls[1].Query)
		require.Equal(t, []any{"events_outbox"}, calls[1].Args)
		require.True(t, calls[0].InTx && calls[1].InTx, "the signal shares the insert's transaction")
		require.Equal(t, 1, fake.Commits())
	})

	t.Run("caller transaction", func(t *testing.T) {
		t.Parallel()
		db, fake := testhelpers.NewFakeSQL(t, ok)
		store, err := sqldb.New(db, sqldb.DialectPostgres, sqldb.WithNotifyChannel("wake"))
		require.NoError(t, err)

		tx, err := db.BeginTx(t.Context(), nil)
		require.NoError(t, err)
		require.NoError(t, store.SaveEvents(sqldb.WithTx(t.Context(), tx), pending("e1")))
		require.Zero(t, fake.Commits(), "the caller commits")
		require.NoError(t, tx.Rollback())

		calls := fake.Calls()
		require.Len(t, calls, 2)
		require.Equal(t, []any{"wake"}, calls[1].Args)
		require.True(t, calls[1].InTx)
	})

	t.Run("insert failure sends nothing", func(t *testing.T) {
		t.Parallel()
		db, fake := testhelpers.NewFakeSQL(t, func(q string, _ []any) testhelpers.FakeSQLReply {
			if strings.HasPrefix(q, "INSERT") {
				return testhelpers.FakeSQLReply{Err: errBoom}
			}
			return testhelpers.FakeSQLReply{Affected: 1}
		})
		store, err := sqldb.New(db, sqldb.DialectPostgres, sqldb.WithNotify())
		require.NoError(t, err)

		require.ErrorIs(t, store.SaveEvents(t.Context(), pending("e1")), errBoom)
		require.Len(t, fake.Calls(), 1)
		require.Equal(t, 1, fake.Rollbacks())
	})

	t.Run("disabled", func(t *testing.T) {
		t.Parallel()
		db, fake := testhelpers.NewFakeSQL(t, ok)
		store, err := sqldb.New(db, sqldb.DialectPostgres)
		require.NoError(t, err)

		require.NoError(t, store.SaveEvents(t.Context(), pending("e1")))
		require.Len(t, fake.Calls(), 1)
		require.Zero(t, fake.Commits(), "a single INSERT needs no transaction")
	})
}

func TestWatchWithoutListenerIsUnsupported(t *testing.T) {
	t.Parallel()
	db, _ := testhelpers.NewFakeSQL(t, ok)
	var typedNil *fakeListener

	for _, opts := range [][]sqldb.Option{nil, {sqldb.WithListener(typedNil)}} {
		store, err := sqldb.New(db, sqldb.DialectPostgres, opts...)
		require.NoError(t, err)
		require.False(t, store.SupportsNotifications())
		require.ErrorIs(t, store.Watch(t.Context(), func() {}), outbox.ErrWatchUnsupported)
	}
}

func TestWatchDeliversAndResubscribes(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		first := make(chan struct{}, 1)
		listener := &fakeListener{subs: []chan struct{}{first, nil}}
		db, _ := testhelpers.NewFakeSQL(t, ok)
		store, err := sqldb.New(db, sqldb.DialectPostgres, sqldb.WithListener(listener))
		require.NoError(t, err)
		require.True(t, store.SupportsNotifications())

		var notified atomic.Int64
		ctx, cancel := context.WithCancel(t.Context())
		done := make(chan error, 1)
		go func() { done <- store.Watch(ctx, func() { notified.Add(1) }) }()

		first <- struct{}{}
		synctest.Wait()
		require.Equal(t, int64(1), notified.Load(), "a signal wakes the dispatcher")

		// The subscription ends, the next attempt fails, the third succeeds
		// and owes one synthetic wake-up for the gap.
		close(first)
		time.Sleep(time.Minute)
		synctest.Wait()
		require.Equal(t, int64(2), notified.Load())
		require.Equal(t, []string{"events_outbox", "events_outbox", "events_outbox"}, listener.calls())

		cancel()
		require.NoError(t, <-done)
	})
}

func TestWatchStopsOnCancelWhileRetrying(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		listener := &fakeListener{subs: []chan struct{}{nil, nil, nil}}
		db, _ := testhelpers.NewFakeSQL(t, ok)
		store, err := sqldb.New(db, sqldb.DialectPostgres, sqldb.WithListener(listener))
		require.NoError(t, err)

		ctx, cancel := context.WithCancel(t.Context())
		done := make(chan error, 1)
		go func() { done <- store.Watch(ctx, func() {}) }()

		synctest.Wait()
		cancel()
		require.NoError(t, <-done)
		require.Len(t, listener.calls(), 1, "the failed attempt backs off before retrying")
	})
}
