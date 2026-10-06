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
	"github.com/altessa-s/go-atlas/data/probfilter/internal/facade"
)

// Filter is a Bloom filter facade that wraps a storage backend.
// It implements probfilter.RebuildableFilter and probfilter.ObservableFilter.
type Filter struct {
	storage storages.Storage
	base    *facade.Base
}

var (
	_ probfilter.Filter                = (*Filter)(nil)
	_ probfilter.RebuildableFilter     = (*Filter)(nil)
	_ probfilter.ObservableFilter      = (*Filter)(nil)
	_ probfilter.StatsProvider         = (*Filter)(nil)
	_ probfilter.RebuildCommitReporter = (*Filter)(nil)
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
		base:    facade.NewBase(storage),
	}
}

// MightExist checks if a value might exist in the filter. It fails with an
// error wrapping [probfilter.ErrCommitIndeterminate] while the filter is
// fenced after an unresolved rebuild commit (see [Filter.Rebuild]).
func (f *Filter) MightExist(ctx context.Context, value string) (bool, error) {
	return f.base.MightExist(ctx, value)
}

// Add inserts a value into the filter.
func (f *Filter) Add(ctx context.Context, value string) error {
	return f.base.Add(ctx, value)
}

// AddBatch inserts multiple values into the filter.
func (f *Filter) AddBatch(ctx context.Context, values iter.Seq[string]) error {
	return f.base.AddBatch(ctx, values)
}

// Done returns a channel that is closed when the filter is closed, so the
// owner of a rebuild schedule (such as the probfilter factory's local cron)
// can release it.
func (f *Filter) Done() <-chan struct{} {
	return f.base.Done()
}

// SetObserver installs o to receive lookup, add and rebuild outcomes; nil
// removes the current observer. [probfilter.Manager.Register] calls it.
func (f *Filter) SetObserver(o probfilter.Observer) {
	f.base.SetObserver(o)
}

// Stats returns current filter statistics.
func (f *Filter) Stats(ctx context.Context) (*probfilter.FilterStats, error) {
	if sp, ok := f.storage.(storages.StatsProvider); ok {
		return sp.Stats(ctx)
	}
	return &probfilter.FilterStats{}, nil
}

// Close releases resources associated with the filter. It interrupts a
// running [Filter.Rebuild] and waits for it to return; afterwards Rebuild
// returns [probfilter.ErrFilterClosed], so no rebuild commits after Close.
func (f *Filter) Close(ctx context.Context) error {
	return f.base.Close(ctx)
}

// Rebuild recreates the filter from scratch using the provided data loader.
//
// The rebuild is atomic: the replacement is populated off to the side (a
// fresh in-process filter, or a staging key renamed onto the live key for
// Redis) while lookups keep seeing the previous contents, and it replaces them
// in one step. A failed or canceled rebuild leaves the previous contents and
// [Filter.LastRebuild] unchanged. Values added through this filter while the
// rebuild runs are journaled and replayed onto the replacement, so they are
// present afterwards; while a rebuild runs, adds are serialized. A shared Redis
// filter also journals, in Redis, the adds of every other process made under
// the rebuild lease, and the commit replays them, so they survive too.
//
// A Redis commit whose outcome cannot be established returns an error wrapping
// [probfilter.ErrCommitIndeterminate]: either contents may be live. If the
// replacement then cannot be discarded either, a delayed promotion could
// still swap it in, so the filter is fenced — lookups and writes fail with
// ErrCommitIndeterminate — until discarding it succeeds (retried on every
// call).
//
// When loader.Count reports a positive count, the replacement is sized for it
// and values are streamed into it; otherwise values are collected in memory
// first. Concurrent rebuilds of one filter are serialized; rebuilds of a
// shared Redis filter by several processes are serialized by a rebuild lease
// taken before the loader runs: a concurrent rebuild returns an error
// wrapping [probfilter.ErrRebuildInProgress], and one that lost its lease
// returns [probfilter.ErrRebuildSuperseded] without publishing.
func (f *Filter) Rebuild(ctx context.Context, loader probfilter.DataLoader) error {
	return f.base.Rebuild(ctx, loader, f.beginRebuild, func() {
		f.storage.SetLastRebuild(time.Now())
	})
}

// beginRebuild starts a rebuild before the loader runs: a shared storage
// ([storages.ExclusiveRebuilder]) first acquires its rebuild lease, so
// rebuilds by several processes are serialized; other storages stage directly.
func (f *Filter) beginRebuild(ctx context.Context) (facade.StageFunc, func(context.Context) error, error) {
	exclusive, ok := f.storage.(storages.ExclusiveRebuilder)
	if !ok {
		return facade.StageWith(f.storage.Stage), nil, nil
	}
	lease, err := exclusive.BeginRebuild(ctx)
	if err != nil {
		return nil, nil, err
	}
	return facade.StageWith(lease.Stage), lease.Release, nil
}

// LastRebuild returns the time of the last successful rebuild.
func (f *Filter) LastRebuild() time.Time {
	return f.storage.LastRebuild()
}

// RebuildCommitted implements [probfilter.RebuildCommitReporter]. A shared
// storage ([storages.RebuildCommitReporter], Redis) reports rebuilds
// committed by any process; otherwise only a successful [Filter.Rebuild] of
// this filter counts.
func (f *Filter) RebuildCommitted(ctx context.Context) (bool, error) {
	if reporter, ok := f.storage.(storages.RebuildCommitReporter); ok {
		return reporter.RebuildCommitted(ctx)
	}
	return !f.LastRebuild().IsZero(), nil
}
