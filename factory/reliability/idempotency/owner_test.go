// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/auth/principal"
	"github.com/altessa-s/go-atlas/reliability/idempotency"

	idempotencyconfig "github.com/altessa-s/go-atlas/config/idempotency"
	storageconfig "github.com/altessa-s/go-atlas/config/storage"
	factory "github.com/altessa-s/go-atlas/factory/reliability/idempotency"
	storages "github.com/altessa-s/go-atlas/reliability/idempotency/storages"
)

func memoryCfg() *idempotencyconfig.Config {
	return storageCfg(&idempotencyconfig.StorageConfig{
		Type:   idempotencyconfig.StorageTypeMemory,
		Memory: &storageconfig.MemoryConfig{CleanupSchedule: "@every 1m"},
	})
}

// lockAndComplete takes key as the caller in ctx and completes it with data.
func lockAndComplete(t *testing.T, k *idempotency.Keeper, ctx context.Context, key string, data any) {
	t.Helper()
	ok, held, err := k.AttemptLock(ctx, key)
	require.NoError(t, err)
	require.True(t, ok)
	require.NoError(t, k.Complete(ctx, key, data, held))
}

// A factory-built Keeper binds every record to the verified principal by
// default: caller B reusing caller A's key is refused A's record, while A
// still sees it.
func TestBuild_DefaultOwnerIsPrincipal(t *testing.T) {
	t.Parallel()

	k, err := factory.New(memoryCfg()).Build()
	require.NoError(t, err)

	alice := principal.NewContext(t.Context(), principal.Principal{Subject: "alice", Tenant: "acme"})
	bob := principal.NewContext(t.Context(), principal.Principal{Subject: "bob", Tenant: "acme"})
	lockAndComplete(t, k, alice, "order-1", "entity-a")

	_, _, err = k.AttemptLock(bob, "order-1")
	require.ErrorIs(t, err, idempotency.ErrOwnerMismatch, "another principal must not see the record")

	_, _, err = k.AttemptLock(t.Context(), "order-1")
	require.ErrorIs(t, err, idempotency.ErrOwnerMismatch, "an unauthenticated caller must not see the record")

	ok, st, err := k.AttemptLock(alice, "order-1")
	require.NoError(t, err)
	require.False(t, ok)
	require.Equal(t, storages.StatusSuccess, st.Status)
	require.NotEmpty(t, st.Owner())
}

// UseOwner replaces the principal default, UseKeyNamespace isolates keys and
// UseReplayAuthorizer decides replays.
func TestBuild_UseOwnerNamespaceAuthorizer(t *testing.T) {
	t.Parallel()

	type ctxKey struct{}
	label := func(ctx context.Context) string { s, _ := ctx.Value(ctxKey{}).(string); return s }
	alice := context.WithValue(t.Context(), ctxKey{}, "alice")
	bob := context.WithValue(t.Context(), ctxKey{}, "bob")

	t.Run("owner", func(t *testing.T) {
		t.Parallel()
		k, err := factory.New(memoryCfg()).UseOwner(label).Build()
		require.NoError(t, err)
		lockAndComplete(t, k, alice, "k", "a")
		_, _, err = k.AttemptLock(bob, "k")
		require.ErrorIs(t, err, idempotency.ErrOwnerMismatch)
	})

	t.Run("namespace", func(t *testing.T) {
		t.Parallel()
		k, err := factory.New(memoryCfg()).UseKeyNamespace(label).Build()
		require.NoError(t, err)
		lockAndComplete(t, k, alice, "k", "a")
		ok, _, err := k.AttemptLock(bob, "k")
		require.NoError(t, err)
		require.True(t, ok, "another namespace holds its own key")
	})

	t.Run("authorizer", func(t *testing.T) {
		t.Parallel()
		errHidden := errors.New("hidden")
		k, err := factory.New(memoryCfg()).UseOwner(label).
			UseReplayAuthorizer(func(context.Context, string) error { return errHidden }).Build()
		require.NoError(t, err)
		lockAndComplete(t, k, alice, "k", "a")
		_, _, err = k.AttemptLock(alice, "k")
		require.ErrorIs(t, err, errHidden)
	})
}
