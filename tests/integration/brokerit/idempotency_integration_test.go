// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package brokerit_test

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/idempotency"
	"github.com/altessa-s/go-atlas/data/idempotency/storages"
	"github.com/altessa-s/go-atlas/tests/integration/brokerit"
	"github.com/altessa-s/go-atlas/transport/broker"
	"github.com/altessa-s/go-atlas/transport/broker/msg"

	idemnats "github.com/altessa-s/go-atlas/data/idempotency/storages/nats"
	idemredis "github.com/altessa-s/go-atlas/data/idempotency/storages/redis"
	brokeridem "github.com/altessa-s/go-atlas/transport/broker/idempotency"
	natsprovider "github.com/altessa-s/go-atlas/transport/broker/providers/nats"
	goredis "github.com/redis/go-redis/v9"
)

// consumerScope is the idempotency scope every scenario wraps its handler in:
// the name of the service consuming the subject.
const consumerScope = "billing"

// redisAddr returns the Redis address, matching tests/integration/docker-compose.yml.
func redisAddr() string {
	if v := os.Getenv("REDIS_ADDR"); v != "" {
		return v
	}
	return "127.0.0.1:16379"
}

// newRedisStorage gives the test an idempotency storage under a key prefix of
// its own on the shared Redis, its keys deleted on cleanup. Two keepers over
// storages built from one call share their records, as two processes of one
// service would.
func newRedisStorage(tb testing.TB) *idemredis.Storage {
	tb.Helper()

	client := goredis.NewClient(&goredis.Options{Addr: redisAddr()})
	ctx, cancel := context.WithTimeout(tb.Context(), 3*time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		tb.Skipf("Redis unreachable at %s (%v) — start it with: make integration-up", redisAddr(), err)
	}

	prefix := uniqueName(tb) + ":"
	tb.Cleanup(func() {
		if keys, _ := client.Keys(context.Background(), prefix+"*").Result(); len(keys) > 0 {
			_ = client.Del(context.Background(), keys...).Err()
		}
		_ = client.Close()
	})

	return idemredis.New(client, idemredis.WithKeyPrefix(prefix))
}

// newKeeper builds a Keeper over storage. The explicit MaxLockDuration keeps
// an orphaned lock from being stolen within a test, so a key left IN_PROGRESS
// stays visible as such.
func newKeeper(storage storages.Storage) *idempotency.Keeper {
	return idempotency.New(storage, idempotency.WithMaxLockDuration(time.Minute))
}

// publishKeyed publishes one message per payload, each carrying key as its
// message ID — the metadata DefaultKeyFunc reads.
func (w *world) publishKeyed(tb testing.TB, key string, payloads ...string) {
	tb.Helper()

	b := broker.New(w.provider)
	for _, p := range payloads {
		m := msg.NewMessageWithMeta(w.subject, []byte(p), msg.Meta{{Key: msg.MetaKeyMessageId, Value: key}})
		require.NoError(tb, b.Publish(tb.Context(), *m))
	}
}

// requireKeyCompleted asserts the middleware recorded key as successfully
// processed, by asking a fresh Keeper for the lock.
func requireKeyCompleted(tb testing.TB, keeper *idempotency.Keeper, key string) {
	tb.Helper()

	acquired, state, err := keeper.AttemptLock(tb.Context(), brokeridem.BuildStorageKey(consumerScope, key))
	require.NoError(tb, err)
	require.False(tb, acquired, "key %s must still be held after processing", key)
	require.Equal(tb, idempotency.StatusSuccess, state.Status, "key %s must be recorded as processed", key)
}

// Three copies of one message — same message ID, different bodies, as a
// republishing producer would send them — reach the handler once. The two
// duplicates are acknowledged without running the handler, so nothing is left
// pending and nothing is redelivered.
func TestIdempotency_DuplicatesAreProcessedOnce(t *testing.T) {
	t.Parallel()

	w := newWorld(t)
	keeper := newKeeper(newRedisStorage(t))
	consumer := uniqueName(t)

	inner := brokerit.NewRecorder(w.subject, brokerit.Ack)
	subscribe(t, w.provider, natsprovider.SubscriberWithConsumer(w.durable(consumer, 10*time.Second)),
		brokeridem.Wrap(inner, keeper, consumerScope))

	w.publishKeyed(t, "invoice-1", "copy-1", "copy-2", "copy-3")
	w.publishKeyed(t, "invoice-2", "other")

	info := w.awaitSettled(t, consumer)
	require.Equal(t, uint64(4), info.Delivered.Consumer, "duplicates must be acknowledged, not redelivered")
	require.Equal(t, 2, inner.Count(), "one handler run per message ID")
	require.Equal(t, 1, inner.Attempts("copy-1"), "the first copy is the one processed")
	require.Equal(t, 1, inner.Attempts("other"))

	requireKeyCompleted(t, keeper, "invoice-1")
	requireKeyCompleted(t, keeper, "invoice-2")
}

