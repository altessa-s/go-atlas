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
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/locks/dlock/errs"
	"github.com/altessa-s/go-atlas/data/locks/dlock/providers"
	"github.com/altessa-s/go-atlas/data/locks/dlock/providers/internal/leasing"
	"github.com/altessa-s/go-atlas/data/locks/dlock/providers/providertest"
	"github.com/altessa-s/go-atlas/internal/testhelpers"
)

// leaseRow is one lock row of a leaseDB.
type leaseRow struct {
	owner                                      string
	fencing, acquired, renewed, expires, ttlUs int64
}

// leaseDB emulates the PostgreSQL locks table behind testhelpers.FakeSQL: it
// interprets the four statements the locker issues against an in-memory map
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

// Close must not return while an acquisition that started before it can still
// leave a lease behind.
func TestClose_WaitsForInflightAcquisition(t *testing.T) {
	t.Parallel()
	l := newFakeLocker(t, newLeaseDB())
	entered, release := make(chan struct{}), make(chan struct{})
	l.engine.AfterAcquire = func() {
		close(entered)
		<-release
	}

	lockErr := make(chan error, 1)
	go func() {
		_, err := l.Lock(context.Background(), "k")
		lockErr <- err
	}()
	<-entered

	closed := make(chan error, 1)
	go func() { closed <- l.Close(context.Background()) }()
	select {
	case <-closed:
		t.Fatal("Close returned while an acquisition was in flight")
	case <-time.After(200 * time.Millisecond):
	}
	close(release)
	require.NoError(t, <-closed)
	require.ErrorIs(t, <-lockErr, leasing.ErrClosed)
	_, err := l.GetLockInfo(t.Context(), "k")
	require.ErrorIs(t, err, errs.ErrLockNotHeld, "Close leaves no lease behind")
}

// An acquisition whose reply arrives after the lease's TTL has elapsed may
// already belong to another holder: Lock must reject it and release it.
func TestLock_LateAcquisitionIsRejected(t *testing.T) {
	t.Parallel()
	l := newFakeLocker(t, newLeaseDB())
	base := time.Now()
	var calls atomic.Int32
	l.engine.Now = func() time.Time {
		if calls.Add(1) == 1 {
			return base
		}
		return base.Add(l.s.ttl)
	}

	_, err := l.Lock(t.Context(), "k")
	require.ErrorIs(t, err, errs.ErrLockNotHeld)
	_, err = l.GetLockInfo(t.Context(), "k")
	require.ErrorIs(t, err, errs.ErrLockNotHeld, "the late acquisition must be released")
}

// When no renewal is confirmed within the lease on this process's clock, the
// lock releases the lease instead of abandoning it.
func TestRenew_UnconfirmedLeaseIsReleased(t *testing.T) {
	t.Parallel()
	l := newFakeLocker(t, newLeaseDB(), WithTTL(3*time.Second))
	var offset atomic.Int64
	l.engine.Now = func() time.Time { return time.Now().Add(time.Duration(offset.Load())) }

	_, err := l.Lock(context.Background(), "k")
	require.NoError(t, err)
	offset.Store(int64(4 * time.Second))
	require.Eventually(t, func() bool {
		_, err := l.GetLockInfo(t.Context(), "k")
		return err != nil
	}, 2*time.Second, 20*time.Millisecond, "the live lease must be released, not left to expire")
}

// An acquisition that completes after Close began and cannot be released at
// once stays registered, so a later Close releases it.
func TestClose_RetriesFailedCleanupOfRacingAcquisition(t *testing.T) {
	t.Parallel()
	l := newFakeLocker(t, newLeaseDB())
	entered, release := make(chan struct{}), make(chan struct{})
	l.engine.AfterAcquire = func() {
		close(entered)
		<-release
	}
	var faults atomic.Int32
	errFault := errors.New("release fault")
	l.engine.ReleaseFault = func() error {
		if faults.Add(1) <= 2 {
			return errFault
		}
		return nil
	}

	lockErr := make(chan error, 1)
	go func() {
		_, err := l.Lock(context.Background(), "k")
		lockErr <- err
	}()
	<-entered
	closed := make(chan error, 1)
	go func() { closed <- l.Close(context.Background()) }()
	time.Sleep(50 * time.Millisecond) // let Close mark the provider closed
	close(release)

	require.ErrorIs(t, <-lockErr, leasing.ErrClosed)
	require.ErrorIs(t, <-closed, errFault, "the retained acquisition's release failed again")
	_, err := l.GetLockInfo(t.Context(), "k")
	require.NoError(t, err, "the lease is still live")

	require.NoError(t, l.Close(t.Context()), "a later Close retries the release")
	_, err = l.GetLockInfo(t.Context(), "k")
	require.ErrorIs(t, err, errs.ErrLockNotHeld)
}

// An acquisition applied by the database whose reply is lost is released by
// Lock; when that release fails too, Close retries it.
func TestLock_AmbiguousAcquisitionIsReleased(t *testing.T) {
	t.Parallel()
	errLost := errors.New("reply lost")

	t.Run("released_by_lock", func(t *testing.T) {
		t.Parallel()
		l := newFakeLocker(t, newLeaseDB())
		l.engine.AcquireFault = func() error { return errLost }

		_, err := l.Lock(t.Context(), "k")
		require.ErrorIs(t, err, errLost)
		_, err = l.GetLockInfo(t.Context(), "k")
		require.ErrorIs(t, err, errs.ErrLockNotHeld)
	})

	t.Run("retried_by_close", func(t *testing.T) {
		t.Parallel()
		l := newFakeLocker(t, newLeaseDB())
		l.engine.AcquireFault = func() error { return errLost }
		var faults atomic.Int32
		l.engine.ReleaseFault = func() error {
			if faults.Add(1) == 1 {
				return errLost
			}
			return nil
		}

		_, err := l.Lock(t.Context(), "k")
		require.ErrorIs(t, err, errLost)
		_, err = l.GetLockInfo(t.Context(), "k")
		require.NoError(t, err, "the applied lease is still live")
		require.NoError(t, l.Close(t.Context()))
		_, err = l.GetLockInfo(t.Context(), "k")
		require.ErrorIs(t, err, errs.ErrLockNotHeld)
	})
}
