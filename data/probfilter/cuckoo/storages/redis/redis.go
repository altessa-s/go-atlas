// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package redis

import (
	"context"
	"iter"
	"strings"

	"github.com/redis/go-redis/v9"

	"github.com/altessa-s/go-atlas/data/internal/redisbase"
	"github.com/altessa-s/go-atlas/data/probfilter/cuckoo/storages"
	"github.com/altessa-s/go-atlas/data/probfilter/internal/redisfilter"
	"github.com/altessa-s/go-atlas/data/probfilter/stats"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
)

// defaultCuckooFPR is the default false positive rate for Cuckoo filters.
// Cuckoo filter FPR is approximately 2^-f where f is fingerprint bits.
// Default is ~8 bits = ~0.4%.
const defaultCuckooFPR = 0.004

// Storage is a Redis-backed Cuckoo filter storage using RedisBloom CF.* commands.
// It is safe for concurrent use.
type Storage struct {
	redisbase.Base
	core *redisfilter.Core
	opts *options
}

var (
	_ storages.Storage               = (*Storage)(nil)
	_ storages.ExclusiveRebuilder    = (*Storage)(nil)
	_ storages.RebuildCommitReporter = (*Storage)(nil)
)

// New creates a new Redis Cuckoo filter Storage.
// Requires Redis with RedisBloom module installed.
//
// Example:
//
//	storage := redis.New(rdb, "myfilter", redis.WithCapacity(100000))
func New(client redis.UniversalClient, filterName string, opt ...Option) *Storage {
	opts := newOptions(opt...)

	s := &Storage{opts: opts}
	s.Base = redisbase.NewBase(client, opts.keyPrefix)
	s.core = redisfilter.New(client, opts.keyPrefix+filterName, redisfilter.Commands{
		Label:  "Cuckoo",
		Exists: "CF.EXISTS",
		// Live inserts never create a missing filter: it is recreated by the
		// reserve path, which also clears the rebuild ready marker.
		Add:         "CF.INSERT",
		AddTokens:   []string{"NOCREATE", "ITEMS"},
		AddBatch:    "CF.INSERT",
		BatchTokens: []string{"NOCREATE", "ITEMS"},
		// CF.INSERT answers a full filter with false under RESP3.
		FalseRejects: true,
		// Staging batches must not recreate a vanished staging key.
		StagingAddBatch:    "CF.INSERT",
		StagingBatchTokens: []string{"NOCREATE", "ITEMS"},
		Reserve:            "CF.RESERVE",
		Info:               "CF.INFO",
	}, s.reserveArgs(opts.capacity)...)
	return s
}

// reserveArgs returns the CF.RESERVE arguments for a filter of capacity.
func (s *Storage) reserveArgs(capacity int64) []any {
	if s.opts.expansion > 0 {
		return []any{capacity, "EXPANSION", s.opts.expansion}
	}
	return []any{capacity}
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

// Delete removes a value from the filter. The delete is bound to the filter
// generation that is live when it starts: if a rebuild replaces the filter
// before the delete executes (for example a request delayed past a client
// timeout), it does nothing — instead of removing a colliding member of the
// new filter — and Delete returns an error wrapping
// "filter replaced during delete"; a retry runs against the new filter.
func (s *Storage) Delete(ctx context.Context, value string) (bool, error) {
	reply, err := s.core.Delete(ctx, "CF.DEL", value)
	if err != nil {
		// RedisBloom reports a missing filter as "Not found" (older versions:
		// "... does not exist"): nothing to delete.
		if msg := strings.ToLower(err.Error()); strings.Contains(msg, "not found") || strings.Contains(msg, "not exist") {
			return false, nil
		}
		return false, coreerrs.WrapOperation(err, "delete from Redis Cuckoo filter")
	}
	deleted, err := redisfilter.ToBool(reply)
	if err != nil {
		return false, coreerrs.WrapOperation(err, "delete from Redis Cuckoo filter")
	}
	return deleted, nil
}

// Stage reserves an empty replacement filter with room for expectedItems plus
// 25% headroom (never less than the configured capacity) under a private
// staging key in the live key's cluster hash slot. Committing renames it onto
// the live key in one atomic step; until then the live filter is untouched.
func (s *Storage) Stage(ctx context.Context, expectedItems int64) (storages.Staging, error) {
	st, err := s.core.Stage(ctx, s.reserveArgs(s.stagingCapacity(expectedItems))...)
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

// rebuildLease stages replacement filters under a held rebuild lease.
type rebuildLease struct {
	storage *Storage
	lease   *redisfilter.Lease
}

func (l *rebuildLease) Stage(ctx context.Context, expectedItems int64) (storages.Staging, error) {
	st, err := l.lease.Stage(ctx, l.storage.reserveArgs(l.storage.stagingCapacity(expectedItems))...)
	if err != nil {
		return nil, err
	}
	return st, nil
}

func (l *rebuildLease) Release(ctx context.Context) error {
	return l.lease.Release(ctx)
}

// stagingCapacity returns the capacity of a replacement filter for
// expectedItems: 25% headroom, never less than the configured capacity.
func (s *Storage) stagingCapacity(expectedItems int64) int64 {
	if expectedItems > 0 {
		return max(s.opts.capacity, expectedItems+expectedItems/stageHeadroomDivisor)
	}
	return s.opts.capacity
}

// stageHeadroomDivisor sizes a rebuilt filter with 1/stageHeadroomDivisor
// spare capacity over the loaded item count.
const stageHeadroomDivisor = 4

// Stats returns current filter statistics.
func (s *Storage) Stats(ctx context.Context) (*stats.FilterStats, error) {
	result, found, err := s.core.Info(ctx)
	if err != nil {
		return nil, err
	}
	if !found {
		// Filter might not exist yet
		return &stats.FilterStats{
			Capacity:          s.opts.capacity,
			ItemCount:         0,
			FillRatio:         0,
			FalsePositiveRate: defaultCuckooFPR,
			StorageType:       "redis",
		}, nil
	}

	fs := parseCuckooInfo(result)
	fs.StorageType = "redis"

	return fs, nil
}

// Close releases resources associated with the storage.
func (s *Storage) Close(_ context.Context) error {
	return nil
}

// parseCuckooInfo parses CF.INFO response into FilterStats.
func parseCuckooInfo(result any) *stats.FilterStats {
	fs := &stats.FilterStats{}

	for key, raw := range redisfilter.InfoFields(result) {
		switch key {
		case "Size":
			if val, err := redisfilter.ToInt64(raw); err == nil {
				fs.Capacity = val
			}
		case "Number of items inserted":
			if val, err := redisfilter.ToInt64(raw); err == nil {
				fs.ItemCount = val
			}
		}
	}

	if fs.Capacity > 0 {
		fs.FillRatio = float64(fs.ItemCount) / float64(fs.Capacity)
	}

	// Cuckoo filter FPR is approximately 2^-f where f is fingerprint bits
	// Default is ~8 bits = ~0.4%
	fs.FalsePositiveRate = defaultCuckooFPR

	return fs
}
