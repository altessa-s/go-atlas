// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package outboxit_test

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/altessa-s/go-atlas/data/outbox"
	"github.com/altessa-s/go-atlas/tests/integration/outboxit"

	outboxstore "github.com/altessa-s/go-atlas/data/outbox/store/mongo"
	mongoOptions "go.mongodb.org/mongo-driver/v2/mongo/options"
)

const (
	// eventsCollection is the outbox collection every fixture uses.
	eventsCollection = "outbox_events"

	// ordersCollection stands in for the business data a Save shares its
	// transaction with.
	ordersCollection = "orders"

	// handleTimeout bounds one dispatch cycle. Short, but comfortably above a
	// local round trip, so a slow machine does not turn into a flaky failure.
	handleTimeout = 5 * time.Second

	// lockTime must exceed handleTimeout — otherwise the unlock sweeper could
	// reclaim an event that is still being published, which is the very
	// condition the contention scenarios are meant to rule out rather than
	// accidentally reproduce.
	lockTime = 3 * handleTimeout

	// retryBaseDelay is the backoff before a second attempt. Long enough that a
	// test can observe an event being withheld, short enough to wait out.
	retryBaseDelay = 2 * time.Second

	// settleWindow is how long the assertions allow a condition to become true.
	settleWindow = 10 * time.Second

	// samplingInterval is the poll cadence for those assertions.
	samplingInterval = 25 * time.Millisecond
)

// mongoURI returns the MongoDB connection string, matching
// tests/integration/docker-compose.yml.
//
// directConnection is required: the compose service is a single-node replica
// set that advertises the address it sees inside its own container, so a driver
// doing topology discovery from the host would follow that advertisement to a
// port nothing listens on.
func mongoURI() string {
	if uri := os.Getenv("MONGO_URI"); uri != "" {
		return uri
	}
	return "mongodb://127.0.0.1:27019/?directConnection=true"
}

// dbSequence disambiguates database names. A timestamp alone is not enough:
// these tests are parallel, so several fixtures are built within the same clock
// tick, and two of them landing on one database would have each test's
// dispatcher draining the other's events.
var dbSequence atomic.Int64

// databaseName derives a per-test database name. The test name makes a failure
// traceable to its data; the sequence number guarantees uniqueness even for
// subtests that share a name across packages.
func databaseName(tb testing.TB) string {
	tb.Helper()

	safe := strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' {
			return r
		}
		return '_'
	}, tb.Name())

	// MongoDB caps database names at 63 bytes, so keep the suffix and trim the
	// descriptive part rather than the other way round.
	const maxNameLen = 40
	if len(safe) > maxNameLen {
		safe = safe[:maxNameLen]
	}

	return fmt.Sprintf("outboxit_%s_%d", safe, dbSequence.Add(1))
}

// fixture is one test's isolated world: its own database, an outbox store over
// it, and the recorder standing in for the destination.
type fixture struct {
	db       *mongo.Database
	events   *mongo.Collection
	orders   *mongo.Collection
	store    *outboxstore.Store
	recorder *outboxit.Recorder
	client   *mongo.Client
}

// newFixture connects to MongoDB and gives the test a throwaway database,
// dropped on cleanup. It skips — rather than fails — when no server is
// reachable or when the server is not a replica set: the outbox locks its batch
// inside a transaction, which a standalone mongod does not support, so the
// suite has nothing to say there.
func newFixture(tb testing.TB) *fixture {
	tb.Helper()

	client, err := mongo.Connect(mongoOptions.Client().
		ApplyURI(mongoURI()).
		SetServerSelectionTimeout(3 * time.Second))
	if err != nil {
		tb.Skipf("MongoDB unreachable at %s (%v) — start it with: docker compose -f tests/integration/docker-compose.yml up -d --wait mongo", mongoURI(), err)
	}

	ctx := context.Background()
	if err = client.Ping(ctx, nil); err != nil {
		_ = client.Disconnect(ctx)
		tb.Skipf("MongoDB unreachable at %s (%v) — start it with: docker compose -f tests/integration/docker-compose.yml up -d --wait mongo", mongoURI(), err)
	}

	db := client.Database(databaseName(tb))
	tb.Cleanup(func() {
		_ = db.Drop(context.Background())
		_ = client.Disconnect(context.Background())
	})

	// Create both collections up front. A transaction that has to create one
	// implicitly is a needless extra failure mode, and the business collection
	// has to exist before the atomicity scenarios write to it.
	require.NoError(tb, db.CreateCollection(ctx, eventsCollection))
	require.NoError(tb, db.CreateCollection(ctx, ordersCollection))

	store, err := outboxstore.NewWithCollectionOptions(db.Collection(eventsCollection))
	require.NoError(tb, err)

	f := &fixture{
		db:       db,
		events:   db.Collection(eventsCollection),
		orders:   db.Collection(ordersCollection),
		store:    store,
		recorder: outboxit.NewRecorder(),
		client:   client,
	}

	f.requireTransactions(tb)

	return f
}

