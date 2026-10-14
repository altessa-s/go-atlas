// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package nats_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/internal/natskey/natskeytest"

	idempnats "github.com/altessa-s/go-atlas/data/idempotency/storages/nats"
)

// TestStorage_KeysNATSRejectsVerbatim runs the whole lifecycle on keys the
// server rejects as they are, and checks that no two of them share an entry.
func TestStorage_KeysNATSRejectsVerbatim(t *testing.T) {
	t.Parallel()
	storage := setupStorage(t)
	ctx := t.Context()
	keys := natskeytest.Keys()

	tokens := make(map[string][]byte, len(keys))
	for _, key := range keys {
		acquired, _, token, err := storage.AttemptLock(ctx, key, []byte("lock "+key))
		require.NoError(t, err, key)
		require.True(t, acquired, key)
		tokens[key] = token
	}

	for _, key := range keys {
		acquired, existing, _, err := storage.AttemptLock(ctx, key, []byte("other"))
		require.NoError(t, err, key)
		require.False(t, acquired, key)
		require.Equal(t, []byte("lock "+key), existing, key)

		stolen, err := storage.Steal(ctx, key, []byte("lock "+key), []byte("stolen "+key))
		require.NoError(t, err, key)
		require.NoError(t, storage.Complete(ctx, key, []byte("done "+key), stolen), key)

		_, existing, _, err = storage.AttemptLock(ctx, key, []byte("other"))
		require.NoError(t, err, key)
		require.Equal(t, []byte("done "+key), existing, key)

		require.NoError(t, storage.Delete(ctx, key), key)
		acquired, _, token, err := storage.AttemptLock(ctx, key, []byte("again"))
		require.NoError(t, err, key)
		require.True(t, acquired, key)
		require.NoError(t, storage.Release(ctx, key, token), key)
	}
}

func TestStorage_ValidKeyIsStoredUnderItself(t *testing.T) {
	t.Parallel()
	storage := setupStorage(t)
	ctx := t.Context()

	_, _, _, err := storage.AttemptLock(ctx, natskeytest.LegacyKey, []byte("v"))
	require.NoError(t, err)

	entry, err := storage.KV().Get(ctx, natskeytest.LegacyKey)
	require.NoError(t, err)
	require.Equal(t, []byte("v"), entry.Value())
}

func TestStorage_KeyTooLong(t *testing.T) {
	t.Parallel()
	storage := setupStorage(t)
	key := strings.Repeat(":", 2000)

	_, _, _, err := storage.AttemptLock(t.Context(), key, []byte("v"))
	require.ErrorIs(t, err, idempnats.ErrKeyTooLong)
	require.ErrorIs(t, storage.Delete(t.Context(), key), idempnats.ErrKeyTooLong)
}
