// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package cuckoo

import (
	"context"
	"iter"
	"sync/atomic"
	"time"

	"github.com/altessa-s/go-atlas/data/probfilter"
	"github.com/altessa-s/go-atlas/data/probfilter/cuckoo/storages"
	"github.com/altessa-s/go-atlas/data/probfilter/internal/facade"
)

// Filter is a Cuckoo filter facade that wraps a storage backend.
// It implements probfilter.DeletableFilter, probfilter.RebuildableFilter and
// probfilter.ObservableFilter.
type Filter struct {
	storage     storages.Storage
	base        *facade.Base
	lastRebuild atomic.Int64 // UnixNano of the last successful rebuild; 0 = never.
}

var (
	_ probfilter.Filter                = (*Filter)(nil)
	_ probfilter.DeletableFilter       = (*Filter)(nil)
	_ probfilter.RebuildableFilter     = (*Filter)(nil)
	_ probfilter.ObservableFilter      = (*Filter)(nil)
	_ probfilter.StatsProvider         = (*Filter)(nil)
	_ probfilter.RebuildCommitReporter = (*Filter)(nil)
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

// Delete removes a value from the filter.
//
// A delete never straddles the commit of a [Filter.Rebuild]: it runs entirely
// against the filter that is live when it starts. A delete made while a
// rebuild runs applies to the live filter only; it is not replayed onto the
// replacement, because removing a fingerprint
// from a differently populated filter could remove another member. If the
// rebuilt contents contain the value it is reported as possibly present again
// (a false positive, never a false negative).
func (f *Filter) Delete(ctx context.Context, value string) (bool, error) {
	return f.base.Delete(ctx, func() (bool, error) {
		return f.storage.Delete(ctx, value)
	})
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
		fs, err := sp.Stats(ctx)
		if err != nil {
			return nil, err
		}
		fs.LastRebuild = f.LastRebuild()
		return fs, nil
	}
	return &probfilter.FilterStats{LastRebuild: f.LastRebuild()}, nil
}

// Close releases resources associated with the filter. It interrupts a
// running [Filter.Rebuild] and waits for it to return; afterwards Rebuild
// returns [probfilter.ErrFilterClosed], so no rebuild commits after Close.
func (f *Filter) Close(ctx context.Context) error {
	return f.base.Close(ctx)
}

// Rebuild recreates the filter from scratch using the provided data loader,
// dropping fingerprints of values that were deleted from the source but never
// from the filter.
//
// The rebuild is atomic: the replacement is populated off to the side (a
// fresh in-process filter, or a staging key renamed onto the live key for
// Redis) while lookups keep seeing the previous contents, and it replaces them
// in one step. A failed or canceled rebuild — including a replacement that
// runs full — leaves the previous contents and [Filter.LastRebuild] unchanged.
// Values added through this filter while the rebuild runs are journaled and
// replayed onto the replacement; deletes are not (see [Filter.Delete]). While
// a rebuild runs, adds are serialized. Writes made to a shared Redis filter by
// other processes during the rebuild are lost when the replacement is
// committed.
//
// A Redis commit whose outcome cannot be established returns an error wrapping
// [probfilter.ErrCommitIndeterminate] and, if the replacement cannot be
// discarded, fences the filter like [bloom.Filter.Rebuild] does.
//
// The replacement has room for the loaded count plus 25% headroom, and never
// less than the configured capacity. Concurrent rebuilds are serialized; for
// a shared Redis filter across processes too, like [bloom.Filter.Rebuild].
func (f *Filter) Rebuild(ctx context.Context, loader probfilter.DataLoader) error {
	return f.base.Rebuild(ctx, loader, f.beginRebuild, func() {
		f.lastRebuild.Store(time.Now().UnixNano())
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

// LastRebuild returns the time of the last successful rebuild, or the zero
// time if the filter has never been rebuilt.
func (f *Filter) LastRebuild() time.Time {
	if ns := f.lastRebuild.Load(); ns != 0 {
		return time.Unix(0, ns)
	}
	return time.Time{}
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
