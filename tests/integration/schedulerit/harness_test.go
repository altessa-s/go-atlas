// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package schedulerit_test

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/altessa-s/go-atlas/service/scheduler"

	corescheduler "github.com/altessa-s/go-atlas/core/scheduler"
	mongostore "github.com/altessa-s/go-atlas/service/scheduler/storages/mongodb"
	redisstore "github.com/altessa-s/go-atlas/service/scheduler/storages/redis"
	goredis "github.com/redis/go-redis/v9"
	mongoOptions "go.mongodb.org/mongo-driver/v2/mongo/options"
)

const (
	// tickInterval drives dispatch. Short enough that a test does not spend its
	// budget waiting for the next tick, long enough not to hammer the backends.
	tickInterval = 50 * time.Millisecond

	// settleWindow is how long an assertion allows a condition to become true.
	// Generous: these tests cross a real network and RediSearch indexes writes
	// asynchronously, so the interesting failures are "never happens", not
	// "happens a bit late".
	settleWindow = 15 * time.Second

	// quietWindow is how long a negative assertion watches for something that
	// must not happen. Long enough to span many ticks.
	quietWindow = 2 * time.Second

	// samplingInterval is the poll cadence for both.
	samplingInterval = 25 * time.Millisecond

	// stopTimeout bounds a graceful shutdown in cleanup.
	stopTimeout = 10 * time.Second
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

// redisAddr returns the Redis address, matching tests/integration/docker-compose.yml.
func redisAddr() string {
	if addr := os.Getenv("REDIS_ADDR"); addr != "" {
		return addr
	}
	return "127.0.0.1:16379"
}

// namespaceSequence disambiguates database names and key prefixes. A timestamp
// alone is not enough: these tests are parallel, so several fixtures are built
// within the same clock tick, and two landing on one namespace would have each
// test's scheduler dispatching the other's tasks.
var namespaceSequence atomic.Int64

// namespace derives a per-test namespace. The test name makes a failure
// traceable to its data; the sequence number guarantees uniqueness.
func namespace(tb testing.TB) string {
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

	return fmt.Sprintf("schedit_%s_%d", safe, namespaceSequence.Add(1))
}

// backend names one storage implementation and how to build an isolated
// instance of it. Every scenario runs against all of them: the whole point of
// this package is that the backends are where the behavior differs.
type backend struct {
	name    string
	storage func(tb testing.TB) scheduler.Storage
}

func backends() []backend {
	return []backend{
		{name: "mongodb", storage: newMongoStorage},
		{name: "redis", storage: newRedisStorage},
	}
}

// newMongoStorage gives the test a throwaway database, dropped on cleanup.
func newMongoStorage(tb testing.TB) scheduler.Storage {
	tb.Helper()

	client, err := mongo.Connect(mongoOptions.Client().
		ApplyURI(mongoURI()).
		SetServerSelectionTimeout(3 * time.Second))
	if err != nil {
		tb.Skipf("MongoDB unreachable at %s (%v) — start it with: make integration-up", mongoURI(), err)
	}

	ctx := context.Background()
	if err = client.Ping(ctx, nil); err != nil {
		_ = client.Disconnect(ctx)
		tb.Skipf("MongoDB unreachable at %s (%v) — start it with: make integration-up", mongoURI(), err)
	}

	db := client.Database(namespace(tb))
	tb.Cleanup(func() {
		_ = db.Drop(context.Background())
		_ = client.Disconnect(context.Background())
	})

	s := mongostore.New(db)
	require.NoError(tb, s.EnsureIndexes(ctx))

	return s
}

// newRedisStorage gives the test its own key prefix and RediSearch indexes,
// both dropped on cleanup. It skips when the server lacks the RedisJSON and
// RediSearch modules the storage is built on — a bare Redis cannot run this.
func newRedisStorage(tb testing.TB) scheduler.Storage {
	tb.Helper()

	ctx := context.Background()
	client := goredis.NewClient(&goredis.Options{Addr: redisAddr()})
	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		tb.Skipf("Redis unreachable at %s (%v) — start it with: make integration-up", redisAddr(), err)
	}

	prefix := namespace(tb)
	s := redisstore.New(client, redisstore.WithKeyPrefix(prefix))
	if err := s.EnsureIndexes(ctx); err != nil {
		_ = client.Close()
		tb.Skipf("Redis lacks the RedisJSON/RediSearch modules (%v) — the storage needs Redis Stack", err)
	}

	tb.Cleanup(func() {
		cleanup := context.Background()
		_ = client.FTDropIndex(cleanup, prefix+":idx:tasks").Err()
		_ = client.FTDropIndex(cleanup, prefix+":idx:history").Err()
		if keys, _ := client.Keys(cleanup, prefix+":*").Result(); len(keys) > 0 {
			_ = client.Del(cleanup, keys...)
		}
		_ = client.Close()
	})

	return s
}

// startScheduler builds a scheduler over the given storage, starts it, and
// stops it on cleanup.
func startScheduler(tb testing.TB, storage scheduler.Storage, opts ...scheduler.Option) *scheduler.Scheduler {
	tb.Helper()

	base := []scheduler.Option{scheduler.WithTickInterval(tickInterval)}
	s := scheduler.New(storage, append(base, opts...)...)

	require.NoError(tb, s.Start(tb.Context()))
	tb.Cleanup(func() {
		// tb.Context() is already canceled when cleanups run; detach so the
		// scheduler gets its full grace period to drain.
		stopCtx, cancel := context.WithTimeout(context.WithoutCancel(tb.Context()), stopTimeout)
		defer cancel()
		_ = s.Stop(stopCtx)
	})

	return s
}

// countingTask returns a task config whose function increments runs, plus the
// counter. RunOnStart makes the first occurrence due immediately; the hourly
// schedule keeps a second one from arriving mid-test.
func countingTask(id string, runs *atomic.Int32) corescheduler.TaskConfig {
	return corescheduler.TaskConfig{
		ID:         id,
		Schedule:   "@every 1h",
		RunOnStart: true,
		Func: func(context.Context) error {
			runs.Add(1)
			return nil
		},
	}
}

// stubElector is a LeaderElector whose verdict the test controls.
type stubElector struct{ leader atomic.Bool }

func (s *stubElector) IsLeader() bool { return s.leader.Load() }
