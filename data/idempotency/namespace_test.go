// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package idempotency

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/testhelpers"
)

type tenantCtxKey struct{}

func withTenant(ctx context.Context, tenant string) context.Context {
	return context.WithValue(ctx, tenantCtxKey{}, tenant)
}

func tenantNamespace(ctx context.Context) string {
	ns, _ := ctx.Value(tenantCtxKey{}).(string)
	return ns
}

// TestKeyNamespace_IsolatesIdempotencyKeys pins the tenant-isolation fix: with a
// namespace, two tenants using the same bare idempotency key never share a lock
// or a stored response, so one tenant can never replay another's completed
// result — while same-tenant duplicate detection still works.
func TestKeyNamespace_IsolatesIdempotencyKeys(t *testing.T) {
	t.Parallel()

	s := testhelpers.NewMockIdempotencyStorage()
	k := New(s, WithKeyNamespace(tenantNamespace), WithMaxLockDuration(DefaultMaxLockDuration))

	ctxA := withTenant(t.Context(), "A")
	ctxB := withTenant(t.Context(), "B")

	// Tenant A acquires the lock for bare key "op-1" and completes it.
	ok, st, err := k.AttemptLock(ctxA, "op-1")
	require.NoError(t, err)
	require.True(t, ok, "tenant A should acquire the lock")
	require.NoError(t, k.Complete(ctxA, "op-1", "result-A", st))

	// The entry is stored under the namespaced key; the bare key is never written.
	_, bare := s.Entry("op-1")
	require.False(t, bare, "bare (un-namespaced) idempotency key must not be written")
	_, nsA := s.Entry(ComposeKey("namespace", "A", "op-1"))
	require.True(t, nsA, "entry must be stored under the tenant-namespaced key")

	// Tenant B uses the SAME bare key: it must acquire a fresh lock (ok=true),
	// i.e. it does not collide with — and cannot replay — tenant A's completed
	// response. Before the fix this returned ok=false with A's stored state.
	okB, _, err := k.AttemptLock(ctxB, "op-1")
	require.NoError(t, err)
	require.True(t, okB, "different tenant must not collide on the same bare key")

	// Same-tenant duplicate detection still works: A retrying "op-1" collides.
	okA2, _, err := k.AttemptLock(ctxA, "op-1")
	require.NoError(t, err)
	require.False(t, okA2, "same tenant + key must still be detected as a duplicate")
}

// TestKeyNamespace_CollidingTuplesAreIsolated pins that the storage key
// frames the namespace and the key, so no (namespace, key) pair reaches the
// record of another — not even pairs whose ':'-joined forms are equal, or an
// unnamespaced caller passing another tenant's joined or composed key. Every
// storage operation is covered: an orphaned lock of the first pair is neither
// stolen nor released by the second, and the second's Delete leaves the
// first's result in place.
func TestKeyNamespace_CollidingTuplesAreIsolated(t *testing.T) {
	t.Parallel()

	type tuple struct{ namespace, key string }
	tests := []struct {
		name          string
		first, second tuple
	}{
		{name: "colon in namespace vs colon in key", first: tuple{"a:b", "c"}, second: tuple{"a", "b:c"}},
		{name: "empty namespace impersonates joined key", first: tuple{"A", "op-1"}, second: tuple{"", "A:op-1"}},
		{
			name:   "empty namespace impersonates composed key",
			first:  tuple{"A", "op-1"},
			second: tuple{"", ComposeKey("namespace", "A", "op-1")},
		},
		{name: "same key under another namespace", first: tuple{"A", "op-1"}, second: tuple{"B", "op-1"}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			s := testhelpers.NewMockIdempotencyStorage()
			k := New(s, WithKeyNamespace(tenantNamespace), WithMaxLockDuration(time.Minute))
			ctx1 := withTenant(t.Context(), tc.first.namespace)
			ctx2 := withTenant(t.Context(), tc.second.namespace)
			key1, key2 := tc.first.key, tc.second.key

			ok, _, err := k.AttemptLock(ctx1, key1)
			require.NoError(t, err)
			require.True(t, ok)
			// The first lock looks orphaned: a caller reaching its record
			// would steal it.
			storageKey1 := ComposeKey("namespace", tc.first.namespace, key1)
			forgeStoredLockedAt(t, s, storageKey1, time.Hour)
			orphan, _ := s.Entry(storageKey1)

			ok, held2, err := k.AttemptLock(ctx2, key2)
			require.NoError(t, err)
			require.True(t, ok, "the second tuple must get a lock of its own")
			require.NoError(t, k.Release(ctx2, key2, held2))
			current, found := s.Entry(storageKey1)
			require.True(t, found, "the second's Release must not remove the first lock")
			require.Equal(t, orphan, current, "the second must not steal the first lock")

			// The first caller reclaims its own orphan and completes.
			ok, held1, err := k.AttemptLock(ctx1, key1)
			require.NoError(t, err)
			require.True(t, ok)
			require.NoError(t, k.Complete(ctx1, key1, "first", held1))

			ok, held2, err = k.AttemptLock(ctx2, key2)
			require.NoError(t, err)
			require.True(t, ok, "the second tuple must not replay the first's result")
			require.NoError(t, k.Complete(ctx2, key2, "second", held2))

			ok, state, err := k.AttemptLock(ctx1, key1)
			require.NoError(t, err)
			require.False(t, ok)
			require.Equal(t, "first", state.Data)
			ok, state, err = k.AttemptLock(ctx2, key2)
			require.NoError(t, err)
			require.False(t, ok)
			require.Equal(t, "second", state.Data)

			require.NoError(t, k.Delete(ctx2, key2))
			ok, state, err = k.AttemptLock(ctx1, key1)
			require.NoError(t, err)
			require.False(t, ok, "the second's Delete must not remove the first's result")
			require.Equal(t, "first", state.Data)
		})
	}
}

// TestKeyNamespace_NilAndEmptyAreTheSameNamespace pins that a Keeper without
// WithKeyNamespace and one whose namespace is "" share their records: both
// compose keys in the empty namespace, and neither stores a key unchanged.
func TestKeyNamespace_NilAndEmptyAreTheSameNamespace(t *testing.T) {
	t.Parallel()

	s := testhelpers.NewMockIdempotencyStorage()
	plain := New(s, WithMaxLockDuration(DefaultMaxLockDuration))
	empty := New(s, WithKeyNamespace(func(context.Context) string { return "" }),
		WithMaxLockDuration(DefaultMaxLockDuration))

	ok, held, err := plain.AttemptLock(t.Context(), "op-1")
	require.NoError(t, err)
	require.True(t, ok)
	require.NoError(t, plain.Complete(t.Context(), "op-1", "result", held))

	_, bare := s.Entry("op-1")
	require.False(t, bare, "a key without a namespace must not be stored unchanged")
	_, composed := s.Entry(ComposeKey("namespace", "", "op-1"))
	require.True(t, composed)

	ok, state, err := empty.AttemptLock(t.Context(), "op-1")
	require.NoError(t, err)
	require.False(t, ok)
	require.Equal(t, "result", state.Data)
}
