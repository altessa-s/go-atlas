// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
)

type dedupTenantKey struct{}

func dedupTenant(ctx context.Context) string {
	id, _ := ctx.Value(dedupTenantKey{}).(string)
	return id
}

// TestDeduplicationKey_IsolatesByIdentity pins the fix: with an identity func,
// the same filter from two callers yields distinct singleflight keys, so a
// concurrent GetEntity/GetEntities never collapses across tenants — while it
// stays stable within one tenant, preserving legitimate deduplication.
func TestDeduplicationKey_IsolatesByIdentity(t *testing.T) {
	t.Parallel()

	m := &Mongo{config: &config{dedupIdentity: dedupTenant}}
	filter := bson.M{"_id": "shared"}

	ctxA := context.WithValue(t.Context(), dedupTenantKey{}, "A")
	ctxB := context.WithValue(t.Context(), dedupTenantKey{}, "B")

	keyA := m.deduplicationKey(ctxA, "db:coll", filter)
	keyB := m.deduplicationKey(ctxB, "db:coll", filter)

	require.NotEqual(t, keyA, keyB, "same filter, different identity must not share a singleflight slot")
	require.Equal(t, keyA, m.deduplicationKey(ctxA, "db:coll", filter), "same identity + filter must be stable")
}

// TestDeduplicationKey_BackwardCompatible checks that a nil identity func, or one
// returning "", leaves the key identical to the un-prefixed form — so existing
// single-tenant callers are unaffected.
func TestDeduplicationKey_BackwardCompatible(t *testing.T) {
	t.Parallel()

	filter := bson.M{"_id": "x"}
	want := generateDeduplicationKey("db:coll", filter)

	t.Run("nil identity", func(t *testing.T) {
		t.Parallel()
		m := &Mongo{config: &config{}}
		require.Equal(t, want, m.deduplicationKey(t.Context(), "db:coll", filter))
	})

	t.Run("empty identity", func(t *testing.T) {
		t.Parallel()
		m := &Mongo{config: &config{dedupIdentity: func(context.Context) string { return "" }}}
		require.Equal(t, want, m.deduplicationKey(t.Context(), "db:coll", filter))
	})
}
