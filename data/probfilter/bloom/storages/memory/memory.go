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

// shardSet is one generation of shards. Commit publishes a new set and marks
// the old one retired while holding all of its shard locks, so a writer that
// finds its set retired under the shard lock retries on the current one.
type shardSet struct {
	shards  []shard
	retired atomic.Bool
}

// index returns the shard for value.
func (set *shardSet) index(value string) *shard {
	if len(set.shards) <= 1 {
		return &set.shards[0]
	}
	return &set.shards[xxhash.Sum64String(value)%uint64(len(set.shards))]
}

// Storage is an in-memory Bloom filter storage using sharded bits-and-blooms/bloom filters.
// Sharding reduces RWMutex contention under concurrent read/write workloads across multi-core systems.
// It is safe for concurrent use.
type Storage struct {
	set         atomic.Pointer[shardSet]
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
	s := &Storage{opts: opts}
	s.set.Store(&shardSet{shards: newShards(opts.shards, opts.expectedItems, opts.falsePositiveRate)})
	return s
}

// add inserts value into the current generation, retrying when a concurrent
// Commit retired the set it loaded.
func (s *Storage) add(value string) {
	for {
		set := s.set.Load()
		sh := set.index(value)
		sh.mu.Lock()
		// Checked under the shard lock: Commit retires a set only while
		// holding all of its shard locks, so the write cannot be lost.
		if set.retired.Load() {
			sh.mu.Unlock()
			continue
		}
		sh.filter.AddString(value)
		sh.mu.Unlock()
		return
	}
}

// MightExist checks if a value might exist in the filter.
func (s *Storage) MightExist(_ context.Context, value string) (bool, error) {
	sh := s.set.Load().index(value)
	sh.mu.RLock()
	exists := sh.filter.TestString(value)
	sh.mu.RUnlock()
	return exists, nil
}

// Add inserts a value into the filter.
func (s *Storage) Add(_ context.Context, value string) error {
	s.add(value)
	s.itemCount.Add(1)
	return nil
}

// AddBatch inserts multiple values into the filter.
func (s *Storage) AddBatch(_ context.Context, values iter.Seq[string]) error {
	var count int64
	for v := range values {
		s.add(v)
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
		live: s,
		set:  &shardSet{shards: shards},
	}, nil
}

// staging is a replacement filter built off to the side of the live one.
type staging struct {
	live      *Storage
	set       *shardSet
	itemCount int64
}

// AddBatch inserts values into the replacement filter.
func (st *staging) AddBatch(_ context.Context, values iter.Seq[string]) error {
	for v := range values {
		st.set.index(v).filter.AddString(v)
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

	// Hold every live shard lock so no write lands in the old set after it
	// is retired; writers that then find it retired retry on the new set.
	old := st.live.set.Load()
	for i := range old.shards {
		old.shards[i].mu.Lock()
		defer old.shards[i].mu.Unlock()
	}

	old.retired.Store(true)
	st.live.set.Store(st.set)
	st.live.itemCount.Store(st.itemCount)
	return nil
}

// Abort discards the replacement filter.
func (st *staging) Abort(_ context.Context) error {
	st.set = nil
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

	set := s.set.Load()
	for i := range set.shards {
		sh := &set.shards[i]
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
