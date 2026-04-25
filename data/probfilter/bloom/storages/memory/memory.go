// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package memory

import (
	"context"
	"iter"
	"math"
	"sync"
	"time"

	"github.com/bits-and-blooms/bloom/v3"

	"github.com/altessa-s/go-atlas/data/probfilter/bloom/storages"
	"github.com/altessa-s/go-atlas/data/probfilter/stats"
)

// bitsPerByte is the number of bits in a byte, used for memory calculation.
const bitsPerByte = 8

// Storage is an in-memory Bloom filter storage using bits-and-blooms/bloom.
// It is safe for concurrent use.
type Storage struct {
	filter      *bloom.BloomFilter
	mu          sync.RWMutex
	opts        *options
	itemCount   int64
	lastRebuild time.Time
}

var _ storages.Storage = (*Storage)(nil)

// New creates a new in-memory Bloom filter Storage.
//
// Example:
//
//	storage := memory.New(
//	    memory.WithExpectedItems(100000),
//	    memory.WithFalsePositiveRate(0.01),
//	)
func New(opt ...Option) *Storage {
	opts := newOptions(opt...)
	return &Storage{
		//nolint:gosec // G115: expectedItems is validated positive via options
		filter: bloom.NewWithEstimates(uint(opts.expectedItems), opts.falsePositiveRate),
		opts:   opts,
	}
}

// MightExist checks if a value might exist in the filter.
func (s *Storage) MightExist(_ context.Context, value string) (bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.filter.TestString(value), nil
}

// Add inserts a value into the filter.
func (s *Storage) Add(_ context.Context, value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.filter.AddString(value)
	s.itemCount++
	return nil
}

// AddBatch inserts multiple values into the filter.
func (s *Storage) AddBatch(_ context.Context, values iter.Seq[string]) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for v := range values {
		s.filter.AddString(v)
		s.itemCount++
	}
	return nil
}

// Reset clears the filter and prepares it for rebuild.
func (s *Storage) Reset(_ context.Context, expectedItems int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if expectedItems <= 0 {
		expectedItems = s.opts.expectedItems
	}

	//nolint:gosec // G115: expectedItems is validated positive above
	s.filter = bloom.NewWithEstimates(uint(expectedItems), s.opts.falsePositiveRate)
	s.itemCount = 0
	return nil
}

// Stats returns current filter statistics.
func (s *Storage) Stats(_ context.Context) (*stats.FilterStats, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	//nolint:gosec // G115: filter.Cap() returns uint, bloom filter capacity fits in int64
	capacity := int64(s.filter.Cap())
	// ApproximatedSize gives an estimate based on bit array state
	approxCount := int64(s.filter.ApproximatedSize()) //nolint:gosec // G115: same as capacity

	var fillRatio float64
	if capacity > 0 {
		fillRatio = float64(approxCount) / float64(capacity)
	}

	// Estimate false positive rate based on current fill
	// FPR = (1 - e^(-k*n/m))^k where k=hash functions, n=items, m=bits
	k := float64(s.filter.K())
	m := float64(s.filter.Cap())
	n := float64(s.itemCount)
	estimatedFPR := s.opts.falsePositiveRate
	if m > 0 && n > 0 {
		estimatedFPR = math.Pow(1-math.Exp(-k*n/m), k)
	}

	return &stats.FilterStats{
		Capacity:          capacity,
		ItemCount:         approxCount,
		FillRatio:         fillRatio,
		FalsePositiveRate: estimatedFPR,
		MemoryUsageBytes:  int64(s.filter.Cap() / bitsPerByte), //nolint:gosec // G115: capacity in bytes fits int64
		LastRebuild:       s.lastRebuild,
		StorageType:       "memory",
	}, nil
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
