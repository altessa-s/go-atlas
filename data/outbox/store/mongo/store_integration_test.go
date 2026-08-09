// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package outboxstore_test

import (
	"context"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/altessa-s/go-atlas/data/outbox"

	outboxstore "github.com/altessa-s/go-atlas/data/outbox/store/mongo"
	mongoOptions "go.mongodb.org/mongo-driver/v2/mongo/options"
)

// dateDoc decodes an outbox document with Date-typed timestamps (current layout).
type dateDoc struct {
	ID            string     `bson:"_id"`
	Status        string     `bson:"status"`
	CreatedAt     time.Time  `bson:"created_at"`
	PublishedAt   *time.Time `bson:"published_at"`
	LastAttemptOn *time.Time `bson:"last_attempt_on"`
	LockedOn      *time.Time `bson:"locked_on"`
	ExpiresAt     *time.Time `bson:"expires_at"`
}

// unixDoc decodes an outbox document with legacy int64 Unix-second timestamps.
type unixDoc struct {
	ID            string `bson:"_id"`
	CreatedAt     int64  `bson:"created_at"`
	PublishedAt   int64  `bson:"published_at"`
	LastAttemptOn int64  `bson:"last_attempt_on"`
	LockedOn      int64  `bson:"locked_on"`
	ExpiresAt     int64  `bson:"expires_at"`
}

// mongoURI returns the MongoDB connection string, honoring MONGO_URI for CI.
func mongoURI() string {
	if uri := os.Getenv("MONGO_URI"); uri != "" {
		return uri
	}
	return "mongodb://localhost:27017"
}

// newIT connects to a live MongoDB, skipping the test when none is reachable.
// Each call gets its own throwaway database (dropped on cleanup) and a Store bound
// to its outbox_events collection.
func newIT(t *testing.T) (*outboxstore.Store, *mongo.Collection) {
	t.Helper()

	client, err := mongo.Connect(mongoOptions.Client().ApplyURI(mongoURI()))
	if err != nil {
		t.Skipf("mongodb not available: %v", err)
	}
	if err := client.Ping(context.Background(), nil); err != nil {
		_ = client.Disconnect(context.Background())
		t.Skipf("mongodb not reachable at %s: %v", mongoURI(), err)
	}

	dbName := "outbox_it_" + strconv.FormatInt(time.Now().UnixNano(), 10)
	db := client.Database(dbName)
	t.Cleanup(func() {
		_ = db.Drop(context.Background())
		_ = client.Disconnect(context.Background())
	})

	coll := db.Collection("outbox_events")
	store, err := outboxstore.NewWithCollectionOptions(coll)
	if err != nil {
		t.Skipf("mongodb store setup failed (server not usable for tests): %v", err)
	}
	return store, coll
}

