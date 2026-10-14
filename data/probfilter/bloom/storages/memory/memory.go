// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package memory

import (
	"context"
	"iter"
	"math"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bits-and-blooms/bloom/v3"
	"github.com/cespare/xxhash/v2"

	"github.com/altessa-s/go-atlas/data/probfilter/bloom/storages"
	"github.com/altessa-s/go-atlas/data/probfilter/stats"
)

// bitsPerByte is the number of bits in a byte, used for memory calculation.
const bitsPerByte = 8

type shard struct {
	filter *bloom.BloomFilter
	mu     sync.RWMutex
}

// Storage is an in-memory Bloom filter storage using sharded bits-and-blooms/bloom filters.
// Sharding reduces RWMutex contention under concurrent read/write workloads across multi-core systems.
// It is safe for concurrent use.
type Storage struct {
	shards      []shard
	numShards   uint64
	opts        *options
	itemCount   atomic.Int64
	lastRebuild atomic.Pointer[time.Time]
	commitMu    sync.Mutex
}

var _ storages.Storage = (*Storage)(nil)

func newShards(numShards int, expectedItems int64, fpRate float64) []shard {
	if numShards <= 0 {
		numShards = DefaultShards
	}
	itemsPerShard := max(uint(math.Ceil(float64(expectedItems)/float64(numShards))), 1)

	shards := make([]shard, numShards)
	for i := range shards {
		shards[i].filter = bloom.NewWithEstimates(itemsPerShard, fpRate)
	}
	return shards
}

// New creates a new in-memory Bloom filter Storage.
//
// Example:
//
//	storage := memory.New(
//	    memory.WithExpectedItems(100000),
//	    memory.WithFalsePositiveRate(0.01),
//	    memory.WithShards(16),
//	)
func New(opt ...Option) *Storage {
	opts := newOptions(opt...)
	shards := newShards(opts.shards, opts.expectedItems, opts.falsePositiveRate)
	return &Storage{
		shards:    shards,
		numShards: uint64(len(shards)),
		opts:      opts,
	}
}

func (s *Storage) shardIndex(value string) uint64 {
	if s.numShards <= 1 {
		return 0
	}
	return xxhash.Sum64String(value) % s.numShards
}

// MightExist checks if a value might exist in the filter.
func (s *Storage) MightExist(_ context.Context, value string) (bool, error) {
	idx := s.shardIndex(value)
	sh := &s.shards[idx]
	sh.mu.RLock()
	exists := sh.filter.TestString(value)
	sh.mu.RUnlock()
	return exists, nil
}

// Add inserts a value into the filter.
func (s *Storage) Add(_ context.Context, value string) error {
	idx := s.shardIndex(value)
	sh := &s.shards[idx]
	sh.mu.Lock()
	sh.filter.AddString(value)
	sh.mu.Unlock()
	s.itemCount.Add(1)
	return nil
}

// AddBatch inserts multiple values into the filter.
func (s *Storage) AddBatch(_ context.Context, values iter.Seq[string]) error {
	var count int64
	for v := range values {
		idx := s.shardIndex(v)
		sh := &s.shards[idx]
		sh.mu.Lock()
		sh.filter.AddString(v)
		sh.mu.Unlock()
		count++
	}
	s.itemCount.Add(count)
	return nil
}

// Stage creates an empty replacement filter sized for expectedItems (the
// configured size when expectedItems <= 0). The live filter is untouched
// until [storages.Staging.Commit].
func (s *Storage) Stage(_ context.Context, expectedItems int64) (storages.Staging, error) {
	if expectedItems <= 0 {
		expectedItems = s.opts.expectedItems
	}

	shards := newShards(s.opts.shards, expectedItems, s.opts.falsePositiveRate)
	return &staging{
		live:   s,
		shards: shards,
	}, nil
}

// staging is a replacement filter built off to the side of the live one.
type staging struct {
	live      *Storage
	shards    []shard
	itemCount int64
}

// AddBatch inserts values into the replacement filter.
func (st *staging) AddBatch(_ context.Context, values iter.Seq[string]) error {
	for v := range values {
		idx := st.live.shardIndex(v)
		sh := &st.shards[idx]
		sh.filter.AddString(v)
		st.itemCount++
	}
	return nil
}

// Commit swaps the replacement filter in under the storage lock.
func (st *staging) Commit(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	st.live.commitMu.Lock()
	defer st.live.commitMu.Unlock()

	// Acquire write locks on all live shards in order
	for i := range st.live.shards {
		st.live.shards[i].mu.Lock()
		defer st.live.shards[i].mu.Unlock()
	}

	st.live.shards = st.shards
	st.live.numShards = uint64(len(st.shards))
	st.live.itemCount.Store(st.itemCount)
	return nil
}

// Abort discards the replacement filter.
func (st *staging) Abort(_ context.Context) error {
	st.shards = nil
	return nil
}

// Stats returns current filter statistics aggregated across all shards.
func (s *Storage) Stats(_ context.Context) (*stats.FilterStats, error) {
	var (
		totalCapacity    int64
		totalApproxCount int64
		totalBits        int64
		k                float64
	)

	for i := range s.shards {
		sh := &s.shards[i]
		sh.mu.RLock()
		totalCapacity += int64(sh.filter.Cap())
		totalApproxCount += int64(sh.filter.ApproximatedSize())
		totalBits += int64(sh.filter.Cap())
		k = float64(sh.filter.K())
		sh.mu.RUnlock()
	}

	var fillRatio float64
	if totalCapacity > 0 {
		fillRatio = float64(totalApproxCount) / float64(totalCapacity)
	}

	n := float64(s.itemCount.Load())
	m := float64(totalBits)
	estimatedFPR := s.opts.falsePositiveRate
	if m > 0 && n > 0 {
		estimatedFPR = math.Pow(1-math.Exp(-k*n/m), k)
	}

	return &stats.FilterStats{
		Capacity:          totalCapacity,
		ItemCount:         totalApproxCount,
		FillRatio:         fillRatio,
		FalsePositiveRate: estimatedFPR,
		MemoryUsageBytes:  totalBits / bitsPerByte,
		LastRebuild:       s.LastRebuild(),
		StorageType:       "memory",
	}, nil
}

// LastRebuild returns the time of the last successful rebuild.
func (s *Storage) LastRebuild() time.Time {
	if p := s.lastRebuild.Load(); p != nil {
		return *p
	}
	return time.Time{}
}

// SetLastRebuild updates the last rebuild timestamp.
func (s *Storage) SetLastRebuild(t time.Time) {
	s.lastRebuild.Store(&t)
}

// Close releases resources associated with the storage.
func (s *Storage) Close(_ context.Context) error {
	return nil
}
