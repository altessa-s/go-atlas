// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package idempotency_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/idempotency"
	"github.com/altessa-s/go-atlas/data/idempotency/storages"
	"github.com/altessa-s/go-atlas/data/idempotency/storages/memory"
	"github.com/altessa-s/go-atlas/internal/testhelpers"

	idnats "github.com/altessa-s/go-atlas/data/idempotency/storages/nats"
	idredis "github.com/altessa-s/go-atlas/data/idempotency/storages/redis"
)

// ownershipStores lists every backend the ownership tests run against.
func ownershipStores() []struct {
	name     string
	newStore func(*testing.T) storages.Storage
} {
	return []struct {
		name     string
		newStore func(*testing.T) storages.Storage
	}{
		{"memory", func(t *testing.T) storages.Storage { return memory.New() }},
		{"mock", func(t *testing.T) storages.Storage { return testhelpers.NewMockIdempotencyStorage() }},
		{"redis", func(t *testing.T) storages.Storage {
			client, _ := testhelpers.RedisClient(t)
			return idredis.New(client)
		}},
		{"nats", func(t *testing.T) storages.Storage {
			server := testhelpers.StartNATSServer(t)
			_, js := testhelpers.ConnectJetStream(t, server)
			store, err := idnats.New(js, idnats.WithBucket("release_test"))
			require.NoError(t, err)
			return store
		}},
	}
}

// TestCompleteRejectsDeniedState guards Complete's ownership check: the
// state AttemptLock returns to a caller denied the lock carries no token,
// and backends treat a nil token as an unconditional overwrite.
func TestCompleteRejectsDeniedState(t *testing.T) {
	t.Parallel()
	for _, tc := range ownershipStores() {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			keeper := idempotency.New(tc.newStore(t))
			ctx := t.Context()

			acquired, owner, err := keeper.AttemptLock(ctx, "key")
			require.NoError(t, err)
			require.True(t, acquired)

			acquired, denied, err := keeper.AttemptLock(ctx, "key")
			require.NoError(t, err)
			require.False(t, acquired)
			require.ErrorIs(t, keeper.Complete(ctx, "key", "from-denied", denied), idempotency.ErrMissingLockState)

			require.NoError(t, keeper.Complete(ctx, "key", "from-owner", owner), "the owner's lock must be untouched")
			_, result, err := keeper.AttemptLock(ctx, "key")
			require.NoError(t, err)
			require.Equal(t, "from-owner", result.Data)
		})
	}
}

func TestReleaseOwnership(t *testing.T) {
	t.Parallel()
	for _, tc := range ownershipStores() {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			store := tc.newStore(t)
			ctx := t.Context()
			namespace, storageKey := "tenant", "tenant:key"
			if tc.name == "nats" {
				namespace, storageKey = "", "key"
			}
			keeper := idempotency.New(store, idempotency.WithMaxLockDuration(idempotency.DefaultMaxLockDuration),
				idempotency.WithKeyNamespace(func(context.Context) string { return namespace }))
			require.ErrorIs(t, keeper.Release(ctx, "", nil), idempotency.ErrEmptyKey)
			require.ErrorIs(t, keeper.Release(ctx, "key", nil), idempotency.ErrMissingLockState)
			require.ErrorIs(t, keeper.Release(ctx, "key", &storages.State{}), idempotency.ErrMissingLockState)
			require.ErrorIs(t, store.Release(ctx, "key", nil), storages.ErrMissingLockState)
			require.ErrorIs(t, store.Release(ctx, "", nil), storages.ErrEmptyKey)
			acquired, old, err := keeper.AttemptLock(ctx, "key")
			require.NoError(t, err)
			require.True(t, acquired)
			_, current, _, err := store.AttemptLock(ctx, storageKey, []byte("unused"))
			require.NoError(t, err)
			newWire := append(current, ' ')
			token, err := store.Steal(ctx, storageKey, current, newWire)
			require.NoError(t, err)
			require.ErrorIs(t, keeper.Release(ctx, "key", old), idempotency.ErrLockStolen)
			acquired, _, err = keeper.AttemptLock(ctx, "key")
			require.NoError(t, err)
			require.False(t, acquired, "stale cleanup must leave the new owner locked")
			fresh := &storages.State{}
			fresh.SetLockToken(token)
			require.NoError(t, keeper.Release(ctx, "key", fresh))
			require.ErrorIs(t, keeper.Release(ctx, "key", fresh), idempotency.ErrLockStolen)
			acquired, fresh, err = keeper.AttemptLock(ctx, "key")
			require.NoError(t, err)
			require.True(t, acquired)
			require.NoError(t, keeper.Complete(ctx, "key", "result", fresh))
			require.ErrorIs(t, keeper.Release(ctx, "key", fresh), idempotency.ErrLockStolen)
			acquired, result, err := keeper.AttemptLock(ctx, "key")
			require.NoError(t, err)
			require.False(t, acquired)
			require.Equal(t, "result", result.Data)
		})
	}
}
