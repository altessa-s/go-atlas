// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package nats_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/internal/natskey/natskeytest"
	"github.com/altessa-s/go-atlas/data/mongo"

	cursnats "github.com/altessa-s/go-atlas/data/mongo/cursorstorages/nats"
	mongohelpers "github.com/altessa-s/go-atlas/data/mongo/internal/testhelpers"
)

// TestStorage_KeysNATSRejectsVerbatim stores, loads and deletes cursors under
// keys the server rejects as they are, each in an entry of its own.
func TestStorage_KeysNATSRejectsVerbatim(t *testing.T) {
	t.Parallel()
	s := setupStorage(t)
	ctx := t.Context()
	keys := natskeytest.Keys()

	for _, key := range keys {
		meta := mongohelpers.SampleCursorMetadata()
		meta.CursorIdField = key
		require.NoError(t, s.Store(ctx, key, meta), key)
	}
	for _, key := range keys {
		loaded, err := s.Load(ctx, key)
		require.NoError(t, err, key)
		require.Equal(t, key, loaded.CursorIdField, key)

		require.NoError(t, s.Delete(ctx, key), key)
		_, err = s.Load(ctx, key)
		require.ErrorIs(t, err, mongo.ErrCursorNotFound, key)
	}
}

func TestStorage_ValidKeyIsStoredUnderItself(t *testing.T) {
	t.Parallel()
	s := setupStorage(t)
	ctx := t.Context()

	require.NoError(t, s.Store(ctx, natskeytest.LegacyKey, mongohelpers.SampleCursorMetadata()))

	_, err := s.KV().Get(ctx, natskeytest.LegacyKey)
	require.NoError(t, err)
}

func TestStorage_KeyTooLong(t *testing.T) {
	t.Parallel()
	s := setupStorage(t)
	key := strings.Repeat(":", 2000)

	require.ErrorIs(t, s.Store(t.Context(), key, mongohelpers.SampleCursorMetadata()), cursnats.ErrKeyTooLong)
	_, err := s.Load(t.Context(), key)
	require.ErrorIs(t, err, cursnats.ErrKeyTooLong)
	require.ErrorIs(t, s.Delete(t.Context(), key), cursnats.ErrKeyTooLong)
}
