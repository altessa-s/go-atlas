// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package redis

import (
	"context"
	"fmt"
	"iter"
	"strconv"
	"strings"

	"github.com/redis/go-redis/v9"

	"github.com/altessa-s/go-atlas/data/internal/redisbase"
	"github.com/altessa-s/go-atlas/data/probfilter/cuckoo/storages"
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
	opts      *options
	filterKey string
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
		Base:      redisbase.NewBase(client, opts.keyPrefix),
		opts:      opts,
		filterKey: opts.keyPrefix + filterName,
	}
}

// MightExist checks if a value might exist in the filter.
func (s *Storage) MightExist(ctx context.Context, value string) (bool, error) {
	result, err := s.Client().Do(ctx, "CF.EXISTS", s.filterKey, value).Int()
	if err != nil {
		return false, coreerrs.WrapOperation(err, "check existence in Redis Cuckoo filter")
	}
	return result == 1, nil
}

// Add inserts a value into the filter.
func (s *Storage) Add(ctx context.Context, value string) error {
	_, err := s.Client().Do(ctx, "CF.ADD", s.filterKey, value).Result()
	if err != nil {
		// Check if filter doesn't exist and create it
		if strings.Contains(err.Error(), "not exist") {
			if ensureErr := s.ensureFilter(ctx); ensureErr != nil {
				return ensureErr
			}
			_, err = s.Client().Do(ctx, "CF.ADD", s.filterKey, value).Result()
		}
		if err != nil {
			return coreerrs.WrapOperation(err, "add to Redis Cuckoo filter")
		}
	}
	return nil
}

// AddBatch inserts multiple values into the filter.
func (s *Storage) AddBatch(ctx context.Context, values iter.Seq[string]) error {
	const batchSize = 1000

	currentBatch := make([]any, 0, batchSize+3) // +3 for command, key, and "ITEMS"
	for v := range values {
		if len(currentBatch) == 0 {
			currentBatch = append(currentBatch, "CF.INSERT", s.filterKey, "ITEMS")
		}
		currentBatch = append(currentBatch, v)

		if len(currentBatch) >= batchSize {
			if err := s.sendBatch(ctx, currentBatch); err != nil {
				return err
			}
			// Reuse underlying array
			currentBatch = currentBatch[:0]
		}
	}

	if len(currentBatch) > 0 {
		return s.sendBatch(ctx, currentBatch)
	}

	return nil
}

func (s *Storage) sendBatch(ctx context.Context, args []any) error {
	_, err := s.Client().Do(ctx, args...).Result()
	if err != nil {
		// Check if filter doesn't exist and create it
		if strings.Contains(err.Error(), "not exist") {
			if ensureErr := s.ensureFilter(ctx); ensureErr != nil {
				return ensureErr
			}
			_, err = s.Client().Do(ctx, args...).Result()
		}
		if err != nil {
			return coreerrs.WrapOperation(err, "batch add to Redis Cuckoo filter")
		}
	}
	return nil
}

// Delete removes a value from the filter.
func (s *Storage) Delete(ctx context.Context, value string) (bool, error) {
	result, err := s.Client().Do(ctx, "CF.DEL", s.filterKey, value).Int()
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
	result, err := s.Client().Do(ctx, "CF.INFO", s.filterKey).Result()
	if err != nil {
		// Filter might not exist yet
		if strings.Contains(err.Error(), "not exist") {
			return &stats.FilterStats{
				Capacity:          s.opts.capacity,
				ItemCount:         0,
				FillRatio:         0,
				FalsePositiveRate: defaultCuckooFPR,
				StorageType:       "redis",
			}, nil
		}
		return nil, coreerrs.WrapOperation(err, "get Redis Cuckoo filter info")
	}

	stats := parseCuckooInfo(result)
	stats.StorageType = "redis"

	return stats, nil
}

// Close releases resources associated with the storage.
func (s *Storage) Close(_ context.Context) error {
	return nil
}

// ensureFilter creates the filter if it doesn't exist.
func (s *Storage) ensureFilter(ctx context.Context) error {
	err := s.Client().Do(ctx, "CF.RESERVE", s.filterKey, s.opts.capacity).Err()
	if err != nil && !strings.Contains(err.Error(), "exists") {
		return coreerrs.WrapOperation(err, "create Redis Cuckoo filter")
	}
	return nil
}

// parseCuckooInfo parses CF.INFO response into FilterStats.
func parseCuckooInfo(result any) *stats.FilterStats {
	fs := &stats.FilterStats{}

	// CF.INFO returns alternating key-value pairs
	if v, ok := result.([]any); ok {
		for i := 0; i < len(v)-1; i += 2 {
			key, ok := v[i].(string)
			if !ok {
				continue
			}
			switch key {
			case "Size":
				if val, err := toInt64(v[i+1]); err == nil {
					fs.Capacity = val
				}
			case "Number of items inserted":
				if val, err := toInt64(v[i+1]); err == nil {
					fs.ItemCount = val
				}
			case "Number of filters":
				// Could be used for memory estimation
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

func toInt64(v any) (int64, error) {
	switch val := v.(type) {
	case int64:
		return val, nil
	case int:
		return int64(val), nil
	case string:
		return strconv.ParseInt(val, 10, 64)
	default:
		return 0, fmt.Errorf("cannot convert %T to int64", v)
	}
}
