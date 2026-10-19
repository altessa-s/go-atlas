// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package storagetest

import (
	"bytes"
	"encoding/base64"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/idempotency"
	"github.com/altessa-s/go-atlas/data/idempotency/storages"
)

// contracts lists the storage contracts; [Run] executes each.
var contracts = []struct {
	name  string
	check func(*testing.T, storages.Storage)
}{
	{"KeeperStoresReplayRecord", KeeperStoresReplayRecord},
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
	require.Equal(t, storages.StatusSuccess, state.Status)
	data, isMap := state.Data.(map[string]any)
	require.True(t, isMap, "%T", state.Data)
	require.Equal(t, "e-1", data["entity_id"])
	require.Equal(t, base64.StdEncoding.EncodeToString(response), data["response"])
}
