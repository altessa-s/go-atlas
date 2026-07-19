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

var _ storages.Storage = (*Storage)(nil)

// New creates a new Redis Cuckoo filter Storage.
// Requires Redis with RedisBloom module installed.
//
// Example:
//
//	storage := redis.New(rdb, "myfilter", redis.WithCapacity(100000))
func New(client redis.UniversalClient, filterName string, opt ...Option) *Storage {
	opts := newOptions(opt...)

	return &Storage{
		Base: redisbase.NewBase(client, opts.keyPrefix),
		core: redisfilter.New(client, opts.keyPrefix+filterName, redisfilter.Commands{
			Label:       "Cuckoo",
			Exists:      "CF.EXISTS",
			Add:         "CF.ADD",
			AddBatch:    "CF.INSERT",
			BatchTokens: []string{"ITEMS"},
			Reserve:     "CF.RESERVE",
			Info:        "CF.INFO",
		}, opts.capacity),
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

// Delete removes a value from the filter.
func (s *Storage) Delete(ctx context.Context, value string) (bool, error) {
	result, err := s.Client().Do(ctx, "CF.DEL", s.core.FilterKey(), value).Int()
	if err != nil {
		if strings.Contains(err.Error(), "not exist") {
			return false, nil
		}
		return false, coreerrs.WrapOperation(err, "delete from Redis Cuckoo filter")
	}
	return result == 1, nil
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
