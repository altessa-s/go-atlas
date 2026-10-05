// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package memory

import (
	"context"
	"errors"
	"iter"
	"sync"

	"github.com/altessa-s/go-atlas/data/probfilter/cuckoo/storages"
	"github.com/altessa-s/go-atlas/data/probfilter/stats"
)

// cuckooFingerprintBits is the number of distinct 8-bit fingerprint values.
// This determines the false positive rate: FPR ≈ 2^-f ≈ 1/256 ≈ 0.4%.
const cuckooFingerprintBits = 256.0

// ErrFilterFull is returned when the cuckoo filter is full and cannot accept
// more items. The failed insert leaves the filter unchanged: no previously
// added value is evicted.
var ErrFilterFull = errors.New("cuckoo filter is full")

// Storage is an in-memory Cuckoo filter storage (4-slot buckets, 8-bit
// fingerprints). A failed insert never evicts an existing member, so every
// added and not deleted value keeps answering true from MightExist.
// It is safe for concurrent use.
type Storage struct {
	filter *cuckooFilter
	mu     sync.RWMutex
	opts   *options
	// capacity is the requested capacity of filter: the configured one, or
	// the one a rebuild sized it to.
	capacity uint
}

var _ storages.Storage = (*Storage)(nil)

// New creates a new in-memory Cuckoo filter Storage.
//
// Example:
//
//	storage := memory.New(memory.WithCapacity(100000))
func New(opt ...Option) *Storage {
	opts := newOptions(opt...)
	return &Storage{
		filter:   newCuckooFilter(opts.capacity),
		opts:     opts,
		capacity: opts.capacity,
	}
}

// MightExist checks if a value might exist in the filter.
func (s *Storage) MightExist(_ context.Context, value string) (bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.filter.lookup(value), nil
}

// Add inserts a value into the filter.
func (s *Storage) Add(_ context.Context, value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.filter.insert(value) {
		return ErrFilterFull
	}
	return nil
}

// AddBatch inserts multiple values into the filter.
func (s *Storage) AddBatch(_ context.Context, values iter.Seq[string]) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return insertAll(s.filter, values)
}

// insertAll inserts values into f, stopping with [ErrFilterFull] at the
// first value that does not fit.
func insertAll(f *cuckooFilter, values iter.Seq[string]) error {
	for v := range values {
		if !f.insert(v) {
			return ErrFilterFull
		}
	}
	return nil
}

// Delete removes a value from the filter.
func (s *Storage) Delete(_ context.Context, value string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.filter.remove(value), nil
}

// Stage creates an empty replacement filter with room for expectedItems plus
// 25% headroom, never smaller than the configured capacity. The live filter is
// untouched until [storages.Staging.Commit].
func (s *Storage) Stage(_ context.Context, expectedItems int64) (storages.Staging, error) {
	capacity := s.opts.capacity
	if expectedItems > 0 {
		//nolint:gosec // G115: expectedItems is positive here
		capacity = max(capacity, uint(expectedItems+expectedItems/stageHeadroomDivisor))
	}
	return &staging{live: s, filter: newCuckooFilter(capacity), capacity: capacity}, nil
}

// stageHeadroomDivisor sizes a rebuilt filter with 1/stageHeadroomDivisor
// spare capacity over the loaded item count, keeping inserts away from the
// load factor at which cuckoo insertion starts failing.
const stageHeadroomDivisor = 4

// staging is a replacement filter built off to the side of the live one.
type staging struct {
	live     *Storage
	filter   *cuckooFilter
	capacity uint
}

// AddBatch inserts values into the replacement filter. It returns
// [ErrFilterFull] when the replacement cannot hold them.
func (st *staging) AddBatch(_ context.Context, values iter.Seq[string]) error {
	return insertAll(st.filter, values)
}

// Commit swaps the replacement filter in under the storage lock.
func (st *staging) Commit(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	st.live.mu.Lock()
	defer st.live.mu.Unlock()
	st.live.filter = st.filter
	st.live.capacity = st.capacity
	return nil
}

// Abort discards the replacement filter.
func (st *staging) Abort(_ context.Context) error {
	st.filter = nil
	return nil
}

// Stats returns current filter statistics.
func (s *Storage) Stats(_ context.Context) (*stats.FilterStats, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	count := int64(s.filter.count) //nolint:gosec // G115: filter count fits in int64
	capacity := int64(s.capacity)  //nolint:gosec // G115: capacity is validated via options

	var fillRatio float64
	if capacity > 0 {
		fillRatio = float64(count) / float64(capacity)
	}

	// Cuckoo filter FPR is approximately 2^-f where f is fingerprint bits
	// Fingerprints are 8 bits
	estimatedFPR := 1.0 / cuckooFingerprintBits

	return &stats.FilterStats{
		Capacity:          capacity,
		ItemCount:         count,
		FillRatio:         fillRatio,
		FalsePositiveRate: estimatedFPR,
		StorageType:       "memory",
	}, nil
}

// Close releases resources associated with the storage.
func (s *Storage) Close(_ context.Context) error {
	return nil
}
