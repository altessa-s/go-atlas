// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package sqldb

import (
	"context"
	"database/sql/driver"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/coord/dlock/providers"
	"github.com/altessa-s/go-atlas/coord/dlock/providers/providertest"
	"github.com/altessa-s/go-atlas/internal/testhelpers"
)

// leaseRow is one lock row of a leaseDB.
type leaseRow struct {
	owner                                      string
	fencing, acquired, renewed, expires, ttlUs int64
}

// leaseDB emulates the PostgreSQL locks table behind testhelpers.FakeSQL: it
// interprets the five statements the locker issues against an in-memory map
// under one mutex, as the database's row lock would serialize them, with the
// wall clock as the server clock.
type leaseDB struct {
	mu   sync.Mutex
	rows map[string]*leaseRow
}

func newLeaseDB() *leaseDB { return &leaseDB{rows: map[string]*leaseRow{}} }

func nowMicro() int64 { return time.Now().UnixMicro() }

var fencingCols = []string{"fencing"}

func (f *leaseDB) respond(query string, args []any) testhelpers.FakeSQLReply {
	f.mu.Lock()
	defer f.mu.Unlock()
	now := nowMicro()
	str := func(i int) string { return args[i].(string) }
	switch {
	case strings.Contains(query, "ON CONFLICT"): // acquire: key, owner, ttl, ttl
		r, ok := f.rows[str(0)]
		if ok && r.expires > now {
			return testhelpers.FakeSQLReply{Columns: fencingCols}
		}
		if !ok {
			r = &leaseRow{}
			f.rows[str(0)] = r
		}
		ttl := args[2].(int64)
		*r = leaseRow{owner: str(1), fencing: r.fencing + 1, acquired: now, renewed: now, expires: now + ttl, ttlUs: ttl}
		return testhelpers.FakeSQLReply{Columns: fencingCols, Rows: [][]driver.Value{{r.fencing}}}
	case strings.HasPrefix(query, "SELECT"): // read: key[, owner, fencing]
		cols := []string{"lock_key", "owner", "fencing", "acquired_at", "renewed_at", "ttl_us"}
		r, ok := f.rows[str(0)]
		if !ok || r.expires <= now || len(args) == 3 && (r.owner != str(1) || r.fencing != args[2].(int64)) {
			return testhelpers.FakeSQLReply{Columns: cols}
		}
		return testhelpers.FakeSQLReply{Columns: cols, Rows: [][]driver.Value{{str(0), r.owner, r.fencing, r.acquired, r.renewed, r.ttlUs}}}
	case strings.Contains(query, "SET renewed_at"): // renew: ttl, key, owner, fencing
		r, ok := f.rows[str(1)]
		if !ok || r.expires <= now || r.owner != str(2) || r.fencing != args[3].(int64) {
			return testhelpers.FakeSQLReply{}
		}
		r.renewed, r.expires = now, now+args[0].(int64)
		return testhelpers.FakeSQLReply{Affected: 1}
	case strings.Contains(query, "SET owner = ''"): // release: key, owner[, fencing]
		r, ok := f.rows[str(0)]
		if !ok || r.owner != str(1) || len(args) == 3 && r.fencing != args[2].(int64) {
			return testhelpers.FakeSQLReply{}
		}
		r.owner, r.expires = "", now
		return testhelpers.FakeSQLReply{Affected: 1}
	case strings.HasPrefix(query, "DELETE"): // delete expired: grace
		var n int64
		for k, r := range f.rows {
			if r.expires <= now-args[0].(int64) {
				delete(f.rows, k)
				n++
			}
		}
		return testhelpers.FakeSQLReply{Affected: n}
	}
	return testhelpers.FakeSQLReply{Err: errors.New("leaseDB: unexpected statement: " + query)}
}

// expire ends the stored lease of key at once.
func (f *leaseDB) expire(key string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if r, ok := f.rows[key]; ok {
		r.expires = nowMicro() - 1
	}
}

// newFakeLocker returns a PostgreSQL locker over f, closed on cleanup.
func newFakeLocker(tb testing.TB, f *leaseDB, opts ...Option) *Locker {
	tb.Helper()
	db, _ := testhelpers.NewFakeSQL(tb, f.respond)
	l, err := New(db, DialectPostgres, opts...)
	require.NoError(tb, err)
	tb.Cleanup(func() { _ = l.Close(context.Background()) })
	return l
}

// TestProviderContract runs the provider contract suite over the emulated
// table; tests/integration/dlockit runs it on live servers.
func TestProviderContract(t *testing.T) {
	t.Parallel()
	providertest.Run(t, func(tb testing.TB) providertest.Backend {
		f := newLeaseDB()
		return providertest.Backend{
			NewProvider: func(tb testing.TB) providers.Provider { return newFakeLocker(tb, f, WithTTL(time.Second)) },
			Expire:      func(_ testing.TB, key string) { f.expire(key) },
		}
	})
}

func TestNew_TruncatesTTLToMilliseconds(t *testing.T) {
	t.Parallel()
	db, _ := testhelpers.NewFakeSQL(t, nil)
	l, err := New(db, DialectPostgres, WithTTL(2900*time.Microsecond), WithRenewRatio(0.9))
	require.NoError(t, err)
	require.Equal(t, 2*time.Millisecond, l.s.ttl)
	require.Less(t, l.engine.Interval(), l.s.ttl)
}

// has reports whether the emulated table holds a row for key.
func (f *leaseDB) has(key string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	_, ok := f.rows[key]
	return ok
}

