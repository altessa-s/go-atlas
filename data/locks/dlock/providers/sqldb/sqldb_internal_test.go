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

	"github.com/altessa-s/go-atlas/data/locks/dlock/providers"
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