// A handler that panics must not leave its key IN_PROGRESS: the middleware
// releases it, so when the server redelivers the message after the ack wait
// the handler runs again — on the first redelivery, not after a string of
// busy rejections — and the key ends up recorded as processed.
func TestIdempotency_PanicReleasesTheKeyForTheRedelivery(t *testing.T) {
	t.Parallel()

	w := newWorld(t)
	keeper := newKeeper(newRedisStorage(t))
	consumer := uniqueName(t)

	inner := brokerit.NewRecorder(w.subject, func(_ context.Context, m *msg.Message, attempt int) {
		if attempt == 1 {
			panic("downstream exploded")
		}
		_ = m.Ack()
	})
	subscribe(t, w.provider, natsprovider.SubscriberWithConsumer(w.durable(consumer, ackWait)),
		brokeridem.Wrap(inner, keeper, consumerScope))

	w.publishKeyed(t, "invoice-7", "charge")

	info := w.awaitSettled(t, consumer)
	require.Equal(t, 2, inner.Attempts("charge"), "the redelivery must reach the handler")
	require.Equal(t, uint64(2), info.Delivered.Consumer, "the released key must admit the first redelivery")
	requireKeyCompleted(t, keeper, "invoice-7")
}

// Two instances of one service share a durable consumer and an idempotency
// store, and every message ID is published several times, so copies of one ID
// regularly land on both instances at once. Whichever loses the race must back
// off rather than run the handler: across both instances each ID's side effect
// happens exactly once, and every copy is eventually acknowledged.
func TestIdempotency_ConcurrentDuplicatesAcrossTwoInstancesRunOnce(t *testing.T) {
	t.Parallel()

	const keys, copies = 40, 3

	w := newWorld(t)
	storage := newRedisStorage(t)
	consumer := uniqueName(t)
	cfg := w.durable(consumer, 10*time.Second, func(c *jetstream.ConsumerConfig) { c.MaxAckPending = 8 })

	var (
		mu      sync.Mutex
		effects = make(map[string]int)
	)
	sideEffect := func(_ context.Context, m *msg.Message, _ int) {
		time.Sleep(20 * time.Millisecond) // widens the window in which the other instance holds a copy
		mu.Lock()
		effects[msg.MessageIdFromMeta(m.Metadata)]++
		mu.Unlock()
		_ = m.Ack()
	}

	instances := make([]*brokerit.Recorder, 2)
	for i := range instances {
		instances[i] = brokerit.NewRecorder(w.subject, sideEffect)
		handler := brokeridem.Wrap(instances[i], newKeeper(storage), consumerScope,
			brokeridem.WithBusyBackOff(msg.ScheduleBackOff(100*time.Millisecond)))
		subscribe(t, newProvider(t), natsprovider.SubscriberWithConsumer(cfg, jetstream.PullMaxMessages(2)), handler)
	}

	// Copies of one ID are published back to back, so they sit next to each
	// other in the stream and are pulled by the two instances at the same time.
	for k := range keys {
		for c := range copies {
			w.publishKeyed(t, fmt.Sprintf("order-%02d", k), fmt.Sprintf("order-%02d/copy-%d", k, c))
		}
	}

	info := w.awaitSettled(t, consumer)
	require.Greater(t, info.Delivered.Consumer, uint64(keys*copies),
		"no copy ever found its ID in progress on the other instance, so the race this scenario is about never happened")
	t.Logf("%d copies took %d deliveries: %d busy rejections redelivered",
		keys*copies, info.Delivered.Consumer, info.Delivered.Consumer-keys*copies)

	mu.Lock()
	defer mu.Unlock()
	require.Len(t, effects, keys, "every message ID must be processed")
	for key, n := range effects {
		require.Equal(t, 1, n, "%s processed %d times", key, n)
	}
	require.Positive(t, instances[0].Count(), "the first instance never processed a message, so nothing raced")
	require.Positive(t, instances[1].Count(), "the second instance never processed a message, so nothing raced")
}

