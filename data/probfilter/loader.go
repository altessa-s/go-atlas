// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package probfilter

import (
	"context"
	"iter"
)

// DataLoader provides streaming access to values for filter rebuilds.
// Implementations should efficiently stream large datasets without
// loading everything into memory.
type DataLoader interface {
	// StreamValues returns an iterator that yields values for the filter.
	// The iterator should stop early if the context is canceled.
	// Each yielded pair contains the value and any error encountered.
	StreamValues(ctx context.Context) iter.Seq2[string, error]

	// Count returns the total number of items available for loading.
	// This is used to size the filter appropriately before loading.
	Count(ctx context.Context) (int64, error)
}

// DataLoaderFunc is an adapter to allow ordinary functions to be used as DataLoader.
// It assumes the count is unknown or not cheaply available (-1).
// To provide a count, implement the DataLoader interface directly.
type DataLoaderFunc func(ctx context.Context) iter.Seq2[string, error]

// StreamValues implements DataLoader.
func (f DataLoaderFunc) StreamValues(ctx context.Context) iter.Seq2[string, error] {
	return f(ctx)
}

// Count implements DataLoader.
// It returns -1 to indicate that the count is unknown.
func (f DataLoaderFunc) Count(ctx context.Context) (int64, error) {
	return -1, nil
}

// DataLoaderOption defines functional options for DataLoader.
type DataLoaderOption func(*baseDataLoader)

// WithCount sets the total number of items for the DataLoader.
func WithCount(count int64) DataLoaderOption {
	return func(l *baseDataLoader) {
		l.count = count
	}
}

type baseDataLoader struct {
	stream func(context.Context) iter.Seq2[string, error]
	count  int64
}

func (l *baseDataLoader) StreamValues(ctx context.Context) iter.Seq2[string, error] {
	return l.stream(ctx)
}

func (l *baseDataLoader) Count(ctx context.Context) (int64, error) {
	return l.count, nil
}

// NewDataLoader creates a new DataLoader using an iterator and optional count.
func NewDataLoader(stream func() iter.Seq[string], opts ...DataLoaderOption) DataLoader {
	l := &baseDataLoader{
		stream: func(ctx context.Context) iter.Seq2[string, error] {
			return func(yield func(string, error) bool) {
				for v := range stream() {
					if !yield(v, nil) {
						return
					}
				}
			}
		},
		count: -1,
	}

	for _, opt := range opts {
		opt(l)
	}

	return l
}
