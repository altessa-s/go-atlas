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

	"github.com/altessa-s/go-atlas/data/internal/redisbase"
	"github.com/altessa-s/go-atlas/data/probfilter/bloom/storages"
	"github.com/altessa-s/go-atlas/data/probfilter/internal/redisfilter"
	"github.com/altessa-s/go-atlas/data/probfilter/stats"
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

var _ storages.Storage = (*Storage)(nil)

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

	return &Storage{
		Base: redisbase.NewBase(client, opts.keyPrefix),
		core: redisfilter.New(client, opts.keyPrefix+filterName, redisfilter.Commands{
			Label:    "Bloom",
			Exists:   "BF.EXISTS",
			Add:      "BF.ADD",
			AddBatch: "BF.MADD",
			// Staging batches must not recreate a vanished staging key.
			StagingAddBatch:    "BF.INSERT",
			StagingBatchTokens: []string{"NOCREATE", "ITEMS"},
			Reserve:            "BF.RESERVE",
			Info:               "BF.INFO",
		}, opts.falsePositiveRate, opts.expectedItems),
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
