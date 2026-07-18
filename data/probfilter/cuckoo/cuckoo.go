// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package cuckoo

import (
	"context"
	"iter"

	"github.com/altessa-s/go-atlas/data/probfilter"
	"github.com/altessa-s/go-atlas/data/probfilter/cuckoo/storages"
)

// Filter is a Cuckoo filter facade that wraps a storage backend.
// It implements probfilter.DeletableFilter.
type Filter struct {
	storage storages.Storage
}

var (
	_ probfilter.Filter          = (*Filter)(nil)
	_ probfilter.DeletableFilter = (*Filter)(nil)
	_ probfilter.StatsProvider   = (*Filter)(nil)
)

// New creates a new Cuckoo filter with the specified storage backend.
//
// Example:
//
//	storage := memory.New(memory.WithCapacity(100000))
//	filter := cuckoo.New(storage)
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

// Delete removes a value from the filter.
func (f *Filter) Delete(ctx context.Context, value string) (bool, error) {
	return f.storage.Delete(ctx, value)
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
