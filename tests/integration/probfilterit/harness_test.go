// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package probfilterit_test

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"iter"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/probfilter"
	"github.com/altessa-s/go-atlas/data/probfilter/bloom"
	"github.com/altessa-s/go-atlas/data/probfilter/cuckoo"

	bloomredis "github.com/altessa-s/go-atlas/data/probfilter/bloom/storages/redis"
	cuckooredis "github.com/altessa-s/go-atlas/data/probfilter/cuckoo/storages/redis"
	goredis "github.com/redis/go-redis/v9"
)

// filterName is the name every test gives its filter; the per-test key
// prefix is what keeps tests apart.
const filterName = "filter"

// envOr returns the environment override for a connection setting, or the
// default that matches tests/integration/docker-compose.yml.
func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// redisClient dials the compose Redis Stack, skipping when it is unreachable
// or has no RedisBloom module.
func redisClient(tb testing.TB) *goredis.Client {
	tb.Helper()

	client := goredis.NewClient(&goredis.Options{Addr: envOr("REDIS_ADDR", "127.0.0.1:16379")})
	ctx, cancel := context.WithTimeout(tb.Context(), 3*time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		tb.Skipf("Redis unreachable (%v) — start it with: make integration-up", err)
	}
	if err := client.Do(ctx, "BF.EXISTS", "probfilterit:module-probe", "x").Err(); err != nil &&
		strings.Contains(strings.ToLower(err.Error()), "unknown command") {
		_ = client.Close()
		tb.Skipf("Redis at %s has no RedisBloom module (%v) — the compose stack runs Redis Stack", client.Options().Addr, err)
	}
	tb.Cleanup(func() { _ = client.Close() })
	return client
}

// fixture is one test's namespace on the shared server: a key prefix under
// which every instance the test opens shares one filter.
type fixture struct {
	admin  *goredis.Client
	prefix string
}

// newFixture derives a key prefix unique to the test and the run, and deletes
// the filter and its metadata keys on cleanup.
func newFixture(tb testing.TB) *fixture {
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

	f := &fixture{admin: redisClient(tb), prefix: "probfilterit:" + safe + "_" + hex.EncodeToString(b[:]) + ":"}
	tb.Cleanup(func() {
		ctx := context.Background()
		for _, pattern := range []string{f.prefix + "*", "__probfilter__:{" + f.prefix + "*"} {
			iter := f.admin.Scan(ctx, 0, pattern, 500).Iterator()
			for iter.Next(ctx) {
				_ = f.admin.Del(ctx, iter.Val()).Err()
			}
		}
	})
	return f
}

// liveKey is the filter key the storages build: prefix plus filter name.
func (f *fixture) liveKey() string {
	return f.prefix + filterName
}

// metaKey mirrors the layout of the "__probfilter__:" metadata keys
// (data/probfilter/internal/redisfilter/keys.go) for a live key without a
// hash tag: the live key itself is the tag. Scenarios use it to stand in for
// the server — expiring a rebuild lease — and to check what a rebuild leaves
// behind.
func (f *fixture) metaKey(kind string) string {
	live := f.liveKey()
	return "__probfilter__:{" + live + "}:" + kind + ":" + hex.EncodeToString([]byte(live))
}

// keys lists the keys matching pattern.
func (f *fixture) keys(tb testing.TB, pattern string) []string {
	tb.Helper()
	var keys []string
	iter := f.admin.Scan(tb.Context(), 0, pattern, 500).Iterator()
	for iter.Next(tb.Context()) {
		keys = append(keys, iter.Val())
	}
	require.NoError(tb, iter.Err())
	return keys
}

// requireNoRebuildLeftovers asserts that a finished rebuild — committed or
// not — left no staging key, no rebuild lease and no journal behind.
func (f *fixture) requireNoRebuildLeftovers(tb testing.TB) {
	tb.Helper()
	require.Empty(tb, f.keys(tb, f.metaKey("staging")+"*"), "a staging key outlived its rebuild")
	for _, kind := range []string{"rebuild-lease", "rebuild-journal"} {
		n, err := f.admin.Exists(tb.Context(), f.metaKey(kind)).Result()
		require.NoError(tb, err)
		require.Zero(tb, n, "the %s key outlived its rebuild", kind)
	}
}

// bloom opens a Bloom facade on its own connection over the fixture's filter.
func (f *fixture) bloom(tb testing.TB, opts ...bloomredis.Option) *bloom.Filter {
	tb.Helper()
	opts = append([]bloomredis.Option{bloomredis.WithKeyPrefix(f.prefix)}, opts...)
	filter := bloom.New(bloomredis.New(redisClient(tb), filterName, opts...))
	tb.Cleanup(func() { _ = filter.Close(context.Background()) })
	return filter
}

