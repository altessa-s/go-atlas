// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo

import (
	"testing"

	"github.com/oklog/ulid/v2"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// TestStoredCursorKeyIsUnpredictable pins S-8: a stored cursor key is a bearer handle, so its entropy must not follow from the
// previous key. ulid.Make draws from a monotonic math/rand source, which within one millisecond only increments the previous
// entropy and leaves its high bytes as they were; keys drawn from crypto/rand share no such prefix.
func TestStoredCursorKeyIsUnpredictable(t *testing.T) {
	t.Parallel()

	storage := newFakeCursorStorage()
	sort := bson.D{{Key: "_id", Value: 1}}
	const n = 64
	keys := make([]ulid.ULID, 0, n)
	for range n {
		key, err := generateNextCursorToken(t.Context(), "id-1", sort, "_id", cursorFilter{original: bson.M{}}, storage, nil, "", "")
		require.NoError(t, err)
		id, err := ulid.ParseStrict(key)
		require.NoError(t, err)
		keys = append(keys, id)
	}
	sharedPrefix := 0
	for i := 1; i < n; i++ {
		prev, cur := keys[i-1].Entropy(), keys[i].Entropy()
		if keys[i-1].Time() == keys[i].Time() && string(prev[:6]) == string(cur[:6]) {
			sharedPrefix++
		}
	}
	require.Zero(t, sharedPrefix, "consecutive keys share the high entropy bytes of a monotonic source")
	require.Len(t, storage.entries, n)
}
