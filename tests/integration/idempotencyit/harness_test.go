// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package idempotencyit_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/altessa-s/go-atlas/data/idempotency/storages"
	"github.com/altessa-s/go-atlas/tests/integration/internal/livebackend"

	idemmongo "github.com/altessa-s/go-atlas/data/idempotency/storages/mongo"
	idemnats "github.com/altessa-s/go-atlas/data/idempotency/storages/nats"
	idemredis "github.com/altessa-s/go-atlas/data/idempotency/storages/redis"
	mongoOptions "go.mongodb.org/mongo-driver/v2/mongo/options"
)

const (
	// longTTL is the storage TTL of a test that is not about expiry: far
	// beyond the test's run time, so nothing ages out under it.
	longTTL = time.Hour

	// shortTTL is the lock or storage TTL of a test that waits for expiry.
	// One second is the smallest per-key TTL NATS accepts.
	shortTTL = time.Second

	// settleWindow bounds the wait for a key to age out. Generous: Redis
	// expires lazily or on its active cycle and NATS on its own timer, so the
	// interesting failure is "never expires", not "expires a little late".
	settleWindow = 15 * time.Second

	// samplingInterval is the poll cadence while waiting.
	samplingInterval = 50 * time.Millisecond

	// contenders is how many goroutines race for one key.
	contenders = 32
)

// backend opens a fresh, isolated storage on one live server.
type backend struct {
	name string
	// open returns a storage over its own key prefix, database or bucket,
	// removed on cleanup, whose configured TTL is ttl.
	open func(tb testing.TB, ttl time.Duration) storages.Storage
}

// backends lists every networked idempotency storage.
func backends() []backend {
	return []backend{
		{name: "redis", open: openRedis},
		{name: "mongo", open: openMongo},
		{name: "nats", open: openNATS},
	}
}

// forEachBackend runs fn as a parallel subtest per backend.
func forEachBackend(t *testing.T, fn func(t *testing.T, b backend)) {
	t.Helper()
	for _, b := range backends() {
		t.Run(b.name, func(t *testing.T) {
			t.Parallel()
			fn(t, b)
		})
	}
}

// openRedis gives the test a key prefix of its own on the shared server.
func openRedis(tb testing.TB, ttl time.Duration) storages.Storage {
	tb.Helper()
	client := livebackend.ConnectRedis(tb)
	prefix := livebackend.RedisPrefix(tb, client, "idemit")
	return idemredis.New(client, idemredis.WithKeyPrefix(prefix), idemredis.WithTTL(ttl))
}

// openMongo gives the test a throwaway database, dropped on cleanup. It runs
// only when MONGO_URI names the server: the compose port is a common choice
// for other projects' MongoDB too, and a default would create and drop
// databases on whatever answers there. directConnection=true is required for
// the compose replica set, which advertises its in-container address.
func openMongo(tb testing.TB, ttl time.Duration) storages.Storage {
	tb.Helper()
	uri := os.Getenv("MONGO_URI")
	if uri == "" {
		tb.Skip("MONGO_URI not set — export it to run the MongoDB backend")
	}
	client, err := mongo.Connect(mongoOptions.Client().ApplyURI(uri).SetServerSelectionTimeout(3 * time.Second))
	if err != nil {
		tb.Skipf("MongoDB unreachable (%v) — start it with: make integration-up", err)
	}
	if err := client.Ping(tb.Context(), nil); err != nil {
		_ = client.Disconnect(context.Background())
		tb.Skipf("MongoDB unreachable (%v) — start it with: make integration-up", err)
	}
	db := client.Database(livebackend.UniqueName(tb, "idemit"))
	tb.Cleanup(func() {
		_ = db.Drop(context.Background())
		_ = client.Disconnect(context.Background())
	})
	storage, err := idemmongo.New(db, idemmongo.WithTTL(ttl))
	require.NoError(tb, err)
	return storage
}

// natsURL returns the broker address, matching tests/integration/docker-compose.yml.
func natsURL() string {
	if v := os.Getenv("NATS_URL"); v != "" {
		return v
	}
	return "nats://127.0.0.1:14222"
}

// openNATS gives the test a bucket of its own, deleted on cleanup. The bucket
// TTL is ttl; per-key TTLs need NATS 2.11 or newer.
func openNATS(tb testing.TB, ttl time.Duration) storages.Storage {
	tb.Helper()
	nc, err := nats.Connect(natsURL(), nats.Timeout(2*time.Second), nats.RetryOnFailedConnect(false))
	if err != nil {
		tb.Skipf("NATS unreachable at %s (%v) — start it with: make integration-up", natsURL(), err)
	}
	tb.Cleanup(nc.Close)
	js, err := jetstream.New(nc)
	require.NoError(tb, err)

	bucket := livebackend.UniqueName(tb, "idemit")
	tb.Cleanup(func() { _ = js.DeleteKeyValue(context.Background(), bucket) })
	storage, err := idemnats.New(js, idemnats.WithBucket(bucket), idemnats.WithMaxAge(ttl))
	require.NoError(tb, err)
	return storage
}

// payload returns a value unique to tag that is not valid UTF-8 and embeds
// NUL bytes, so a backend comparing values as C strings or as text would
// confuse two distinct holders.
func payload(tag string) []byte {
	return append([]byte{0x00, 0xff, 0xfe}, append([]byte(tag), 0x00, 0x80)...)
}

// observe returns the value key holds, as a losing AttemptLock reports it.
// It fails the test when the attempt acquires the key, which means key held
// nothing.
func observe(tb testing.TB, s storages.Storage, key string) []byte {
	tb.Helper()
	acquired, existing, _, err := s.AttemptLock(tb.Context(), key, payload("probe"))
	require.NoError(tb, err)
	require.False(tb, acquired, "key %q holds nothing", key)
	return existing
}

// requireFree asserts that key holds nothing: a fresh AttemptLock acquires it.
// The probe lock is released again.
func requireFree(tb testing.TB, s storages.Storage, key string) {
	tb.Helper()
	acquired, _, token, err := s.AttemptLock(tb.Context(), key, payload("free-probe"))
	require.NoError(tb, err)
	require.True(tb, acquired, "key %q is still held", key)
	require.NoError(tb, s.Release(tb.Context(), key, token))
}

// lock acquires key with val and returns the lock token.
func lock(tb testing.TB, s storages.Storage, key string, val []byte) []byte {
	tb.Helper()
	acquired, existing, token, err := s.AttemptLock(tb.Context(), key, val)
	require.NoError(tb, err)
	require.True(tb, acquired, "key %q already held: %q", key, existing)
	require.NotEmpty(tb, token, "an acquired lock must carry a token")
	return token
}

// awaitAcquire polls AttemptLock with val until it acquires key, which
// happens once the server has aged the previous holder out, and returns the
// new lock token.
func awaitAcquire(tb testing.TB, s storages.Storage, key string, val []byte) []byte {
	tb.Helper()
	var (
		token   []byte
		lastErr error
	)
	// The condition runs off the test goroutine, so it records errors rather
	// than failing from there.
	ok := assert.Eventually(tb, func() bool {
		acquired, _, tok, err := s.AttemptLock(tb.Context(), key, val)
		lastErr, token = err, tok
		return err == nil && acquired
	}, settleWindow, samplingInterval)
	require.NoError(tb, lastErr)
	require.True(tb, ok, "key %q never expired", key)
	return token
}
