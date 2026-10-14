// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package nats_test

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/internal/natskey/natskeytest"
	"github.com/altessa-s/go-atlas/data/limiters/storages"

	limitnats "github.com/altessa-s/go-atlas/data/limiters/storages/nats"
)

// TestProvider_KeysNATSRejectsVerbatim limits keys the server rejects as they
// are (an IPv6 address among them) and checks each has a budget of its own.
func TestProvider_KeysNATSRejectsVerbatim(t *testing.T) {
	t.Parallel()
	provider := setupProvider(t)
	ctx := t.Context()

	for _, key := range natskeytest.Keys() {
		_, err := provider.Allow(ctx, key, 1, time.Minute)
		require.NoError(t, err, key)
	}
	for _, key := range natskeytest.Keys() {
		_, err := provider.Allow(ctx, key, 1, time.Minute)
		require.ErrorIs(t, err, storages.ErrLimitExceeded, key)

		require.NoError(t, provider.Reset(ctx, key), key)
		_, err = provider.Allow(ctx, key, 1, time.Minute)
		require.NoError(t, err, key)
	}
}

func TestProvider_ValidKeyIsStoredUnderItself(t *testing.T) {
	t.Parallel()
	provider := setupProvider(t)
	ctx := t.Context()

	_, err := provider.Allow(ctx, natskeytest.LegacyKey, 1, time.Minute)
	require.NoError(t, err)

	_, err = provider.KV().Get(ctx, natskeytest.LegacyKey)
	require.NoError(t, err)
}

func TestProvider_KeyTooLong(t *testing.T) {
	t.Parallel()
	provider := setupProvider(t)
	key := strings.Repeat(":", 2000)

	_, err := provider.Allow(t.Context(), key, 1, time.Minute)
	require.ErrorIs(t, err, limitnats.ErrKeyTooLong)
	require.ErrorIs(t, provider.Reset(t.Context(), key), limitnats.ErrKeyTooLong)
}
