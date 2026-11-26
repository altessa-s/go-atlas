// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package limiterit_test

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/altessa-s/go-atlas/coord/limiters/storages"
	"github.com/altessa-s/go-atlas/coord/limiters/storages/memory"

	limiters "github.com/altessa-s/go-atlas/coord/limiters"
	redislimiter "github.com/altessa-s/go-atlas/coord/limiters/storages/redis"
	goredis "github.com/redis/go-redis/v9"
)

// envOr returns the environment override for a connection setting, or the
// default that matches tests/integration/docker-compose.yml.
func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// namespace derives a name unique to the test and the run: the sanitized test
// name makes leftovers traceable, the random suffix keeps parallel tests and
// concurrent runs against the shared server apart.
func namespace(tb testing.TB) string {
	tb.Helper()

	safe := strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' {
			return r
		}
		return '_'
	}, tb.Name())
	const maxNameLen = 40
	if len(safe) > maxNameLen {
		safe = safe[len(safe)-maxNameLen:]
	}

	var b [6]byte
	_, _ = rand.Read(b[:])
	return "limiterit_" + safe + "_" + hex.EncodeToString(b[:])
}

// replicaFunc returns a provider instance over the namespace of one test. For
// a shared backend every call returns a new instance on its own connection —
// a separate service replica as far as the backend can tell.
type replicaFunc func() storages.Storage

// backend is one storage implementation and how to open an isolated
// namespace on it.
type backend struct {
	name string
	// shared is set when instances returned by open share their state through
	// the server; the in-memory provider is per process.
	shared bool
	open   func(tb testing.TB) replicaFunc
}

func backends() []backend {
	return []backend{
		{name: "memory", open: openMemory},
		{name: "redis", shared: true, open: openRedis},
	}
}

// openMemory returns the one in-process provider: replicas of a process-local
// limiter are the same instance.
func openMemory(testing.TB) replicaFunc {
	p := memory.New()
	return func() storages.Storage { return p }
}

// redisClient dials the compose Redis, skipping when it is unreachable.
func redisClient(tb testing.TB) *goredis.Client {
	tb.Helper()

	client := goredis.NewClient(&goredis.Options{Addr: envOr("REDIS_ADDR", "127.0.0.1:16379")})
	ctx, cancel := context.WithTimeout(tb.Context(), 3*time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		tb.Skipf("Redis unreachable (%v) — start it with: make integration-up", err)
	}
	tb.Cleanup(func() { _ = client.Close() })
	return client
}

// openRedis gives the test a key prefix of its own; every replica dials its
// own client. The prefix's keys are deleted on cleanup.
func openRedis(tb testing.TB) replicaFunc {
	tb.Helper()

	admin := redisClient(tb)
	prefix := namespace(tb)
	tb.Cleanup(func() { deleteRedisKeys(admin, prefix+"*") })

	return func() storages.Storage {
		return redislimiter.New(redisClient(tb), redislimiter.WithKeyPrefix(prefix))
	}
}

// deleteRedisKeys removes every key matching pattern. It runs on cleanup,
// after the test context is gone.
func deleteRedisKeys(client *goredis.Client, pattern string) {
	ctx := context.Background()
	iter := client.Scan(ctx, 0, pattern, 500).Iterator()
	for iter.Next(ctx) {
		_ = client.Del(ctx, iter.Val()).Err()
	}
}

// tally counts the outcomes of Allow calls made from many goroutines.
type tally struct {
	admitted atomic.Int64
	rejected atomic.Int64

	mu     sync.Mutex
	failed []error
}

func (c *tally) record(err error) {
	switch {
	case err == nil:
		c.admitted.Add(1)
	case errors.Is(err, limiters.ErrLimitExceeded):
		c.rejected.Add(1)
	default:
		c.mu.Lock()
		c.failed = append(c.failed, err)
		c.mu.Unlock()
	}
}

func (c *tally) failures() []error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.failed
}

// hammer runs workers goroutines, each making attempts calls of call.
func hammer(workers, attempts int, call func(worker int) error) *tally {
	var (
		res   tally
		wg    sync.WaitGroup
		start = make(chan struct{})
	)
	for w := range workers {
		wg.Go(func() {
			<-start
			for range attempts {
				res.record(call(w))
			}
		})
	}
	close(start)
	wg.Wait()
	return &res
}

// sleepUntil blocks until the wall clock reaches t.
func sleepUntil(t time.Time) {
	time.Sleep(time.Until(t))
}

// resetBounds returns the range of Reset values a window opened by a request
// made between before and after can report: the providers compute
// floor((request ms + period ms) / 1000) from the local clock.
func resetBounds(before, after time.Time, period time.Duration) (lo, hi int64) {
	return (before.UnixMilli() + period.Milliseconds()) / 1000, (after.UnixMilli() + period.Milliseconds()) / 1000
}
