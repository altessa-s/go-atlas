// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package redisfilter_test

import (
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/alicebob/miniredis/v2/server"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/cache/probfilter"
	"github.com/altessa-s/go-atlas/cache/probfilter/internal/redisfilter"
)

// existsOnlyForKey answers the exists command with false for every value, like
// RedisBloom answers a filter that holds none of them (or does not exist).
func existsOnlyForKey(t *testing.T, mr *miniredis.Miniredis) {
	t.Helper()
	require.NoError(t, mr.Server().Register(testCommands.Exists, func(c *server.Peer, _ string, _ []string) {
		writeBool(c, false)
	}))
}

// TestCore_MightExist_LostFilterFailsClosed pins that, once this process has
// seen the filter key, a missing key is an error rather than "absent".
func TestCore_MightExist_LostFilterFailsClosed(t *testing.T) {
	t.Parallel()

	mr := miniredis.RunT(t)
	existsOnlyForKey(t, mr)
	core := redisfilter.New(protocolClient(t, mr, 2), "t:f", testCommands)

	// The filter was never seen: a missing key is an empty filter.
	ok, err := core.MightExist(t.Context(), "a")
	require.NoError(t, err)
	require.False(t, ok)

	require.NoError(t, mr.Set("t:f", "filter"))
	ok, err = core.MightExist(t.Context(), "a")
	require.NoError(t, err)
	require.False(t, ok)

	mr.Del("t:f")
	_, err = core.MightExist(t.Context(), "a")
	require.ErrorIs(t, err, probfilter.ErrFilterLost)
}

// TestCore_MightExist_TreatMissingAsEmpty pins the explicit permissive mode.
func TestCore_MightExist_TreatMissingAsEmpty(t *testing.T) {
	t.Parallel()

	mr := miniredis.RunT(t)
	existsOnlyForKey(t, mr)
	core := redisfilter.New(protocolClient(t, mr, 2), "t:f", testCommands)
	core.TreatMissingAsEmpty()

	require.NoError(t, mr.Set("t:f", "filter"))
	_, err := core.MightExist(t.Context(), "a")
	require.NoError(t, err)

	mr.Del("t:f")
	ok, err := core.MightExist(t.Context(), "a")
	require.NoError(t, err)
	require.False(t, ok)
}