func TestIntegration_MigrateTimestampsToDate_ForwardAndRevert(t *testing.T) {
	t.Parallel()
	store, coll := newIT(t)
	ctx := t.Context()
	_ = store

	const sec = int64(1700000000)
	_, err := coll.InsertMany(ctx, []any{
		// m1: optional timestamps absent/zero, as legacy SaveEvents wrote them.
		bson.M{"_id": "m1", "status": "pending", "created_at": sec, "published_at": int64(0), "last_attempt_on": int64(0)},
		// m2: every timestamp set.
		bson.M{"_id": "m2", "status": "sent", "created_at": sec, "published_at": sec + 10,
			"last_attempt_on": sec + 5, "locked_on": sec + 1, "expires_at": sec + 100},
	})
	require.NoError(t, err)

	n, err := outboxstore.MigrateTimestampsToDate(ctx, coll)
	require.NoError(t, err)
	require.Equal(t, int64(2), n)

	// Idempotent: a second run touches nothing.
	n2, err := outboxstore.MigrateTimestampsToDate(ctx, coll)
	require.NoError(t, err)
	require.Equal(t, int64(0), n2)

	// m1: created_at is now a Date; zero optionals were dropped, not mapped to 1970.
	var d1 dateDoc
	require.NoError(t, coll.FindOne(ctx, bson.M{"_id": "m1"}).Decode(&d1))
	require.Equal(t, sec, d1.CreatedAt.Unix())
	require.Nil(t, d1.PublishedAt)
	require.Nil(t, d1.LastAttemptOn)
	require.Nil(t, d1.LockedOn)
	require.Nil(t, d1.ExpiresAt)

	// m2: every timestamp converted to the right instant.
	var d2 dateDoc
	require.NoError(t, coll.FindOne(ctx, bson.M{"_id": "m2"}).Decode(&d2))
	require.Equal(t, sec, d2.CreatedAt.Unix())
	require.NotNil(t, d2.PublishedAt)
	require.Equal(t, sec+10, d2.PublishedAt.Unix())
	require.NotNil(t, d2.LastAttemptOn)
	require.Equal(t, sec+5, d2.LastAttemptOn.Unix())
	require.NotNil(t, d2.LockedOn)
	require.Equal(t, sec+1, d2.LockedOn.Unix())
	require.NotNil(t, d2.ExpiresAt)
	require.Equal(t, sec+100, d2.ExpiresAt.Unix())

	// Revert restores the legacy int64 layout (round-trip).
	nr, err := outboxstore.RevertTimestampsToUnix(ctx, coll)
	require.NoError(t, err)
	require.Equal(t, int64(2), nr)

	nr2, err := outboxstore.RevertTimestampsToUnix(ctx, coll)
	require.NoError(t, err)
	require.Equal(t, int64(0), nr2)

	var u2 unixDoc
	require.NoError(t, coll.FindOne(ctx, bson.M{"_id": "m2"}).Decode(&u2))
	require.Equal(t, sec, u2.CreatedAt)
	require.Equal(t, sec+10, u2.PublishedAt)
	require.Equal(t, sec+100, u2.ExpiresAt)
	require.Equal(t, sec+1, u2.LockedOn)
	require.Equal(t, sec+5, u2.LastAttemptOn)

	// m1: created_at restored; dropped optionals decode back to zero.
	var u1 unixDoc
	require.NoError(t, coll.FindOne(ctx, bson.M{"_id": "m1"}).Decode(&u1))
	require.Equal(t, sec, u1.CreatedAt)
	require.Equal(t, int64(0), u1.PublishedAt)
}

func TestIntegration_ExpireEvents_UsesServerClock(t *testing.T) {
	t.Parallel()
	store, coll := newIT(t)
	ctx := t.Context()

	now := time.Now().UTC()
	require.NoError(t, store.SaveEvents(ctx,
		outbox.Event{Id: "exp", Key: "k", Status: outbox.StatusPending, CreatedAt: now, ExpiresAt: now.Add(-time.Hour)},
		outbox.Event{Id: "live", Key: "k", Status: outbox.StatusPending, CreatedAt: now, ExpiresAt: now.Add(time.Hour)},
	))

	n, err := store.ExpireEvents(ctx)
	require.NoError(t, err)
	require.Equal(t, int64(1), n)

	var expired dateDoc
	require.NoError(t, coll.FindOne(ctx, bson.M{"_id": "exp"}).Decode(&expired))
	require.Equal(t, string(outbox.StatusExpired), expired.Status)
	require.NotNil(t, expired.PublishedAt) // stamped with $$NOW for later cleanup

	var live dateDoc
	require.NoError(t, coll.FindOne(ctx, bson.M{"_id": "live"}).Decode(&live))
	require.Equal(t, string(outbox.StatusPending), live.Status)
}

func TestIntegration_UnlockStuckEvents_UsesServerClock(t *testing.T) {
	t.Parallel()
	store, coll := newIT(t)
	ctx := t.Context()

	now := time.Now().UTC()
	_, err := coll.InsertMany(ctx, []any{
		// Locked an hour ago — stale relative to any positive lockExpiry.
		bson.M{"_id": "stuck", "status": string(outbox.StatusInProgress), "created_at": now, "locked_on": now.Add(-time.Hour)},
		// Just locked — must survive.
		bson.M{"_id": "fresh", "status": string(outbox.StatusInProgress), "created_at": now, "locked_on": now},
	})
	require.NoError(t, err)

	require.NoError(t, store.UnlockStuckEvents(ctx, time.Minute))

	var stuck dateDoc
	require.NoError(t, coll.FindOne(ctx, bson.M{"_id": "stuck"}).Decode(&stuck))
	require.Equal(t, string(outbox.StatusPending), stuck.Status)
	require.Nil(t, stuck.LockedOn) // lock cleared

	var fresh dateDoc
	require.NoError(t, coll.FindOne(ctx, bson.M{"_id": "fresh"}).Decode(&fresh))
	require.Equal(t, string(outbox.StatusInProgress), fresh.Status)
	require.NotNil(t, fresh.LockedOn)
}

