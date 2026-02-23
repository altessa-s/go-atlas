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
	"sync"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/altessa-s/go-atlas/data/internal/redisbase"
	"github.com/altessa-s/go-atlas/data/probfilter/bloom/storages"
	"github.com/altessa-s/go-atlas/data/probfilter/stats"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
)

// Storage is a Redis-backed Bloom filter storage using RedisBloom BF.* commands.
// It is safe for concurrent use.
type Storage struct {
	redisbase.Base
	opts        *options
	filterKey   string
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

	s := &Storage{
		Base:      redisbase.NewBase(client, opts.keyPrefix),
		opts:      opts,
		filterKey: opts.keyPrefix + filterName,
	}

	return s
}

// MightExist checks if a value might exist in the filter.
func (s *Storage) MightExist(ctx context.Context, value string) (bool, error) {
	result, err := s.Client().Do(ctx, "BF.EXISTS", s.filterKey, value).Int()
	if err != nil {
		return false, coreerrs.WrapOperation(err, "check existence in Redis Bloom filter")
	}
	return result == 1, nil
}

// Add inserts a value into the filter.
func (s *Storage) Add(ctx context.Context, value string) error {
	_, err := s.Client().Do(ctx, "BF.ADD", s.filterKey, value).Result()
	if err != nil {
		// Check if filter doesn't exist and create it
		if strings.Contains(err.Error(), "not exist") {
			if ensureErr := s.ensureFilter(ctx); ensureErr != nil {
				return ensureErr
			}
			_, err = s.Client().Do(ctx, "BF.ADD", s.filterKey, value).Result()
		}
		if err != nil {
			return coreerrs.WrapOperation(err, "add to Redis Bloom filter")
		}
	}
	return nil
}

// AddBatch inserts multiple values into the filter.
func (s *Storage) AddBatch(ctx context.Context, values iter.Seq[string]) error {
	const batchSize = 1000 // Process in chunks to avoid huge Redis commands

	currentBatch := make([]any, 0, batchSize+2) // +2 for command and key
	for v := range values {
		if len(currentBatch) == 0 {
			currentBatch = append(currentBatch, "BF.MADD", s.filterKey)
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
			return coreerrs.WrapOperation(err, "batch add to Redis Bloom filter")
		}
	}
	return nil
}

// Reset clears the filter and prepares it for rebuild.
func (s *Storage) Reset(ctx context.Context, expectedItems int64) error {
	// Delete existing filter
	if err := s.Client().Del(ctx, s.filterKey).Err(); err != nil {
		return coreerrs.WrapOperation(err, "delete Redis Bloom filter")
	}

	// Create new filter with specified capacity
	if expectedItems <= 0 {
		expectedItems = s.opts.expectedItems
	}

	err := s.Client().Do(ctx, "BF.RESERVE", s.filterKey, s.opts.falsePositiveRate, expectedItems).Err()
	if err != nil {
		return coreerrs.WrapOperation(err, "create Redis Bloom filter")
	}

	return nil
}

// Stats returns current filter statistics.
func (s *Storage) Stats(ctx context.Context) (*stats.FilterStats, error) {
	result, err := s.Client().Do(ctx, "BF.INFO", s.filterKey).Result()
	if err != nil {
		// Filter might not exist yet
		if strings.Contains(err.Error(), "not exist") {
			return &stats.FilterStats{
				Capacity:          s.opts.expectedItems,
				ItemCount:         0,
				FillRatio:         0,
				FalsePositiveRate: s.opts.falsePositiveRate,
				LastRebuild:       s.lastRebuild,
				StorageType:       "redis",
			}, nil
		}
		return nil, coreerrs.WrapOperation(err, "get Redis Bloom filter info")
	}

	fs := parseBloomInfo(result)
	fs.LastRebuild = s.lastRebuild
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

// ensureFilter creates the filter if it doesn't exist.
func (s *Storage) ensureFilter(ctx context.Context) error {
	err := s.Client().Do(ctx, "BF.RESERVE", s.filterKey, s.opts.falsePositiveRate, s.opts.expectedItems).Err()
	if err != nil && !strings.Contains(err.Error(), "exists") {
		return coreerrs.WrapOperation(err, "create Redis Bloom filter")
	}
	return nil
}

// parseBloomInfo parses BF.INFO response into FilterStats.
func parseBloomInfo(result any) *stats.FilterStats {
	fs := &stats.FilterStats{}

	// BF.INFO returns alternating key-value pairs
	if v, ok := result.([]any); ok {
		for i := 0; i < len(v)-1; i += 2 {
			key, ok := v[i].(string)
			if !ok {
				continue
			}
			switch key {
			case "Capacity":
				if val, err := toInt64(v[i+1]); err == nil {
					fs.Capacity = val
				}
			case "Number of items inserted":
				if val, err := toInt64(v[i+1]); err == nil {
					fs.ItemCount = val
				}
			case "Size":
				if val, err := toInt64(v[i+1]); err == nil {
					fs.MemoryUsageBytes = val
				}
			}
		}
	}

	if fs.Capacity > 0 {
		fs.FillRatio = float64(fs.ItemCount) / float64(fs.Capacity)
	}

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
