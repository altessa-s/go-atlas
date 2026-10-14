// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package cacheit_test

import (
	"crypto/rand"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/cache/storages"

	cacheredis "github.com/altessa-s/go-atlas/data/cache/storages/redis"
)

// TestStorage_RoundTrip pins Save, Get, Exists and Delete, and what each
// reports for a key that was never written.
func TestStorage_RoundTrip(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	ctx := t.Context()
	value := []byte{0x00, 0xff, 'v', 0x00, 0x80}

	_, err := f.storage.Get(ctx, "k")
	require.ErrorIs(t, err, storages.ErrMissing)
	exists, err := f.storage.Exists(ctx, "k")
	require.NoError(t, err)
	require.False(t, exists)

	require.NoError(t, f.storage.Save(ctx, "k", value, time.Minute))
	got, err := f.storage.Get(ctx, "k")
	require.NoError(t, err)
	require.Equal(t, value, got, "binary values must round-trip unchanged")
	exists, err = f.storage.Exists(ctx, "k")
	require.NoError(t, err)
	require.True(t, exists)

	require.NoError(t, f.storage.Delete(ctx, "k"))
	_, err = f.storage.Get(ctx, "k")
	require.ErrorIs(t, err, storages.ErrMissing)
	require.NoError(t, f.storage.Delete(ctx, "k"), "deleting a missing key is not an error")
}

// TestStorage_EmptyValueIsNotAMiss pins that a stored empty value is a hit:
// a cache that conflated it with a missing key would re-run the fallback
// for it forever.
func TestStorage_EmptyValueIsNotAMiss(t *testing.T) {
	t.Parallel()
	f := newFixture(t)

	require.NoError(t, f.storage.Save(t.Context(), "k", []byte{}, time.Minute))
	got, err := f.storage.Get(t.Context(), "k")
	require.NoError(t, err)
	require.Empty(t, got)
}

// TestStorage_DeleteMany pins the MULTI/EXEC delete: every named key goes,
// missing ones included, and nothing else does.
func TestStorage_DeleteMany(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	ctx := t.Context()

	for _, key := range []string{"a", "b", "c", "keep"} {
		require.NoError(t, f.storage.Save(ctx, key, []byte(key), time.Minute))
	}
	require.NoError(t, f.storage.DeleteMany(ctx, "a", "b", "c", "never-written"))

	for _, key := range []string{"a", "b", "c"} {
		_, err := f.storage.Get(ctx, key)
		require.ErrorIs(t, err, storages.ErrMissing, key)
	}
	got, err := f.storage.Get(ctx, "keep")
	require.NoError(t, err)
	require.Equal(t, []byte("keep"), got)
}

// TestStorage_TTL pins expiry on the server clock: a TTL ages the key out, a
// zero TTL never does, and a later Save without a TTL clears the old one —
// Redis SET semantics, which the facade's NoTTL relies on.
func TestStorage_TTL(t *testing.T) {
	t.Parallel()

	t.Run("Expires", func(t *testing.T) {
		t.Parallel()
		f := newFixture(t)
		require.NoError(t, f.storage.Save(t.Context(), "k", []byte("v"), shortTTL))
		require.Positive(t, f.pttl(t, "k"))
		require.LessOrEqual(t, f.pttl(t, "k"), shortTTL)

		f.awaitGone(t, "k")
		_, err := f.storage.Get(t.Context(), "k")
		require.ErrorIs(t, err, storages.ErrMissing)
	})

	t.Run("ZeroNeverExpires", func(t *testing.T) {
		t.Parallel()
		f := newFixture(t)
		require.NoError(t, f.storage.Save(t.Context(), "k", []byte("v"), 0))
		require.Equal(t, time.Duration(-1), f.pttl(t, "k"))
	})

	t.Run("OverwriteReplacesTheTTL", func(t *testing.T) {
		t.Parallel()
		f := newFixture(t)
		require.NoError(t, f.storage.Save(t.Context(), "k", []byte("v1"), shortTTL))
		require.NoError(t, f.storage.Save(t.Context(), "k", []byte("v2"), 0))
		require.Equal(t, time.Duration(-1), f.pttl(t, "k"))
	})
}

// TestStorage_LargeValue round-trips an 8 MiB random value over the wire.
func TestStorage_LargeValue(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	value := make([]byte, 8<<20)
	_, _ = rand.Read(value)

	require.NoError(t, f.storage.Save(t.Context(), "big", value, time.Minute))
	got, err := f.storage.Get(t.Context(), "big")
	require.NoError(t, err)
	require.Equal(t, value, got)
}

// TestStorage_PrefixesIsolate pins the key prefix: two storages on one server
// with different prefixes never see each other's keys, not even through
// DeleteMany.
func TestStorage_PrefixesIsolate(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	other := cacheredis.New(f.client, cacheredis.WithPrefix(f.prefix+"other"))
	ctx := t.Context()

	require.NoError(t, f.storage.Save(ctx, "k", []byte("mine"), time.Minute))
	require.NoError(t, other.Save(ctx, "k", []byte("theirs"), time.Minute))
	require.NoError(t, other.DeleteMany(ctx, "k"))

	got, err := f.storage.Get(ctx, "k")
	require.NoError(t, err)
	require.Equal(t, []byte("mine"), got)
	_, err = other.Get(ctx, "k")
	require.ErrorIs(t, err, storages.ErrMissing)
}