// cuckoo opens a Cuckoo facade on its own connection over the fixture's filter.
func (f *fixture) cuckoo(tb testing.TB, opts ...cuckooredis.Option) *cuckoo.Filter {
	tb.Helper()
	opts = append([]cuckooredis.Option{cuckooredis.WithKeyPrefix(f.prefix)}, opts...)
	filter := cuckoo.New(cuckooredis.New(redisClient(tb), filterName, opts...))
	tb.Cleanup(func() { _ = filter.Close(context.Background()) })
	return filter
}

// values returns n distinct values with the given label. They are the same on
// every run, so a false positive among them is reproducible rather than flaky.
func values(label string, n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = fmt.Sprintf("%s-%06d", label, i)
	}
	return out
}

// requirePresent asserts that the filter reports every value — a filter may
// answer a false positive, never a false negative.
func requirePresent(tb testing.TB, f probfilter.Filter, vals ...string) {
	tb.Helper()
	for _, v := range vals {
		ok, err := f.MightExist(tb.Context(), v)
		require.NoError(tb, err)
		require.True(tb, ok, "false negative for %q", v)
	}
}

// requireAbsent asserts that the filter reports none of the values. Callers
// pass values a correct filter cannot hold apart from a false positive and
// keep the set small and fixed, so the answer is deterministic.
func requireAbsent(tb testing.TB, f probfilter.Filter, vals ...string) {
	tb.Helper()
	for _, v := range vals {
		ok, err := f.MightExist(tb.Context(), v)
		require.NoError(tb, err)
		require.False(tb, ok, "%q is reported present", v)
	}
}

// falsePositiveRate probes values never added and returns the share reported
// present.
func falsePositiveRate(tb testing.TB, f probfilter.Filter, probes []string) float64 {
	tb.Helper()
	hits := 0
	for _, v := range probes {
		ok, err := f.MightExist(tb.Context(), v)
		require.NoError(tb, err)
		if ok {
			hits++
		}
	}
	return float64(hits) / float64(len(probes))
}

// gatedLoader yields its values and then holds the rebuild open — its rebuild
// lease taken, its snapshot read — until released, so a scenario can act
// while the rebuild runs.
type gatedLoader struct {
	vals []string
	// counted makes Count report the number of values, so the rebuild stages
	// a sized filter and streams into it; otherwise values are collected first.
	counted bool
	// failAfter, when positive, makes the stream fail after that many values.
	failAfter int

	loadedOnce sync.Once
	loaded     chan struct{}
	release    chan struct{}
}

func newGatedLoader(vals []string, counted bool) *gatedLoader {
	return &gatedLoader{vals: vals, counted: counted, loaded: make(chan struct{}), release: make(chan struct{})}
}

func (l *gatedLoader) StreamValues(ctx context.Context) iter.Seq2[string, error] {
	return func(yield func(string, error) bool) {
		for i, v := range l.vals {
			if l.failAfter > 0 && i == l.failAfter {
				yield("", errSourceFailed)
				return
			}
			if !yield(v, nil) {
				return
			}
		}
		l.loadedOnce.Do(func() { close(l.loaded) })
		select {
		case <-l.release:
		case <-ctx.Done():
		}
	}
}

func (l *gatedLoader) Count(context.Context) (int64, error) {
	if l.counted {
		return int64(len(l.vals)), nil
	}
	return -1, nil
}

// errSourceFailed is the failure a gatedLoader injects into its stream.
var errSourceFailed = errors.New("source of truth unavailable")

// openLoader is a gatedLoader that never holds the rebuild.
func openLoader(vals []string, counted bool) *gatedLoader {
	l := newGatedLoader(vals, counted)
	close(l.release)
	return l
}

// rebuildInBackground starts f.Rebuild with loader and waits until the loader
// has yielded its snapshot, so the rebuild holds its lease. The returned
// channel delivers the rebuild's result once the loader is released.
func rebuildInBackground(tb testing.TB, f probfilter.RebuildableFilter, loader *gatedLoader) <-chan error {
	tb.Helper()
	done := make(chan error, 1)
	go func() { done <- f.Rebuild(context.WithoutCancel(tb.Context()), loader) }()
	select {
	case <-loader.loaded:
	case err := <-done:
		tb.Fatalf("rebuild returned before reading its snapshot: %v", err)
	case <-time.After(10 * time.Second):
		tb.Fatal("rebuild never read its snapshot")
	}
	return done
}

// awaitRebuild releases loader and returns the rebuild's result.
func awaitRebuild(tb testing.TB, loader *gatedLoader, done <-chan error) error {
	tb.Helper()
	close(loader.release)
	select {
	case err := <-done:
		return err
	case <-time.After(30 * time.Second):
		tb.Fatal("rebuild never finished")
		return nil
	}
}
