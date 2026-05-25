// Copyright 2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package secrets_test

import (
	"context"
	"iter"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/security/secrets"
)

// slowMockProvider lets the test pause the Value() round-trip so a
// Save() can race a still-in-flight fetch — the exact scenario the
// CAS-style fix is designed to handle. Only the FIRST Value() call
// is blocked on releaseCh; subsequent Value() calls (notably the one
// Save() makes internally to re-cache the freshly-saved value) pass
// through immediately so the test doesn't deadlock waiting for itself.
type slowMockProvider struct {
	mu         sync.Mutex
	values     map[string]*secrets.Value[string]
	releaseCh  chan struct{}
	valueCalls atomic.Int64
}

func (m *slowMockProvider) Name() string { return "slow-mock" }

func (m *slowMockProvider) List(_ context.Context) ([]*secrets.Value[string], error) {
	return nil, nil
}

func (m *slowMockProvider) Values(_ context.Context) iter.Seq2[*secrets.Value[string], error] {
	return func(_ func(*secrets.Value[string], error) bool) {}
}

func (m *slowMockProvider) Value(ctx context.Context, key string) (*secrets.Value[string], error) {
	callIdx := m.valueCalls.Add(1)

	// Snapshot the value before blocking so we deliver what was current
	// at the moment the fetch was issued (modeling a slow storage
	// round-trip that committed to "stale" data before Save raced in).
	m.mu.Lock()
	snapshot, ok := m.values[key]
	m.mu.Unlock()

	// Only the first Value() call blocks. Save() internally calls
	// Value() again to re-cache after the storage write — that nested
	// call must pass through immediately or the test deadlocks waiting
	// for releaseCh while Save holds it open.
	if callIdx == 1 && m.releaseCh != nil {
		select {
		case <-m.releaseCh:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		// Re-snapshot AFTER unblocking so we still deliver the OLD
		// value (the one this call captured before being held). This
		// models a slow storage that committed to its read snapshot at
		// request time and only returned later.
		_ = snapshot // intentional: keep the pre-block snapshot below
	}

	if !ok {
		return nil, secrets.ErrNotFound
	}
	return snapshot, nil
}

func (m *slowMockProvider) Save(_ context.Context, key string, value string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.values[key] = secrets.NewValue(key, value, []byte(value), "v-"+value)
	return nil
}

func (m *slowMockProvider) Delete(_ context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.values, key)
	return nil
}

// TestManager_Save_DoesNotLoseToInFlightFetch is the regression guard
// for the singleflight + rotation race. Sequence under test:
//
//  1. Seed the storage with the OLD value.
//  2. Goroutine A calls Value(force=true) → enters singleflight → hits
//     a blocked Value() call inside the mock storage (slowMockProvider
//     captures the OLD snapshot at this point).
//  3. Goroutine B calls Save() with the NEW value. Save bumps the
//     per-key version BEFORE its own cache.Put and publishes NEW.
//  4. The test releases the slow mock; goroutine A's fetch returns
//     OLD. Without the fix, A's cache.Put(OLD) would overwrite the
//     fresh NEW value Save just wrote. With the fix, A observes the
//     version mismatch and skips the cache write.
//  5. Final assertion: the cached value visible to a subsequent
//     Value(force=false) call must be NEW, not OLD.
func TestManager_Save_DoesNotLoseToInFlightFetch(t *testing.T) {
	provider := &slowMockProvider{
		values:    map[string]*secrets.Value[string]{},
		releaseCh: make(chan struct{}),
	}
	// Seed the OLD value so the in-flight fetch has something to
	// return.
	require.NoError(t, provider.Save(t.Context(), "k", "old"))

	mgr, err := secrets.New[string](provider)
	require.NoError(t, err)

	// Goroutine A: in-flight fetch.
	type result struct {
		val *secrets.Value[string]
		err error
	}
	aDone := make(chan result, 1)
	go func() {
		val, err := mgr.Value(t.Context(), "k", true)
		aDone <- result{val, err}
	}()

	// Give A a moment to enter the storage Value() call and capture
	// the OLD snapshot. Without this sleep we'd race the goroutine
	// scheduler; this is the minimum coordination needed to model
	// the audit's "fetch already in flight" precondition.
	time.Sleep(50 * time.Millisecond)

	// Goroutine B: Save the NEW value while A is blocked.
	require.NoError(t, mgr.Save(t.Context(), "k", "new"))

	// Release A.
	close(provider.releaseCh)
	got := <-aDone
	require.NoError(t, got.err)
	require.Equal(t, "old", got.val.Value,
		"goroutine A must still receive its own fetch result (OLD); the fix only protects the cache, not the in-flight return value")

	// The decisive assertion: a subsequent read MUST see NEW, not the
	// stale OLD that an unprotected cache.Put would have published.
	cached, err := mgr.Value(t.Context(), "k", false)
	require.NoError(t, err)
	require.Equal(t, "new", cached.Value,
		"cache MUST hold the post-Save value — a stale fetch returning after Save must not silently overwrite the fresh value (audit fix)")
}
