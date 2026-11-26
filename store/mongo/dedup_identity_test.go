// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo

import (
	"context"
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
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

	m := &repository{dedupIdentity: dedupTenant}
	coll := dedupTestCollection(t)
	model := reflect.TypeFor[bson.M]()
	filter := mustPrepareFilter(t, bson.M{"_id": "shared"})

	ctxA := context.WithValue(t.Context(), dedupTenantKey{}, "A")
	ctxB := context.WithValue(t.Context(), dedupTenantKey{}, "B")

	keyA := m.deduplicationKey(ctxA, coll, model, "find", filter)
	keyB := m.deduplicationKey(ctxB, coll, model, "find", filter)

	require.NotEqual(t, keyA, keyB, "same filter, different identity must not share a singleflight slot")
	require.Equal(t, keyA, m.deduplicationKey(ctxA, coll, model, "find", filter), "same identity + filter must be stable")
}

// TestDeduplicationKey_EmptyIdentity checks that a nil identity func, or one
// returning "", leaves the identity segment empty.
func TestDeduplicationKey_EmptyIdentity(t *testing.T) {
	t.Parallel()

	coll := dedupTestCollection(t)
	model := reflect.TypeFor[bson.M]()
	filter := mustPrepareFilter(t, bson.M{"_id": "x"})
	want := (&repository{}).deduplicationKey(t.Context(), coll, model, "find", filter)

	t.Run("nil identity", func(t *testing.T) {
		t.Parallel()
		m := &repository{}
		require.Equal(t, want, m.deduplicationKey(t.Context(), coll, model, "find", filter))
		require.Empty(t, want.identity)
	})

	t.Run("empty identity", func(t *testing.T) {
		t.Parallel()
		m := &repository{dedupIdentity: func(context.Context) string { return "" }}
		require.Equal(t, want, m.deduplicationKey(t.Context(), coll, model, "find", filter))
		require.Empty(t, want.identity)
	})
}

func dedupTestCollection(t *testing.T) *mongo.Collection {
	t.Helper()
	client, err := mongo.Connect(options.Client().ApplyURI("mongodb://127.0.0.1:1"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, client.Disconnect(context.WithoutCancel(t.Context()))) })
	return client.Database("db").Collection("coll")
}
