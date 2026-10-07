// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package leasing_test

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/locks/dlock/errs"
	"github.com/altessa-s/go-atlas/data/locks/dlock/providers"
	"github.com/altessa-s/go-atlas/data/locks/dlock/providers/internal/leasing"
	"github.com/altessa-s/go-atlas/data/locks/dlock/providers/providertest"
)

// record is one lease of memStore.
type record struct {
	owner             string
	fencing           uint64
	acquired, renewed time.Time
	expires           time.Time
}

// memStore is a [leasing.Store] over a map, with the wall clock as the
// storage clock; one mutex stands in for the storage's record lock.
type memStore struct {
	mu   sync.Mutex
	ttl  time.Duration
	recs map[string]*record
}

func newMemStore(ttl time.Duration) *memStore { return &memStore{ttl: ttl, recs: map[string]*record{}} }

func (m *memStore) Acquire(_ context.Context, key, owner string) (uint64, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now()
	r, ok := m.recs[key]
	if ok && r.expires.After(now) {
		return 0, false, nil
	}
	if !ok {
		r = &record{}
		m.recs[key] = r
	}
	*r = record{owner: owner, fencing: r.fencing + 1, acquired: now, renewed: now, expires: now.Add(m.ttl)}
	return r.fencing, true, nil
}

func (m *memStore) info(key string, r *record) *providers.LockInfo {
	return &providers.LockInfo{Key: key, Owner: r.owner, FencingToken: r.fencing, AcquiredAt: r.acquired, LastRenewed: r.renewed, TTL: m.ttl}
}

func (m *memStore) Read(_ context.Context, key string) (*providers.LockInfo, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.recs[key]
	if !ok || !r.expires.After(time.Now()) {
		return nil, errs.ErrLockNotHeld
	}
	return m.info(key, r), nil
}

func (m *memStore) ReadOwned(_ context.Context, key, owner string, fencing uint64) (*providers.LockInfo, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.recs[key]
	if !ok || !r.expires.After(time.Now()) || r.owner != owner || r.fencing != fencing {
		return nil, errs.ErrLockNotHeld
	}
	return m.info(key, r), nil
}

func (m *memStore) Renew(_ context.Context, key, owner string, fencing uint64) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now()
	r, ok := m.recs[key]
	if !ok || !r.expires.After(now) || r.owner != owner || r.fencing != fencing {
		return false, nil
	}
	r.renewed, r.expires = now, now.Add(m.ttl)
	return true, nil
}

func (m *memStore) Release(_ context.Context, key, owner string, fencing uint64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if r, ok := m.recs[key]; ok && r.owner == owner && (fencing == 0 || r.fencing == fencing) {
		r.owner, r.expires = "", time.Now()
	}
	return nil
}

func (m *memStore) Ping(context.Context) error { return nil }

func (m *memStore) expire(key string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if r, ok := m.recs[key]; ok {
		r.expires = time.Now().Add(-time.Millisecond)
	}
}

func newEngine(tb testing.TB, m *memStore) *leasing.Engine {
	tb.Helper()
	ttl, interval, err := leasing.Timing("test", m.ttl, 1.0/3.0)
	require.NoError(tb, err)
	e := leasing.New(m, leasing.Config{Logger: slog.New(slog.DiscardHandler), TTL: ttl, Interval: interval, OperationsTimeout: time.Second})
	tb.Cleanup(func() { _ = e.Close(context.Background()) })
	return e
}

// engineProvider exposes an Engine as a providers.Provider.
type engineProvider struct{ *leasing.Engine }

var _ providers.Provider = engineProvider{}

// TestProviderContract runs the provider contract suite on the engine over
// the in-memory store.
func TestProviderContract(t *testing.T) {
	t.Parallel()
	providertest.Run(t, func(testing.TB) providertest.Backend {
		m := newMemStore(time.Second)
		return providertest.Backend{
			NewProvider: func(tb testing.TB) providers.Provider { return engineProvider{newEngine(tb, m)} },
			Expire:      func(_ testing.TB, key string) { m.expire(key) },
		}
	})
}

func TestTiming(t *testing.T) {
	t.Parallel()
	ttl, interval, err := leasing.Timing("x", 2900*time.Microsecond, 0.9)
	require.NoError(t, err)
	require.Equal(t, 2*time.Millisecond, ttl)
	require.Less(t, interval, ttl)
	for _, tc := range []struct {
		ttl   time.Duration
		ratio float64
	}{{time.Second, 1}, {time.Second, 0}, {500 * time.Microsecond, 0.5}, {2 * time.Millisecond, 0.1}} {
		_, _, err := leasing.Timing("x", tc.ttl, tc.ratio)
		require.Error(t, err, "%v %v", tc.ttl, tc.ratio)
	}
}

// TestAcquireFaultReleasesByOwner pins that an acquisition the store applied
// but whose reply was lost is released, so the key is free again.
func TestAcquireFaultReleasesByOwner(t *testing.T) {
	t.Parallel()
	m := newMemStore(time.Second)
	e := newEngine(t, m)
	errLost := errors.New("reply lost")
	e.AcquireFault = func() error { return errLost }
	_, err := e.Lock(t.Context(), "k")
	require.ErrorIs(t, err, errLost)
	_, err = e.GetLockInfo(t.Context(), "k")
	require.ErrorIs(t, err, errs.ErrLockNotHeld)
}