// requireTransactions skips the test unless the server can start one. The
// message names the cause explicitly — a standalone mongod fails this in a way
// that is otherwise reported as an opaque command error deep inside a fetch.
func (f *fixture) requireTransactions(tb testing.TB) {
	tb.Helper()

	sess, err := f.client.StartSession()
	if err != nil {
		tb.Skipf("MongoDB sessions unavailable (%v) — the outbox needs a replica set, not a standalone mongod", err)
	}
	defer sess.EndSession(context.Background())

	_, err = sess.WithTransaction(context.Background(), func(sessCtx context.Context) (any, error) {
		return nil, f.events.FindOne(sessCtx, bson.M{"_id": "transaction-probe"}).Err()
	})
	if err != nil && !isNoDocuments(err) {
		tb.Skipf("MongoDB transactions unavailable (%v) — start a replica set with: docker compose -f tests/integration/docker-compose.yml up -d --wait mongo", err)
	}
}

func isNoDocuments(err error) bool {
	return err != nil && err.Error() == mongo.ErrNoDocuments.Error()
}

// newOutbox builds an outbox over the fixture's store, wired to its recorder.
// Cycles are driven explicitly by the tests rather than by a scheduler: a
// scenario that asserts "nothing was delivered yet" cannot share the store with
// a background poller that might deliver it at any moment.
func (f *fixture) newOutbox(tb testing.TB, opts ...outbox.Option) *outbox.Outbox {
	tb.Helper()

	base := []outbox.Option{
		outbox.WithHandleTimeout(handleTimeout),
		outbox.WithMaxLockTime(lockTime),
		outbox.WithRetryBaseDelay(retryBaseDelay),
		outbox.WithRetryMaxDelay(time.Minute),
	}

	return outbox.New(f.store, f.recorder.Handler(), append(base, opts...)...)
}

// save persists events through a committed transaction, the way production code
// is required to. Returns the saved events, with the IDs the outbox assigned.
func (f *fixture) save(tb testing.TB, ob *outbox.Outbox, events ...outbox.Event) []outbox.Event {
	tb.Helper()

	saved := make([]outbox.Event, len(events))
	copy(saved, events)

	f.inTransaction(tb, func(sessCtx context.Context) error {
		return ob.Save(sessCtx, saved...)
	})

	for i := range saved {
		require.NotEmpty(tb, saved[i].Id, "Save must assign an ID")
	}

	return saved
}

// inTransaction runs fn inside a MongoDB transaction and requires it to commit.
func (f *fixture) inTransaction(tb testing.TB, fn func(sessCtx context.Context) error) {
	tb.Helper()

	sess, err := f.client.StartSession()
	require.NoError(tb, err)
	defer sess.EndSession(tb.Context())

	_, err = sess.WithTransaction(tb.Context(), func(sessCtx context.Context) (any, error) {
		return nil, fn(sessCtx)
	})
	require.NoError(tb, err)
}

// storedEvent is the persisted view of an event, decoded straight from the
// collection so the assertions read what the store actually wrote rather than
// what the outbox believed it wrote.
type storedEvent struct {
	ID            string     `bson:"_id"`
	Status        string     `bson:"status"`
	Attempts      uint32     `bson:"attempts"`
	Topic         string     `bson:"topic"`
	LastError     *string    `bson:"error"`
	CreatedAt     time.Time  `bson:"created_at"`
	PublishedAt   *time.Time `bson:"published_at"`
	LastAttemptOn *time.Time `bson:"last_attempt_on"`
	NextAttemptAt *time.Time `bson:"next_attempt_at"`
	LockedOn      *time.Time `bson:"locked_on"`
	LockToken     string     `bson:"lock_token"`
}

// load reads one event back from the collection.
func (f *fixture) load(tb testing.TB, id string) storedEvent {
	tb.Helper()

	var doc storedEvent
	require.NoError(tb, f.events.FindOne(tb.Context(), bson.M{"_id": id}).Decode(&doc))

	return doc
}

// loadAll reads every event back, oldest first.
func (f *fixture) loadAll(tb testing.TB) []storedEvent {
	tb.Helper()

	cursor, err := f.events.Find(tb.Context(), bson.M{},
		mongoOptions.Find().SetSort(bson.M{"created_at": 1}))
	require.NoError(tb, err)

	var docs []storedEvent
	require.NoError(tb, cursor.All(tb.Context(), &docs))

	return docs
}

// countEvents returns how many documents the outbox collection holds.
func (f *fixture) countEvents(tb testing.TB) int64 {
	tb.Helper()

	n, err := f.events.CountDocuments(tb.Context(), bson.M{})
	require.NoError(tb, err)

	return n
}

// countOrders returns how many business documents were committed.
func (f *fixture) countOrders(tb testing.TB) int64 {
	tb.Helper()

	n, err := f.orders.CountDocuments(tb.Context(), bson.M{})
	require.NoError(tb, err)

	return n
}

// event builds a well-formed outbox event with the given key and payload.
func event(key, payload string) outbox.Event {
	return outbox.Event{Key: key, Payload: []byte(payload)}
}
