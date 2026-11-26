// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package redis

import (
	"context"
	"iter"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/altessa-s/go-atlas/cache/probfilter/bloom/storages"
	"github.com/altessa-s/go-atlas/cache/probfilter/internal/redisfilter"
	"github.com/altessa-s/go-atlas/cache/probfilter/stats"
	"github.com/altessa-s/go-atlas/internal/redis/redisbase"
)

// Storage is a Redis-backed Bloom filter storage using RedisBloom BF.* commands.
// It is safe for concurrent use.
type Storage struct {
	redisbase.Base
	core        *redisfilter.Core
	opts        *options
	mu          sync.RWMutex
	lastRebuild time.Time
}

var (
	_ storages.Storage               = (*Storage)(nil)
	_ storages.ExclusiveRebuilder    = (*Storage)(nil)
	_ storages.RebuildCommitReporter = (*Storage)(nil)

	_ storages.RebuildCommitTimeReporter = (*Storage)(nil)
	_ storages.RebuildLoadReporter       = (*Storage)(nil)
	_ storages.RebuildCommitAgeReporter  = (*Storage)(nil)
)

// New creates a new Redis Bloom filter Storage.
// Requires Redis with RedisBloom module installed.
//
// Example:
//
//	storage := redis.New(rdb, "myfilter",
//	    redis.WithExpectedItems(100000),
//	    redis.WithFalsePositiveRate(0.01),
//	)
func New(client redis.UniversalClient, filterName string, opt ...Option) *Storage {
	opts := newOptions(opt...)

	core := redisfilter.New(client, opts.keyPrefix+filterName, redisfilter.Commands{
		Label:  "Bloom",
		Exists: "BF.EXISTS",
		// Inserts never create a missing filter: a live filter is recreated
		// by the reserve path, which also clears the rebuild ready marker,
		// and a vanished staging key fails the rebuild.
		Add:         "BF.INSERT",
		AddTokens:   []string{"NOCREATE", "ITEMS"},
		AddBatch:    "BF.INSERT",
		BatchTokens: []string{"NOCREATE", "ITEMS"},
		Reserve:     "BF.RESERVE",
		Info:        "BF.INFO",
		// Re-inserting into a Bloom filter is idempotent, so inserts by
		// every process sharing the filter are journaled and replayed
		// onto a rebuild's replacement.
		JournalAdds: true,
	}, opts.falsePositiveRate, opts.expectedItems)
	if opts.insecureTreatMissingAsEmpty {
		core.TreatMissingAsEmpty()
	}
	return &Storage{
		Base: redisbase.NewBase(client, opts.keyPrefix),
		core: core,
		opts: opts,
	}
}

// MightExist checks if a value might exist in the filter.
func (s *Storage) MightExist(ctx context.Context, value string) (bool, error) {
	return s.core.MightExist(ctx, value)
}

// Add inserts a value into the filter.
func (s *Storage) Add(ctx context.Context, value string) error {
	return s.core.Add(ctx, value)
}

// AddBatch inserts multiple values into the filter.
func (s *Storage) AddBatch(ctx context.Context, values iter.Seq[string]) error {
	return s.core.AddBatch(ctx, values)
}

// Stage reserves an empty replacement filter sized for expectedItems (the
// configured size when expectedItems <= 0) under a private staging key in the
// live key's cluster hash slot. Committing renames it onto the live key in one
// atomic step; until then the live filter is untouched.
func (s *Storage) Stage(ctx context.Context, expectedItems int64) (storages.Staging, error) {
	if expectedItems <= 0 {
		expectedItems = s.opts.expectedItems
	}

	st, err := s.core.Stage(ctx, s.opts.falsePositiveRate, expectedItems)
	if err != nil {
		return nil, err
	}
	return st, nil
}

// BeginRebuild acquires the filter's rebuild lease (see
// [storages.ExclusiveRebuilder]), so concurrent rebuilds of the shared filter
// by several processes are serialized and a stale snapshot cannot overwrite a
// newer one.
func (s *Storage) BeginRebuild(ctx context.Context) (storages.RebuildLease, error) {
	lease, err := s.core.BeginRebuild(ctx)
	if err != nil {
		return nil, err
	}
	return &rebuildLease{storage: s, lease: lease}, nil
}

