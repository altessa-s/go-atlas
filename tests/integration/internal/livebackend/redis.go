// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package livebackend

import (
	"context"
	"crypto/rand"
	"os"
	"strings"
	"testing"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

// DefaultRedisAddr is the Redis address of tests/integration/docker-compose.yml.
const DefaultRedisAddr = "127.0.0.1:16379"

// maxNamePart caps the part of a generated name taken from the test name, so
// the name stays within backend limits (MongoDB database names, NATS bucket
// names) once the random suffix is appended.
const maxNamePart = 40

// pingTimeout bounds the reachability check of [ConnectRedis].
const pingTimeout = 3 * time.Second

// scanBatch is the SCAN COUNT hint of the cleanup in [RedisPrefix].
const scanBatch = 256

// RedisAddr returns the Redis address: REDIS_ADDR when set, otherwise
// [DefaultRedisAddr].
func RedisAddr() string {
	if addr := os.Getenv("REDIS_ADDR"); addr != "" {
		return addr
	}
	return DefaultRedisAddr
}

// ConnectRedis returns a client for [RedisAddr], closed on cleanup. It skips
// the test, rather than failing it, when the server does not answer a PING.
func ConnectRedis(tb testing.TB) *goredis.Client {
	tb.Helper()

	client := goredis.NewClient(&goredis.Options{Addr: RedisAddr()})
	ctx, cancel := context.WithTimeout(tb.Context(), pingTimeout)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		tb.Skipf("Redis unreachable at %s (%v) — start it with: make integration-up", RedisAddr(), err)
	}
	tb.Cleanup(func() { _ = client.Close() })

	return client
}

// UniqueName returns prefix, the test name reduced to [a-z0-9_] and capped,
// and a random suffix, joined by underscores. Two tests — or two runs of one
// test against the same server — never get the same name.
func UniqueName(tb testing.TB, prefix string) string {
	tb.Helper()

	safe := strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			return r
		}
		return '_'
	}, strings.ToLower(tb.Name()))

	return prefix + "_" + safe[:min(len(safe), maxNamePart)] + "_" + strings.ToLower(rand.Text()[:8])
}

// RedisPrefix returns a key prefix unique to the test, ending in ":", and
// deletes every key under it on cleanup.
func RedisPrefix(tb testing.TB, client *goredis.Client, prefix string) string {
	tb.Helper()

	p := UniqueName(tb, prefix) + ":"
	tb.Cleanup(func() {
		ctx := context.Background()
		iter := client.Scan(ctx, 0, p+"*", scanBatch).Iterator()
		var keys []string
		for iter.Next(ctx) {
			keys = append(keys, iter.Val())
		}
		if len(keys) > 0 {
			_ = client.Del(ctx, keys...).Err()
		}
	})

	return p
}