// The deduplication ID a producer sets — and the one the broker outbox sets on
// every event, the event ID — is the first key DefaultKeyFunc looks for. It
// must therefore reach the subscriber. The NATS provider maps it onto the
// Nats-Msg-Id header on publish and drops every Nats-* header on receive, so
// it never does: the middleware then sees no key and passes every redelivery
// through undeduplicated.
func TestIdempotency_DeduplicateIDReachesTheSubscriber(t *testing.T) {
	t.Parallel()

	w := newWorld(t)
	got := make(chan *msg.Message, 1)
	subscribe(t, w.provider, nil, brokerit.NewRecorder(w.subject, func(_ context.Context, m *msg.Message, _ int) {
		_ = m.Ack()
		got <- m
	}))

	sent := msg.NewMessageWithMeta(w.subject, []byte("paid"), msg.Meta{{Key: msg.MetaKeyDeduplicateId, Value: "invoice-9-paid"}})
	require.NoError(t, broker.New(w.provider).Publish(t.Context(), *sent))

	var m *msg.Message
	select {
	case m = <-got:
	case <-time.After(settleWindow):
		t.Fatal("the message never reached the subscriber")
	}

	key, err := brokeridem.DefaultKeyFunc(m)
	require.NoError(t, err)
	require.Equal(t, "invoice-9-paid", key,
		"the deduplication ID was lost on the wire; received metadata: %v", m.Metadata)
}

// The idempotency storage backed by NATS KV is the natural pairing for a
// NATS-backed broker. The middleware composes its storage keys as
// ("ibk", consumer, key) in the v2 format, which the Keeper wraps in its
// namespace — "idk:v2:9:namespace0:32:idk:v2:3:ibk7:billing9:invoice-1" — and
// NATS KV accepts only [-/_=.a-zA-Z0-9] in a key, so the storage must escape
// the ':' rather than fail every lock attempt and redeliver every message
// forever as a storage error.
func TestIdempotency_NATSKVStorageAcceptsTheMiddlewareKeys(t *testing.T) {
	t.Parallel()

	w := newWorld(t)
	bucket := uniqueName(t)
	storage, err := idemnats.New(w.provider.JetStream(), idemnats.WithBucket(bucket), idemnats.WithMaxAge(time.Hour))
	require.NoError(t, err)
	t.Cleanup(func() { _ = w.provider.JetStream().DeleteKeyValue(context.Background(), bucket) })

	keeper := newKeeper(storage)
	key := brokeridem.BuildStorageKey(consumerScope, "invoice-1")
	acquired, state, err := keeper.AttemptLock(t.Context(), key)
	require.NoError(t, err, "the NATS KV storage rejects the key the broker middleware builds")
	require.True(t, acquired)
	require.NoError(t, keeper.Complete(t.Context(), key, nil, state))
	requireKeyCompleted(t, keeper, "invoice-1")
}

// Two consumers of one subject share an idempotency store, scoped "a:b" and
// "a", and the subject carries messages with IDs "c" and "b:c". Under the
// former "ibk:{consumer}:{key}" layout ("a:b", "c") and ("a", "b:c") both
// became "ibk:a:b:c", so whichever consumer came second acknowledged its
// message as a duplicate without running its handler. Every consumer must
// process every message of its own.
func TestIdempotency_ConsumerScopesDoNotCollide(t *testing.T) {
	t.Parallel()

	w := newWorld(t)
	storage := newRedisStorage(t)
	scopes := []string{"a:b", "a"}

	recorders := make([]*brokerit.Recorder, len(scopes))
	consumers := make([]string, len(scopes))
	for i, scope := range scopes {
		consumers[i] = fmt.Sprintf("%s-%d", uniqueName(t), i)
		recorders[i] = brokerit.NewRecorder(w.subject, brokerit.Ack)
		subscribe(t, w.provider, natsprovider.SubscriberWithConsumer(w.durable(consumers[i], 10*time.Second)),
			brokeridem.Wrap(recorders[i], newKeeper(storage), scope))
	}

	w.publishKeyed(t, "c", "message-c")
	w.publishKeyed(t, "b:c", "message-b:c")

	for i, consumer := range consumers {
		w.awaitSettled(t, consumer)
		require.Equal(t, 1, recorders[i].Attempts("message-c"), "consumer %q must process message c", scopes[i])
		require.Equal(t, 1, recorders[i].Attempts("message-b:c"), "consumer %q must process message b:c", scopes[i])
	}
}
