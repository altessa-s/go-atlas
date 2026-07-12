// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package idempotency

import (
	"context"
	"testing"

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
	_, nsA := s.Entry("A:op-1")
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
