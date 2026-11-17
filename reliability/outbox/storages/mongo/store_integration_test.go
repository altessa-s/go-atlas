// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo_test

import (
	"context"
	"os"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/altessa-s/go-atlas/reliability/outbox"

	outboxmongo "github.com/altessa-s/go-atlas/reliability/outbox/storages/mongo"
	mongoOptions "go.mongodb.org/mongo-driver/v2/mongo/options"
)

// dateDoc decodes an outbox document with Date-typed timestamps.
type dateDoc struct {
	ID            string     `bson:"_id"`
	Status        string     `bson:"status"`
	CreatedAt     time.Time  `bson:"created_at"`
	PublishedAt   *time.Time `bson:"published_at"`
	LastAttemptOn *time.Time `bson:"last_attempt_on"`
	LockedOn      *time.Time `bson:"locked_on"`
	ExpiresAt     *time.Time `bson:"expires_at"`
}

// itDBSeq keeps database names unique when parallel tests start in the same nanosecond.
var itDBSeq atomic.Int64

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
func newIT(t *testing.T) (*outboxmongo.Store, *mongo.Collection) {
	t.Helper()

	client, err := mongo.Connect(mongoOptions.Client().ApplyURI(mongoURI()))
	if err != nil {
		t.Skipf("mongodb not available: %v", err)
	}
	if err := client.Ping(context.Background(), nil); err != nil {
		_ = client.Disconnect(context.Background())
		t.Skipf("mongodb not reachable at %s: %v", mongoURI(), err)
	}

	dbName := "outbox_it_" + strconv.FormatInt(time.Now().UnixNano(), 10) + "_" + strconv.FormatInt(itDBSeq.Add(1), 10)
	db := client.Database(dbName)
	t.Cleanup(func() {
		_ = db.Drop(context.Background())
		_ = client.Disconnect(context.Background())
	})

	coll := db.Collection("outbox_events")
	store, err := outboxmongo.NewWithCollectionOptions(coll)
	if err != nil {
		t.Skipf("mongodb store setup failed (server not usable for tests): %v", err)
	}
	return store, coll
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

// Events saved together share created_at; the fetch must still return them in
// insertion order.
func TestIntegration_FetchUnprocessedEvents_KeepsBatchOrder(t *testing.T) {
	t.Parallel()
	store, _ := newIT(t)
	ctx := t.Context()

	now := time.Now().UTC()
	ids := []string{"z", "m", "a", "q", "b"}
	batch := make([]outbox.Event, 0, len(ids))
	for _, id := range ids {
		batch = append(batch, outbox.Event{Id: id, Key: "k", Status: outbox.StatusPending, CreatedAt: now})
	}
	require.NoError(t, store.SaveEvents(ctx, batch...))

	events, err := store.FetchUnprocessedEvents(ctx, 10)
	if err != nil {
		t.Skipf("FetchUnprocessedEvents requires a replica set: %v", err)
	}
	got := make([]string, 0, len(events))
	for _, ev := range events {
		got = append(got, ev.Id)
	}
	require.Equal(t, ids, got)
}

// Pending and failed events are fetched by separate queries; the batch merges
// them oldest first and keeps to the batch size.
func TestIntegration_FetchUnprocessedEvents_MergesStatusesInOrder(t *testing.T) {
	t.Parallel()
	store, _ := newIT(t)
	ctx := t.Context()

	now := time.Now().UTC().Truncate(time.Millisecond)
	require.NoError(t, store.SaveEvents(ctx,
		outbox.Event{Id: "p2", Key: "k", Status: outbox.StatusPending, CreatedAt: now.Add(2 * time.Second)},
		outbox.Event{Id: "f1", Key: "k", Status: outbox.StatusFailed, CreatedAt: now.Add(time.Second)},
		outbox.Event{Id: "p0", Key: "k", Status: outbox.StatusPending, CreatedAt: now},
		outbox.Event{Id: "f3", Key: "k", Status: outbox.StatusFailed, CreatedAt: now.Add(3 * time.Second)},
	))

	events, err := store.FetchUnprocessedEvents(ctx, 3)
	if err != nil {
		t.Skipf("FetchUnprocessedEvents requires a replica set: %v", err)
	}
	got := make([]string, 0, len(events))
	for _, ev := range events {
		got = append(got, ev.Id)
	}
	require.Equal(t, []string{"p0", "f1", "p2"}, got)
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

func TestIntegration_SupportsChangeStreams(t *testing.T) {
	t.Parallel()
	store, _ := newIT(t)

	supported, err := store.SupportsChangeStreams(t.Context())
	require.NoError(t, err)
	t.Logf("change streams supported by this deployment: %v", supported)
}

// The watcher exists to turn a save into a wake-up without waiting for the next
// poll tick, so the test asserts exactly that: insert after the stream is open,
// and expect a notification.
func TestIntegration_Watch_NotifiesOnInsert(t *testing.T) {
	t.Parallel()
	store, _ := newIT(t)

	supported, err := store.SupportsChangeStreams(t.Context())
	require.NoError(t, err)
	if !supported {
		t.Skip("deployment is not a replica set or sharded cluster; change streams unavailable")
	}

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	notified := make(chan struct{}, 16)
	watchDone := make(chan error, 1)
	go func() {
		watchDone <- store.Watch(ctx, func() {
			select {
			case notified <- struct{}{}:
			default:
			}
		})
	}()

	// The stream is opened asynchronously; keep saving until it is listening so
	// the test does not race the handshake.
	saved := 0
	deadline := time.After(20 * time.Second)
	for {
		saved++
		require.NoError(t, store.SaveEvents(ctx, outbox.Event{
			Id:        "w" + strconv.Itoa(saved),
			Key:       "orders.created",
			Status:    outbox.StatusPending,
			CreatedAt: time.Now().UTC(),
		}))

		select {
		case <-notified:
			cancel()
			require.NoError(t, <-watchDone, "cancellation must be a clean shutdown")
			return
		case <-deadline:
			t.Fatalf("change stream never reported an insert after %d saves", saved)
		case <-time.After(200 * time.Millisecond):
		}
	}
}

// A standalone mongod has no oplog, so the capability is reported rather than
// retried: the caller keeps polling and the watcher stays out of the way.
func TestIntegration_Watch_UnsupportedDeployment(t *testing.T) {
	t.Parallel()
	store, _ := newIT(t)

	supported, err := store.SupportsChangeStreams(t.Context())
	require.NoError(t, err)
	if supported {
		t.Skip("deployment supports change streams; nothing to assert here")
	}

	require.ErrorIs(t, store.Watch(t.Context(), func() {}), outbox.ErrWatchUnsupported)
}
