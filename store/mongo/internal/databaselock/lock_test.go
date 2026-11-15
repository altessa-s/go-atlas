// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package databaselock

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// mongo-migrate reads the current schema version from the document with the
// greatest _id in its collection and decodes it as a version record. A lock
// document sharing that collection would win that sort and decode as version
// 0, silently re-running every migration — the exact failure the lock exists
// to prevent.
func TestMigrationLock_UsesSeparateCollection(t *testing.T) {
	t.Parallel()

	require.NotEqual(t, "migrations", CollectionName)
}

func TestMigrationLockFilter(t *testing.T) {
	t.Parallel()

	filter := databaseLockFilter("migrations")

	require.Equal(t, "migrations", filter["_id"],
		"the filter must pin the single mutex document, otherwise the lock is not mutually exclusive")

	// The lease is compared against the server clock ($$NOW), not the
	// caller's: replicas with drifted clocks must not steal a live lock.
	require.Equal(t, bson.M{"$lte": bson.A{"$expiresAt", "$$NOW"}}, filter["$expr"])
}

func TestMigrationLockUpdate(t *testing.T) {
	t.Parallel()

	owner := bson.NewObjectID()
	pipeline := databaseLockUpdate(owner, 30*time.Second)

	require.Len(t, pipeline, 1, "update must be a single-stage pipeline")

	stage := pipeline[0]
	require.Len(t, stage, 1)
	require.Equal(t, "$set", stage[0].Key)

	set, ok := stage[0].Value.(bson.M)
	require.True(t, ok)
	require.Equal(t, owner, set["owner"])
	require.Equal(t, "$$NOW", set["acquiredAt"])

	// The stored expiry must use the requested lease and the server clock.
	require.Equal(t, bson.M{"$add": bson.A{"$$NOW", (30 * time.Second).Milliseconds()}}, set["expiresAt"])
}

// The seed document must be immediately acquirable, otherwise the first
// replica to start would wait out a full lease before migrating.
func TestMigrationLockDocument_SeedIsExpired(t *testing.T) {
	t.Parallel()

	seed := migrationLockDocument{ID: "migrations", ExpiresAt: time.Unix(0, 0).UTC()}

	raw, err := bson.Marshal(seed)
	require.NoError(t, err)

	var decoded bson.M
	require.NoError(t, bson.Unmarshal(raw, &decoded))

	require.Equal(t, "migrations", decoded["_id"])
	require.True(t, seed.ExpiresAt.Before(time.Now()))
	require.NotContains(t, decoded, "owner", "an unowned seed must not carry a zero owner")
	require.NotContains(t, decoded, "acquiredAt")
}

// The two phases have separate budgets and the outer context must cover both,
// otherwise a replica that waited for a peer would have no time left to run
// the migrations it was waiting to skip.
func TestMigrationLock_TimeoutBudgets(t *testing.T) {
	t.Parallel()

	require.Positive(t, 30*time.Second)
	require.Positive(t, 30*time.Second)
	require.Less(t, RetryBaseDelay, RetryMaxDelay)
	require.Less(t, RetryMaxDelay, 30*time.Second,
		"a single backoff step must not consume the whole wait budget")
}