// A hard lease is never renewed: the locker issues no renewal, reports the
// lease lost one TTL after the acquisition, and another locker then takes the
// key with the next fencing token.
func TestHardLeaseIsNotRenewed(t *testing.T) {
	t.Parallel()
	f := newLeaseDB()
	db, fake := testhelpers.NewFakeSQL(t, f.respond)
	const ttl = 100 * time.Millisecond
	a, err := New(db, DialectPostgres, WithTTL(ttl), WithoutRenewal())
	require.NoError(t, err)
	t.Cleanup(func() { _ = a.Close(context.Background()) })
	b := newFakeLocker(t, f, WithTTL(ttl), WithoutRenewal())

	lk, err := a.Lock(t.Context(), "k")
	require.NoError(t, err)
	_, err = b.Lock(t.Context(), "k")
	require.ErrorIs(t, err, providers.ErrLockNotHeld)

	notifier, ok := lk.(providers.LostNotifier)
	require.True(t, ok)
	select {
	case <-notifier.Lost():
	case <-time.After(2 * time.Second):
		t.Fatal("the hard lease must end one TTL after its acquisition")
	}
	require.Eventually(t, func() bool {
		next, err := b.Lock(t.Context(), "k")
		if err != nil {
			return false
		}
		info, err := next.GetLockInfo(t.Context())
		return err == nil && info.FencingToken == 2 && info.AcquiredAt.Equal(info.LastRenewed)
	}, 2*time.Second, 5*time.Millisecond)
	for _, c := range fake.Calls() {
		require.NotContains(t, c.Query, "SET renewed_at", "a hard lease is never renewed")
	}
	require.NoError(t, lk.Release(t.Context()), "releasing an ended hard lease is a no-op")
}

// The background sweep starts with the first Lock, deletes rows whose lease
// ended more than the grace ago (so the key restarts at fencing token 1), and
// stops at Close.
func TestCleanupSweepLifecycle(t *testing.T) {
	t.Parallel()
	f := newLeaseDB()
	db, fake := testhelpers.NewFakeSQL(t, f.respond)
	l, err := New(db, DialectPostgres, WithExpiredCleanup(0))
	require.NoError(t, err)
	l.sweepEvery = 5 * time.Millisecond

	time.Sleep(20 * time.Millisecond)
	require.Empty(t, fake.Calls(), "nothing runs before the first Lock")

	lk, err := l.Lock(t.Context(), "k")
	require.NoError(t, err)
	info, err := lk.GetLockInfo(t.Context())
	require.NoError(t, err)
	require.EqualValues(t, 1, info.FencingToken)
	time.Sleep(20 * time.Millisecond)
	require.True(t, f.has("k"), "a held lease is never deleted")
	require.NoError(t, lk.Release(t.Context()))
	require.Eventually(t, func() bool { return !f.has("k") }, 2*time.Second, 5*time.Millisecond,
		"the released row is deleted once the grace passed")

	lk, err = l.Lock(t.Context(), "k")
	require.NoError(t, err)
	info, err = lk.GetLockInfo(t.Context())
	require.NoError(t, err)
	require.EqualValues(t, 1, info.FencingToken, "fencing restarts for a deleted key")

	require.NoError(t, l.Close(t.Context()))
	select {
	case <-l.sweepDone:
	default:
		t.Fatal("Close must stop the sweep")
	}
	require.NoError(t, l.Close(t.Context()), "a second Close is harmless")
	_, err = l.Lock(t.Context(), "k")
	require.Error(t, err)
}

// blockedSweep returns a locker over f whose sweep, once running, blocks in
// its DELETE until the test ends, and the channel closed when it got there.
func blockedSweep(t *testing.T, f *leaseDB) (*Locker, <-chan struct{}) {
	t.Helper()
	entered, unblock := make(chan struct{}), make(chan struct{})
	var once sync.Once
	t.Cleanup(func() { close(unblock) })
	db, _ := testhelpers.NewFakeSQL(t, func(query string, args []any) testhelpers.FakeSQLReply {
		if strings.HasPrefix(query, "DELETE") {
			once.Do(func() { close(entered) })
			<-unblock
		}
		return f.respond(query, args)
	})
	l, err := New(db, DialectPostgres, WithExpiredCleanup(0))
	require.NoError(t, err)
	l.sweepEvery = time.Millisecond
	return l, entered
}

// A sweep that does not return in time must not keep Close from closing the
// engine and releasing held locks; the failed wait is reported.
func TestClose_BlockedSweepStillReleases(t *testing.T) {
	t.Parallel()
	f := newLeaseDB()
	l, entered := blockedSweep(t, f)
	lk, err := l.Lock(t.Context(), "k")
	require.NoError(t, err)
	<-entered

	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()
	err = l.Close(ctx)
	require.ErrorIs(t, err, context.DeadlineExceeded, "the sweep did not stop in time")
	_, err = lk.GetLockInfo(t.Context())
	require.ErrorIs(t, err, providers.ErrLockNotHeld, "the held lock was released")
	_, err = l.Lock(t.Context(), "k")
	require.Error(t, err, "the engine is closed")
}

// With a context that already ended, Close still closes the engine (releases
// need a live context and fail), so the locker rejects new locks.
func TestClose_ExpiredContextStillClosesEngine(t *testing.T) {
	t.Parallel()
	f := newLeaseDB()
	l, entered := blockedSweep(t, f)
	_, err := l.Lock(t.Context(), "k")
	require.NoError(t, err)
	<-entered

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	require.ErrorIs(t, l.Close(ctx), context.Canceled)
	_, err = l.Lock(t.Context(), "k")
	require.Error(t, err, "the engine is closed")
	require.Error(t, l.Probe(t.Context()))
}
