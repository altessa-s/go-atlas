// Copyright 2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package cache

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/cache/providers"
)

type tenantCtxKey struct{}

func withTenant(ctx context.Context, tenant string) context.Context {
	return context.WithValue(ctx, tenantCtxKey{}, tenant)
}

func tenantNamespace(ctx context.Context) string {
	ns, _ := ctx.Value(tenantCtxKey{}).(string)
	return ns
}

// TestKeyNamespace_IsolatesEntries verifies that WithKeyNamespace prefixes every
// backend operation so two tenants using the same bare key never share an entry.
func TestKeyNamespace_IsolatesEntries(t *testing.T) {
	t.Parallel()

	mp := newMockProvider()
	c := New(mp, WithKeyNamespace(tenantNamespace))

	ctxA := withTenant(t.Context(), "A")
	ctxB := withTenant(t.Context(), "B")

	require.NoError(t, c.Save(ctxA, "user:1", "alice"))
	require.NoError(t, c.Save(ctxB, "user:1", "bob"))

	// Same bare key, two tenants → two distinct namespaced entries, no bare key.
	_, bare := mp.store["user:1"]
	require.False(t, bare, "bare (un-namespaced) key must not be written")
	require.Contains(t, mp.store, "A:user:1")
	require.Contains(t, mp.store, "B:user:1")

	// Each tenant reads back its own value.
	var got string
	require.NoError(t, c.Get(ctxA, "user:1", &got))
	require.Equal(t, "alice", got)
	require.NoError(t, c.Get(ctxB, "user:1", &got))
	require.Equal(t, "bob", got)

	// Delete is namespaced too: deleting under A leaves B intact.
	require.NoError(t, c.Delete(ctxA, "user:1"))
	require.NotContains(t, mp.store, "A:user:1")
	require.Contains(t, mp.store, "B:user:1")
}

// syncProvider is a concurrency-safe in-memory provider for the singleflight
// isolation test (the package's mockProvider is not mutex-guarded).
type syncProvider struct {
	mu    sync.Mutex
	store map[string][]byte
}

func newSyncProvider() *syncProvider { return &syncProvider{store: map[string][]byte{}} }

func (p *syncProvider) Save(_ context.Context, key string, value []byte, _ time.Duration) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.store[key] = value
	return nil
}

func (p *syncProvider) Get(_ context.Context, key string) ([]byte, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if v, ok := p.store[key]; ok {
		return v, nil
	}
	return nil, providers.ErrMissing
}

func (p *syncProvider) Delete(_ context.Context, key string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.store, key)
	return nil
}

func (p *syncProvider) DeleteMany(_ context.Context, keys ...string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, k := range keys {
		delete(p.store, k)
	}
	return nil
}

func (p *syncProvider) Exists(_ context.Context, key string) (bool, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	_, ok := p.store[key]
	return ok, nil
}

// TestKeyNamespace_SingleflightIsolation pins the Critical fix: singleflight must
// collapse concurrent callers only within a namespace, never across tenants. Two
// concurrent GetWithFallback calls with the same bare key run one fallback when
// they share a tenant, but two independent fallbacks when their tenants differ —
// so a tenant can never receive another tenant's fallback result.
func TestKeyNamespace_SingleflightIsolation(t *testing.T) {
	t.Parallel()

	run := func(t *testing.T, tenants [2]string, wantCalls int64) {
		t.Helper()
		c := New(newSyncProvider(), WithKeyNamespace(tenantNamespace))

		var calls atomic.Int64
		started := make(chan struct{}, len(tenants))
		release := make(chan struct{})

		var wg sync.WaitGroup
		results := make([]string, len(tenants))
		errs := make([]error, len(tenants))
		for i, tenant := range tenants {
			wg.Go(func() {
				ctx := withTenant(t.Context(), tenant)
				var out string
				// Assert only on the test goroutine after Wait — require.FailNow
				// must not be called from a spawned goroutine.
				errs[i] = c.GetWithFallback(ctx, "user:1", &out, func() (any, time.Duration, error) {
					calls.Add(1)
					started <- struct{}{}
					<-release // hold the fallback in-flight until the barrier lifts
					return "value-" + tenant, TTLUseDefault, nil
				})
				results[i] = out
			})
		}

		// Wait until the expected number of fallbacks are concurrently in-flight,
		// then release them together. If the same-tenant case had collapsed
		// incorrectly (or the cross-tenant case had fanned in), this would block.
		for range wantCalls {
			<-started
		}
		close(release)
		wg.Wait()

		require.Equal(t, wantCalls, calls.Load())
		for i, tenant := range tenants {
			require.NoError(t, errs[i])
			require.Equal(t, "value-"+tenant, results[i])
		}
	}

	t.Run("same tenant collapses to one fallback", func(t *testing.T) {
		t.Parallel()
		run(t, [2]string{"A", "A"}, 1)
	})

	t.Run("different tenants never fan in", func(t *testing.T) {
		t.Parallel()
		run(t, [2]string{"A", "B"}, 2)
	})
}
