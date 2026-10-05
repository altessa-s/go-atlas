// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package facade

import (
	"context"
	"errors"
	"iter"

	"github.com/altessa-s/go-atlas/data/probfilter"
)

// Store is the part of a Bloom or Cuckoo storage the shared facade
// operations use: the live filter and its Close.
type Store interface {
	MightExist(ctx context.Context, value string) (bool, error)
	Add(ctx context.Context, value string) error
	AddBatch(ctx context.Context, values iter.Seq[string]) error
	Close(ctx context.Context) error
}

// Base implements the operations the Bloom and Cuckoo facades share: fenced
// lookups, journaled adds, coordinated deletes and rebuilds, observation and
// Close. It is safe for concurrent use and must not be copied.
type Base struct {
	store    Store
	coord    Coordinator
	observer ObserverSlot
}

// NewBase returns a Base serving the live filter of store.
func NewBase(store Store) *Base {
	return &Base{store: store}
}

// MightExist checks if a value might exist in the filter. It fails with an
// error wrapping [probfilter.ErrCommitIndeterminate] while the filter is
// fenced (see [Coordinator.Fence]).
func (b *Base) MightExist(ctx context.Context, value string) (bool, error) {
	return b.observer.Lookup(func() (bool, error) {
		for {
			if err := b.coord.Fence(ctx); err != nil {
				return false, err
			}
			found, err := b.store.MightExist(ctx, value)
			if !b.coord.Fenced() {
				return found, err
			}
			// A fence was published while the lookup ran: re-check it.
		}
	})
}

// Add inserts a value into the filter.
func (b *Base) Add(ctx context.Context, value string) error {
	err := b.coord.Add(ctx, value, func() error {
		return b.store.Add(ctx, value)
	})
	if err == nil {
		b.observer.Added(1)
	}
	return err
}

// AddBatch inserts multiple values into the filter and reports how many the
// store consumed.
func (b *Base) AddBatch(ctx context.Context, values iter.Seq[string]) error {
	n := 0
	err := b.coord.AddBatch(ctx, values, func(values iter.Seq[string]) error {
		return b.store.AddBatch(ctx, func(yield func(string) bool) {
			for v := range values {
				n++
				if !yield(v) {
					return
				}
			}
		})
	})
	if err == nil {
		b.observer.Added(n)
	}
	return err
}

// Delete runs the live delete del through [Coordinator.Delete].
func (b *Base) Delete(ctx context.Context, del func() (bool, error)) (bool, error) {
	return b.coord.Delete(ctx, del)
}

// Rebuild runs an observed [Coordinator.RebuildOrdered]; committed, when the
// rebuild succeeded, records it before the rebuild is reported.
func (b *Base) Rebuild(ctx context.Context, loader probfilter.DataLoader, begin BeginFunc, committed func()) error {
	return b.observer.Rebuild(func() error {
		if err := b.coord.RebuildOrdered(ctx, loader, begin); err != nil {
			return err
		}
		committed()
		return nil
	})
}

// Done returns a channel that is closed when the filter is closed.
func (b *Base) Done() <-chan struct{} {
	return b.coord.Done()
}

// SetObserver installs o; nil removes the current observer.
func (b *Base) SetObserver(o probfilter.Observer) {
	b.observer.Set(o)
}

// Close stops rebuilding (see [Coordinator.Close]) and closes the store.
func (b *Base) Close(ctx context.Context) error {
	return errors.Join(b.coord.Close(ctx), b.store.Close(ctx))
}

// StageWith adapts the Stage method of a storage or rebuild lease, which
// returns its own staging type, to a [StageFunc].
func StageWith[S Staging](stage func(context.Context, int64) (S, error)) StageFunc {
	return func(ctx context.Context, expectedItems int64) (Staging, error) {
		return stage(ctx, expectedItems)
	}
}
