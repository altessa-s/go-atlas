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

	cuckoo "github.com/seiflotfy/cuckoofilter"
)

// cuckooFingerprintBits is the number of fingerprint bits used by seiflotfy/cuckoofilter.
// This determines the false positive rate: FPR ≈ 2^-f ≈ 1/256 ≈ 0.4%.
const cuckooFingerprintBits = 256.0

// ErrFilterFull is returned when the cuckoo filter is full and cannot accept more items.
var ErrFilterFull = errors.New("cuckoo filter is full")

// Storage is an in-memory Cuckoo filter storage using seiflotfy/cuckoofilter.
// It is safe for concurrent use.
type Storage struct {
	filter *cuckoo.Filter
	mu     sync.RWMutex
	opts   *options
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
		filter: cuckoo.NewFilter(opts.capacity),
		opts:   opts,
	}
}

// MightExist checks if a value might exist in the filter.
func (s *Storage) MightExist(_ context.Context, value string) (bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.filter.Lookup([]byte(value)), nil
}

// Add inserts a value into the filter.
func (s *Storage) Add(_ context.Context, value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.filter.Insert([]byte(value)) {
		return ErrFilterFull
	}
	return nil
}

// AddBatch inserts multiple values into the filter.
func (s *Storage) AddBatch(_ context.Context, values iter.Seq[string]) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for v := range values {
		if !s.filter.Insert([]byte(v)) {
			return ErrFilterFull
		}
	}
	return nil
}

// Delete removes a value from the filter.
func (s *Storage) Delete(_ context.Context, value string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.filter.Delete([]byte(value)), nil
}

// Stats returns current filter statistics.
func (s *Storage) Stats(_ context.Context) (*stats.FilterStats, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	count := int64(s.filter.Count())   //nolint:gosec // G115: filter count fits in int64
	capacity := int64(s.opts.capacity) //nolint:gosec // G115: capacity is validated via options

	var fillRatio float64
	if capacity > 0 {
		fillRatio = float64(count) / float64(capacity)
	}

	// Cuckoo filter FPR is approximately 2^-f where f is fingerprint bits
	// Default fingerprint size in seiflotfy/cuckoofilter is ~8 bits
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
