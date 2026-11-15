// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package oidc

import (
	"encoding/hex"
	"strconv"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/cache/probfilter/bloom"

	bloommemory "github.com/altessa-s/go-atlas/cache/probfilter/bloom/storages/memory"
	bloomredis "github.com/altessa-s/go-atlas/cache/probfilter/bloom/storages/redis"
)

// sharedFilterKey is the Redis key of the filter the sharedReplica storages
// share.
const sharedFilterKey = bloomredis.DefaultKeyPrefix + "oidc-revocation-shared"

// markRevokedInSharedFilter puts item into the shared filter the way an
// insert outside a rebuild does — MarkRevoked by any replica after an
// introspection answer of active: false. The fake module does not emulate the
// journaling insert script, so the item is written to the filter directly.
func markRevokedInSharedFilter(t *testing.T, mr *miniredis.Miniredis, item string) {
	t.Helper()
	require.True(t, mr.Exists(sharedFilterKey), "the shared filter must have been committed")
	held, err := mr.Get(sharedFilterKey)
	require.NoError(t, err)
	if held != "" {
		held += "\n"
	}
	require.NoError(t, mr.Set(sharedFilterKey, held+item))
}

// sharedReadyMarker is the key of the shared filter's ready marker, whose
// value is the commit metadata.
func sharedReadyMarker(t *testing.T, mr *miniredis.Miniredis) string {
	t.Helper()
	key := "__probfilter__:{" + sharedFilterKey + "}:ready:" + hex.EncodeToString([]byte(sharedFilterKey))
	require.True(t, mr.Exists(key), "ready marker %s must exist after a commit", key)
	return key
}

func requireSharedRevoked(t *testing.T, s *filterRevocationStorage, item string, want bool) {
	t.Helper()
	revoked, err := s.IsRevoked(t.Context(), item)
	require.NoError(t, err)
	require.Equal(t, want, revoked)
}

// A revocation list that is legitimately empty stays empty for the guard
// whatever introspection adds to the shared filter: the entries MarkRevoked
// put there are not part of the list, so the next empty sync of any replica
// succeeds without WithRevocationAllowEmpty.
func TestRevocationSync_EmptyListWithMarkedEntriesKeepsSyncing(t *testing.T) {
	t.Parallel()

	client, mr := newSharedBloomRedis(t)
	a, _ := sharedReplica(client)
	b, _ := sharedReplica(client)

	require.NoError(t, a.Sync(t.Context()))
	markRevokedInSharedFilter(t, mr, "inactive-1")
	markRevokedInSharedFilter(t, mr, "inactive-2")
	requireSharedRevoked(t, b, "inactive-1", true)

	require.NoError(t, a.Sync(t.Context()), "the replica that committed the empty list")
	markRevokedInSharedFilter(t, mr, "inactive-3")
	require.NoError(t, b.Sync(t.Context()), "a replica that never synced before")
}

// The same holds for a single-process in-memory filter, with the real
// MarkRevoked.
func TestRevocationSync_MemoryFilterEmptyListWithMarkedEntries(t *testing.T) {
	t.Parallel()

	source := &scriptedSource{}
	storage := &filterRevocationStorage{filter: bloom.New(bloommemory.New()), loader: source}

	require.NoError(t, storage.Sync(t.Context()))
	require.NoError(t, storage.MarkRevoked(t.Context(), "inactive", 0))
	require.NoError(t, storage.Sync(t.Context()))

	source.set("j1")
	require.NoError(t, storage.Sync(t.Context()))
	source.set()
	require.ErrorIs(t, storage.Sync(t.Context()), ErrRevocationLoadFailed)
}

// A list a peer committed with entries is what an erroneous empty response
// must not replace, also when the replica's own last sync loaded nothing. Once
// a peer commits an empty list on purpose, an empty result is accepted again.
func TestRevocationSync_EmptyResultRejectedWhilePeerListHoldsEntries(t *testing.T) {
	t.Parallel()

	client, mr := newSharedBloomRedis(t)
	a, _ := sharedReplica(client)
	b, peerSource := sharedReplica(client, "j1", "j2")

	require.NoError(t, a.Sync(t.Context()))
	markRevokedInSharedFilter(t, mr, "inactive")
	require.NoError(t, b.Sync(t.Context()))

	require.ErrorIs(t, a.Sync(t.Context()), ErrRevocationLoadFailed)
	requireSharedRevoked(t, a, "j1", true)
	requireSharedRevoked(t, a, "j2", true)

	peerSource.set()
	require.ErrorIs(t, b.Sync(t.Context()), ErrRevocationLoadFailed, "the peer's own empty result is guarded the same way")
	b.allowEmpty = true
	require.NoError(t, b.Sync(t.Context()))
	requireSharedRevoked(t, a, "j1", false)

	markRevokedInSharedFilter(t, mr, "inactive")
	require.NoError(t, a.Sync(t.Context()), "the committed list is empty again")
}

// A commit that recorded no list size — a marker in the timestamp-only format
// of the previous release, or the older "1" — leaves the size unknown, and
// the guard falls back to the filter's item count: any entry, marked ones
// included, keeps the set.
func TestRevocationSync_UnknownListSizeFallsBackToItemCount(t *testing.T) {
	t.Parallel()

	for name, marker := range map[string]string{
		"timestamp only": strconv.FormatInt(time.Now().UnixMilli(), 10),
		"legacy":         "1",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			client, mr := newSharedBloomRedis(t)
			a, _ := sharedReplica(client)
			require.NoError(t, a.Sync(t.Context()))
			ready := sharedReadyMarker(t, mr)

			require.NoError(t, mr.Set(ready, marker))
			require.NoError(t, a.Sync(t.Context()), "an empty filter is an empty list")

			require.NoError(t, mr.Set(sharedReadyMarker(t, mr), marker))
			markRevokedInSharedFilter(t, mr, "inactive")
			require.ErrorIs(t, a.Sync(t.Context()), ErrRevocationLoadFailed)
			requireSharedRevoked(t, a, "inactive", true)
		})
	}
}

// A list size that cannot be read counts as entries.
func TestRevocationSync_UnreadableListSizeKeepsSet(t *testing.T) {
	t.Parallel()

	client, mr := newSharedBloomRedis(t)
	a, _ := sharedReplica(client)
	require.NoError(t, a.Sync(t.Context()))

	require.NoError(t, mr.Set(sharedReadyMarker(t, mr), "1700000000000:many"))
	require.ErrorIs(t, a.Sync(t.Context()), ErrRevocationLoadFailed)
}