func TestIntegration_FetchUnprocessedEvents_LocksWithServerClock(t *testing.T) {
	t.Parallel()
	store, coll := newIT(t)
	ctx := t.Context()

	now := time.Now().UTC()
	require.NoError(t, store.SaveEvents(ctx,
		outbox.Event{Id: "p1", Key: "k", Status: outbox.StatusPending, CreatedAt: now},
	))

	events, err := store.FetchUnprocessedEvents(ctx, 10)
	if err != nil {
		// FetchUnprocessedEvents wraps Find+lock in a transaction, which a
		// standalone mongod rejects; skip rather than fail there.
		t.Skipf("FetchUnprocessedEvents requires a replica set: %v", err)
	}
	require.Len(t, events, 1)
	require.Equal(t, outbox.StatusInProgress, events[0].Status)
	require.NotEmpty(t, events[0].LockToken, "a locked event must carry its fencing token")

	// The lock timestamp was written by the server ($$NOW) as a Date.
	var locked dateDoc
	require.NoError(t, coll.FindOne(ctx, bson.M{"_id": "p1"}).Decode(&locked))
	require.Equal(t, string(outbox.StatusInProgress), locked.Status)
	require.NotNil(t, locked.LockedOn)
}

// A dispatcher that lost its lease must not be able to write its result: the
// unlock sweeper already handed the event to someone else, and a late write
// would erase that worker's outcome.
func TestIntegration_UpdateEvents_FencesStaleLockToken(t *testing.T) {
	t.Parallel()
	store, coll := newIT(t)
	ctx := t.Context()

	now := time.Now().UTC()
	require.NoError(t, store.SaveEvents(ctx,
		outbox.Event{Id: "fenced", Key: "k", Status: outbox.StatusPending, CreatedAt: now},
	))

	events, err := store.FetchUnprocessedEvents(ctx, 10)
	if err != nil {
		t.Skipf("FetchUnprocessedEvents requires a replica set: %v", err)
	}
	require.Len(t, events, 1)

	stale := events[0]
	stale.LockToken = "token-from-a-previous-lease"
	stale.Status = outbox.StatusSent
	require.NoError(t, store.UpdateEvents(ctx, stale))

	var doc dateDoc
	require.NoError(t, coll.FindOne(ctx, bson.M{"_id": "fenced"}).Decode(&doc))
	require.Equal(t, string(outbox.StatusInProgress), doc.Status,
		"a write carrying a stale lock token must not be applied")

	// The live token still works.
	current := events[0]
	current.Status = outbox.StatusSent
	require.NoError(t, store.UpdateEvents(ctx, current))
	require.NoError(t, coll.FindOne(ctx, bson.M{"_id": "fenced"}).Decode(&doc))
	require.Equal(t, string(outbox.StatusSent), doc.Status)
}

func TestIntegration_Stats_CountsBacklogAndDeadLetters(t *testing.T) {
	t.Parallel()
	store, _ := newIT(t)
	ctx := t.Context()

	now := time.Now().UTC()
	require.NoError(t, store.SaveEvents(ctx,
		outbox.Event{Id: "s1", Key: "k", Status: outbox.StatusPending, CreatedAt: now.Add(-time.Hour)},
		outbox.Event{Id: "s2", Key: "k", Status: outbox.StatusFailed, CreatedAt: now},
		outbox.Event{Id: "s3", Key: "k", Status: outbox.StatusMaxAttemptReached, CreatedAt: now},
		outbox.Event{Id: "s4", Key: "k", Status: outbox.StatusRejected, CreatedAt: now},
		outbox.Event{Id: "s5", Key: "k", Status: outbox.StatusSent, CreatedAt: now},
	))

	stats, err := store.Stats(ctx)
	require.NoError(t, err)
	require.Equal(t, int64(2), stats.Pending)
	require.Equal(t, int64(2), stats.DeadLettered)
	require.Positive(t, stats.OldestPendingAge, "the hour-old pending event must show up as lag")
}
