// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package bloom

import (
	"context"
	"iter"
	"time"

	"github.com/altessa-s/go-atlas/data/probfilter"
	"github.com/altessa-s/go-atlas/data/probfilter/bloom/storages"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
)

// Filter is a Bloom filter facade that wraps a storage backend.
// It implements probfilter.RebuildableFilter.
type Filter struct {
	storage storages.Storage
}

var (
	_ probfilter.Filter            = (*Filter)(nil)
	_ probfilter.RebuildableFilter = (*Filter)(nil)
	_ probfilter.StatsProvider     = (*Filter)(nil)
)

// New creates a new Bloom filter with the specified storage backend.
//
// Example:
//
//	storage := memory.New(memory.WithExpectedItems(100000))
//	filter := bloom.New(storage)
func New(storage storages.Storage) *Filter {
	return &Filter{
		storage: storage,
	}
}

// MightExist checks if a value might exist in the filter.
func (f *Filter) MightExist(ctx context.Context, value string) (bool, error) {
	return f.storage.MightExist(ctx, value)
}

// Add inserts a value into the filter.
func (f *Filter) Add(ctx context.Context, value string) error {
	return f.storage.Add(ctx, value)
}

// AddBatch inserts multiple values into the filter.
func (f *Filter) AddBatch(ctx context.Context, values iter.Seq[string]) error {
	return f.storage.AddBatch(ctx, values)
}

// Stats returns current filter statistics.
func (f *Filter) Stats(ctx context.Context) (*probfilter.FilterStats, error) {
	if sp, ok := f.storage.(storages.StatsProvider); ok {
		return sp.Stats(ctx)
	}
	return &probfilter.FilterStats{}, nil
}

// Close releases resources associated with the filter.
func (f *Filter) Close(ctx context.Context) error {
	return f.storage.Close(ctx)
}

// Rebuild recreates the filter from scratch using the provided data loader.
func (f *Filter) Rebuild(ctx context.Context, loader probfilter.DataLoader) error {
	// Try to get count from loader first
	count, err := loader.Count(ctx)

	// If count is available and valid, use optimized path with Reset + StreamValues
	if err == nil && count > 0 {
		if err := f.storage.Reset(ctx, count); err != nil {
			return coreerrs.WrapOperation(err, "reset filter")
		}

		for value, err := range loader.StreamValues(ctx) {
			if err != nil {
				return coreerrs.WrapOperation(err, "load value during rebuild")
			}
			if err := f.storage.Add(ctx, value); err != nil {
				return coreerrs.WrapOperation(err, "add value during rebuild")
			}
		}

		f.storage.SetLastRebuild(time.Now())
		return nil
	}

	// Fallback: count not available, collect all values first to avoid iterator exhaustion.
	// Many iterators (e.g., database cursors) can only be iterated once.
	values := make([]string, 0)
	for value, err := range loader.StreamValues(ctx) {
		if err != nil {
			return coreerrs.WrapOperation(err, "load value during rebuild")
		}
		values = append(values, value)
	}

	// Reset filter with actual count
	if err := f.storage.Reset(ctx, int64(len(values))); err != nil {
		return coreerrs.WrapOperation(err, "reset filter")
	}

	// Add collected values
	for _, value := range values {
		if err := f.storage.Add(ctx, value); err != nil {
			return coreerrs.WrapOperation(err, "add value during rebuild")
		}
	}

	f.storage.SetLastRebuild(time.Now())
	return nil
}

// LastRebuild returns the time of the last successful rebuild.
func (f *Filter) LastRebuild() time.Time {
	return f.storage.LastRebuild()
}
