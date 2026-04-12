// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package bloom_test

import (
	"context"
	"errors"
	"iter"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/probfilter"
	"github.com/altessa-s/go-atlas/data/probfilter/bloom"
	"github.com/altessa-s/go-atlas/data/probfilter/bloom/storages/memory"
)

func newTestFilter() *bloom.Filter {
	return bloom.New(memory.New(memory.WithExpectedItems(1000)))
}

func TestFilter_AddAndMightExist(t *testing.T) {
	f := newTestFilter()
	ctx := t.Context()

	require.NoError(t, f.Add(ctx, "hello"))

	exists, err := f.MightExist(ctx, "hello")
	require.NoError(t, err)
	require.True(t, exists, "MightExist(hello) = false after Add")
}

func TestFilter_MightExist_NotAdded(t *testing.T) {
	f := newTestFilter()
	ctx := t.Context()

	exists, err := f.MightExist(ctx, "not-added")
	require.NoError(t, err)
	require.False(t, exists, "MightExist(not-added) = true on empty filter")
}

func TestFilter_AddBatch(t *testing.T) {
	f := newTestFilter()
	ctx := t.Context()

	values := []string{"a", "b", "c"}
	require.NoError(t, f.AddBatch(ctx, slices.Values(values)))

	for _, v := range values {
		exists, err := f.MightExist(ctx, v)
		require.NoError(t, err)
		require.True(t, exists, "MightExist(%q) = false after AddBatch", v)
	}
}

func TestFilter_Stats(t *testing.T) {
	f := newTestFilter()
	ctx := t.Context()

	stats, err := f.Stats(ctx)
	require.NoError(t, err)
	require.NotNil(t, stats, "Stats() returned nil")
}

func TestFilter_Stats_NoProvider(t *testing.T) {
	// memory.Storage implements StatsProvider, so Stats returns real data.
	// Just verify it doesn't error.
	f := newTestFilter()
	ctx := t.Context()

	require.NoError(t, f.Add(ctx, "x"))

	stats, err := f.Stats(ctx)
	require.NoError(t, err)
	require.True(t, stats.ItemCount > 0, "ItemCount = %d, want > 0 after Add", stats.ItemCount)
}

type mockDataLoader struct {
	values []string
	count  int64
}

func (m *mockDataLoader) StreamValues(_ context.Context) iter.Seq2[string, error] {
	return func(yield func(string, error) bool) {
		for _, v := range m.values {
			if !yield(v, nil) {
				return
			}
		}
	}
}

func (m *mockDataLoader) Count(_ context.Context) (int64, error) {
	return m.count, nil
}

var _ probfilter.DataLoader = (*mockDataLoader)(nil)

func TestFilter_Rebuild_WithCount(t *testing.T) {
	f := newTestFilter()
	ctx := t.Context()

	loader := &mockDataLoader{values: []string{"x", "y", "z"}, count: 3}
	require.NoError(t, f.Rebuild(ctx, loader))

	for _, v := range loader.values {
		exists, err := f.MightExist(ctx, v)
		require.NoError(t, err)
		require.True(t, exists, "MightExist(%q) = false after Rebuild", v)
	}
}

func TestFilter_Rebuild_WithoutCount(t *testing.T) {
	f := newTestFilter()
	ctx := t.Context()

	loader := &mockDataLoader{values: []string{"a", "b"}, count: -1}
	require.NoError(t, f.Rebuild(ctx, loader))

	for _, v := range loader.values {
		exists, err := f.MightExist(ctx, v)
		require.NoError(t, err)
		require.True(t, exists, "MightExist(%q) = false after Rebuild", v)
	}
}

func TestFilter_LastRebuild(t *testing.T) {
	f := newTestFilter()
	ctx := t.Context()

	before := f.LastRebuild()
	require.True(t, before.IsZero(), "LastRebuild() = %v before rebuild, want zero", before)

	loader := &mockDataLoader{values: []string{"x"}, count: 1}
	now := time.Now()
	require.NoError(t, f.Rebuild(ctx, loader))

	after := f.LastRebuild()
	require.False(t, after.Before(now), "LastRebuild() = %v, want >= %v", after, now)
}

func TestFilter_Close(t *testing.T) {
	f := newTestFilter()
	require.NoError(t, f.Close(t.Context()))
}

// minimalStorage implements storages.Storage but NOT storages.StatsProvider.
type minimalStorage struct {
	data map[string]bool
}

func (m *minimalStorage) MightExist(_ context.Context, value string) (bool, error) {
	return m.data[value], nil
}
func (m *minimalStorage) Add(_ context.Context, value string) error {
	m.data[value] = true
	return nil
}
func (m *minimalStorage) AddBatch(_ context.Context, values iter.Seq[string]) error {
	for v := range values {
		m.data[v] = true
	}
	return nil
}
func (m *minimalStorage) Reset(_ context.Context, _ int64) error {
	m.data = make(map[string]bool)
	return nil
}
func (m *minimalStorage) LastRebuild() time.Time        { return time.Time{} }
func (m *minimalStorage) SetLastRebuild(_ time.Time)    {}
func (m *minimalStorage) Close(_ context.Context) error { return nil }

func TestFilter_Stats_NoStatsProvider(t *testing.T) {
	f := bloom.New(&minimalStorage{data: make(map[string]bool)})
	ctx := t.Context()

	stats, err := f.Stats(ctx)
	require.NoError(t, err)
	require.NotNil(t, stats, "Stats() returned nil")
	// Should return empty FilterStats when storage doesn't implement StatsProvider
	require.Equal(t, int64(0), stats.Capacity)
}

// errorDataLoader returns errors from StreamValues.
type errorDataLoader struct {
	streamErr error
	count     int64
}

func (e *errorDataLoader) StreamValues(_ context.Context) iter.Seq2[string, error] {
	return func(yield func(string, error) bool) {
		yield("", e.streamErr)
	}
}

func (e *errorDataLoader) Count(_ context.Context) (int64, error) {
	return e.count, nil
}

func TestFilter_Rebuild_StreamError_WithCount(t *testing.T) {
	f := newTestFilter()
	ctx := t.Context()

	loader := &errorDataLoader{streamErr: errors.New("stream failed"), count: 5}
	err := f.Rebuild(ctx, loader)
	require.Error(t, err, "Rebuild() should return error when StreamValues fails")
}

func TestFilter_Rebuild_StreamError_WithoutCount(t *testing.T) {
	f := newTestFilter()
	ctx := t.Context()

	loader := &errorDataLoader{streamErr: errors.New("stream failed"), count: -1}
	err := f.Rebuild(ctx, loader)
	require.Error(t, err, "Rebuild() should return error when StreamValues fails (no count)")
}
