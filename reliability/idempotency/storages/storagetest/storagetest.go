// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package storagetest

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/reliability/idempotency"
	"github.com/altessa-s/go-atlas/reliability/idempotency/storages"
)

// contracts lists the storage contracts; [Run] executes each.
var contracts = []struct {
	name  string
	check func(*testing.T, storages.Storage)
}{
	{"KeeperStoresReplayRecord", KeeperStoresReplayRecord},
	{"StealRefusesCompletedRecord", StealRefusesCompletedRecord},
}

// replayResponseSize is the size of the response in the replay record, as
// large as the transports store by default.
const replayResponseSize = 256 << 10

// Run executes every contract as a parallel subtest on a fresh store from
// newStore. The names in skip are reported as skipped.
func Run(t *testing.T, newStore func(testing.TB) storages.Storage, skip ...string) {
	t.Helper()
	for _, c := range contracts {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			if slices.Contains(skip, c.name) {
				t.Skip("not supported by this backend")
			}
			c.check(t, newStore(t))
		})
	}
}

// KeeperStoresReplayRecord completes a key with a transport replay record as
// large as the transports store by default (a 256 KiB response, base64 in the
// JSON state) and reads it back intact from a repeat's AttemptLock.
func KeeperStoresReplayRecord(t *testing.T, store storages.Storage) {
	t.Helper()
	k := idempotency.New(store, idempotency.WithMaxLockDuration(time.Minute))
	pattern := []byte{0xA5, 0x5A}
	response := bytes.Repeat(pattern, replayResponseSize/len(pattern))
	record := map[string]any{"entity_id": "e-1", "response": response}

	ok, held, err := k.AttemptLock(t.Context(), "replay-key")
	require.NoError(t, err)
	require.True(t, ok)
	require.NoError(t, k.Complete(t.Context(), "replay-key", record, held))

	ok, state, err := k.AttemptLock(t.Context(), "replay-key")
	require.NoError(t, err)
	require.False(t, ok)
	require.Equal(t, idempotency.StatusSuccess, state.Status)
	data, isMap := state.Data.(map[string]any)
	require.True(t, isMap, "%T", state.Data)
	require.Equal(t, "e-1", data["entity_id"])
	require.Equal(t, base64.StdEncoding.EncodeToString(response), data["response"])
}

// KeeperUsesStorageClock verifies a fresh lock survives an apparent age
// injected into the holder's wire timestamp. Run it against stores with a
// backend clock, and pass a fresh store with a default TTL above maxAge.
func KeeperUsesStorageClock(t *testing.T, store storages.Storage) {
	t.Helper()
	ctx := t.Context()
	const maxAge = time.Hour
	key := idempotency.ComposeKey("namespace", "", "clock")
	// From the Keeper's perspective, the holder's clock was behind by
	// more than maxAge. The store stamped its own time at acquisition.
	wire, err := json.Marshal(map[string]any{
		"status":    idempotency.StatusInProgress,
		"locked_at": time.Now().Add(-2 * maxAge),
	})
	require.NoError(t, err)
	acquired, _, old, err := store.AttemptLock(ctx, key, wire)
	require.NoError(t, err)
	require.True(t, acquired)

	k := idempotency.New(store, idempotency.WithMaxLockDuration(maxAge))
	acquired, state, err := k.AttemptLock(ctx, "clock")
	require.NoError(t, err)
	require.False(t, acquired)
	require.Equal(t, idempotency.StatusInProgress, state.Status)
	require.Empty(t, state.LockToken())

	stolen, current, token, err := store.Steal(ctx, key, []byte("different bytes"), []byte("replacement"), 0, 0)
	require.NoError(t, err)
	require.False(t, stolen)
	require.Equal(t, wire, current)
	require.Nil(t, token)
	require.NoError(t, store.Complete(ctx, key, []byte("completed by holder"), old))
}

// StealRefusesCompletedRecord verifies that matching completed bytes do not
// authorize a takeover, even when the caller requests no age threshold.
func StealRefusesCompletedRecord(t *testing.T, store storages.Storage) {
	t.Helper()
	ctx := t.Context()
	const key = "completed-key"
	acquired, _, lockToken, err := store.AttemptLock(ctx, key, []byte("lock"))
	require.NoError(t, err)
	require.True(t, acquired)
	done := []byte{1, 0, 255}
	require.NoError(t, store.Complete(ctx, key, done, lockToken))

	stolen, current, token, err := store.Steal(ctx, key, done, []byte("new lock"), 0, time.Minute)
	require.NoError(t, err)
	require.False(t, stolen)
	require.Equal(t, done, current)
	require.Nil(t, token)
	acquired, current, token, err = store.AttemptLock(ctx, key, []byte("another lock"))
	require.NoError(t, err)
	require.False(t, acquired)
	require.Equal(t, done, current)
	require.Nil(t, token)
}