// RebuildCommitted reports whether any process committed a rebuild of the
// shared filter and the filter key has not been deleted or recreated since
// (see [storages.RebuildCommitReporter]). Rebuilds committed by releases
// that predate this check, and a filter key recreated by a client other than
// probfilter, are not recognized; delete the filter key together with its
// "__probfilter__:" metadata keys.
func (s *Storage) RebuildCommitted(ctx context.Context) (bool, error) {
	return s.core.RebuildCommitted(ctx)
}

// LastRebuildCommit returns the Redis server time at which the last rebuild
// of the shared filter committed, whichever process ran it (see
// [storages.RebuildCommitTimeReporter]). It is read from Redis on every call
// and is the zero time when [Storage.RebuildCommitted] would report false.
// [Storage.LastRebuild] stays process-local.
func (s *Storage) LastRebuildCommit(ctx context.Context) (time.Time, error) {
	return s.core.LastRebuildCommit(ctx)
}

// RebuildCommitAge returns how long ago the last rebuild of the shared filter
// committed, and its commit time, both on the Redis server clock and read in
// one script (see [storages.RebuildCommitAgeReporter]): the age does not
// depend on the skew between this process's clock and the server's.
func (s *Storage) RebuildCommitAge(ctx context.Context) (age time.Duration, committed time.Time, known bool, err error) {
	return s.core.RebuildCommitAge(ctx)
}

// LastRebuildLoaded returns how many values the last rebuild of the shared
// filter loaded from its data source, whichever process ran it (see
// [storages.RebuildLoadReporter]). The count is stored with the commit and
// read from Redis on every call; known is false when
// [Storage.RebuildCommitted] would report false or the rebuild did not record
// the count.
func (s *Storage) LastRebuildLoaded(ctx context.Context) (loaded int64, known bool, err error) {
	return s.core.LastRebuildLoaded(ctx)
}

// rebuildLease stages replacement filters under a held rebuild lease.
type rebuildLease struct {
	storage *Storage
	lease   *redisfilter.Lease
}

func (l *rebuildLease) Stage(ctx context.Context, expectedItems int64) (storages.Staging, error) {
	if expectedItems <= 0 {
		expectedItems = l.storage.opts.expectedItems
	}
	st, err := l.lease.Stage(ctx, l.storage.opts.falsePositiveRate, expectedItems)
	if err != nil {
		return nil, err
	}
	return st, nil
}

func (l *rebuildLease) Release(ctx context.Context) error {
	return l.lease.Release(ctx)
}

// Stats returns current filter statistics.
func (s *Storage) Stats(ctx context.Context) (*stats.FilterStats, error) {
	result, found, err := s.core.Info(ctx)
	if err != nil {
		return nil, err
	}
	if !found {
		// Filter might not exist yet
		return &stats.FilterStats{
			Capacity:          s.opts.expectedItems,
			ItemCount:         0,
			FillRatio:         0,
			FalsePositiveRate: s.opts.falsePositiveRate,
			LastRebuild:       s.LastRebuild(),
			StorageType:       "redis",
		}, nil
	}

	fs := parseBloomInfo(result)
	fs.LastRebuild = s.LastRebuild()
	fs.StorageType = "redis"
	fs.FalsePositiveRate = s.opts.falsePositiveRate

	return fs, nil
}

// LastRebuild returns the time of the last successful rebuild.
func (s *Storage) LastRebuild() time.Time {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.lastRebuild
}

// SetLastRebuild updates the last rebuild timestamp.
func (s *Storage) SetLastRebuild(t time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastRebuild = t
}

// Close releases resources associated with the storage.
func (s *Storage) Close(_ context.Context) error {
	return nil
}

// parseBloomInfo parses BF.INFO response into FilterStats.
func parseBloomInfo(result any) *stats.FilterStats {
	fs := &stats.FilterStats{}

	for key, raw := range redisfilter.InfoFields(result) {
		switch key {
		case "Capacity":
			if val, err := redisfilter.ToInt64(raw); err == nil {
				fs.Capacity = val
			}
		case "Number of items inserted":
			if val, err := redisfilter.ToInt64(raw); err == nil {
				fs.ItemCount = val
			}
		case "Size":
			if val, err := redisfilter.ToInt64(raw); err == nil {
				fs.MemoryUsageBytes = val
			}
		}
	}

	if fs.Capacity > 0 {
		fs.FillRatio = float64(fs.ItemCount) / float64(fs.Capacity)
	}

	return fs
}
