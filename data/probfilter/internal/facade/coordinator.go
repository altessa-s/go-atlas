// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package facade

import (
	"context"
	"errors"
	"iter"
	"slices"
	"sync"
	"sync/atomic"

	"github.com/altessa-s/go-atlas/data/probfilter"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
)

// Staging is a replacement filter populated off to the side while the live
// filter keeps serving. It is used by a single rebuild goroutine.
type Staging interface {
	// AddBatch inserts values into the replacement filter.
	AddBatch(ctx context.Context, values iter.Seq[string]) error
	// Commit atomically replaces the live filter with the replacement. It
	// fails without swapping when ctx is already canceled; on any error the
	// live filter is unchanged.
	Commit(ctx context.Context) error
	// Abort discards the replacement; the live filter is unchanged.
	Abort(ctx context.Context) error
}

// StageFunc creates an empty [Staging] sized for expectedItems.
type StageFunc func(ctx context.Context, expectedItems int64) (Staging, error)

// BeginFunc starts a rebuild before its data source is read — for example by
// acquiring a shared filter's rebuild lease — and returns the stage function
// of the rebuild and a release function run when the rebuild ends.
type BeginFunc func(ctx context.Context) (stage StageFunc, release func(context.Context) error, err error)

// Coordinator makes a filter rebuild atomic with respect to concurrent adds.
// Adds routed through [Coordinator.Add] and [Coordinator.AddBatch] while a
// rebuild runs are journaled and replayed onto the replacement filter before
// it is committed, so no add made through the filter during a rebuild is
// lost. Deletes are deliberately not journaled: replaying a fingerprint
// delete onto a differently populated filter could remove a colliding member.
// [Coordinator.Delete] only keeps a delete from straddling the commit.
// [Coordinator.Close] stops rebuilding for good.
//
// The zero value is ready for use; a Coordinator must not be copied after
// first use.
type Coordinator struct {
	// rebuildMu serializes rebuilds and guards closed.
	rebuildMu sync.Mutex
	// closed is set by Close; later rebuilds return ErrFilterClosed.
	closed bool

	// done is closed by Close; doneOnce creates it lazily.
	doneOnce sync.Once
	done     chan struct{}

	// cancelMu guards closing and cancel, the cancel function of the
	// running rebuild, so Close can interrupt it.
	cancelMu sync.Mutex
	closing  bool
	cancel   context.CancelFunc

	// gate is held shared by every add and exclusively by a rebuild while it
	// switches journaling on or off and while it replays the journal tail
	// and commits.
	gate sync.RWMutex
	// active reports whether adds must be journaled. Written under the
	// exclusive gate, read under the shared gate.
	active bool

	// journalMu guards journal. While a rebuild is active an add holds it
	// across the live write and the journal append, so the journal order is
	// the order in which the live filter saw the adds.
	journalMu sync.Mutex
	journal   []string

	// unresolved is a replacement whose commit outcome is unknown and whose
	// abort failed: a delayed promotion of it could still replace the live
	// filter, so the filter is fenced until an abort of it succeeds. Written
	// under the exclusive gate; fenced mirrors unresolved != nil for a cheap
	// check on every operation.
	unresolved Staging
	fenced     atomic.Bool
}

// Add performs the live write of value. While a rebuild runs, value is also
// journaled — even when the live write fails, which keeps the rebuilt filter
// a superset of everything the live filter may hold.
func (c *Coordinator) Add(ctx context.Context, value string, write func() error) error {
	if err := c.admit(ctx); err != nil {
		return err
	}
	defer c.gate.RUnlock()

	if !c.active {
		return write()
	}

	c.journalMu.Lock()
	defer c.journalMu.Unlock()
	c.journal = append(c.journal, value)
	return write()
}

// AddBatch performs the live batch write. While a rebuild runs, every value
// the write consumes is journaled, including the consumed prefix of a batch
// that fails part way.
func (c *Coordinator) AddBatch(ctx context.Context, values iter.Seq[string], write func(iter.Seq[string]) error) error {
	if err := c.admit(ctx); err != nil {
		return err
	}
	defer c.gate.RUnlock()

	if !c.active {
		return write(values)
	}

	c.journalMu.Lock()
	defer c.journalMu.Unlock()
	return write(func(yield func(string) bool) {
		for v := range values {
			c.journal = append(c.journal, v)
			if !yield(v) {
				return
			}
		}
	})
}

// Delete performs the live delete while holding the shared gate, so the
// commit — which holds the gate exclusively — cannot swap filters while the
// delete is in flight: a delete runs entirely against one filter generation.
// Deletes are not journaled.
func (c *Coordinator) Delete(ctx context.Context, write func() (bool, error)) (bool, error) {
	if err := c.admit(ctx); err != nil {
		return false, err
	}
	defer c.gate.RUnlock()
	return write()
}

// admit acquires the shared gate for a write or delete, re-checking the fence
// while holding it: the fence is only published under the exclusive gate, so
// an operation that queued behind a commit whose outcome turned out unknown
// sees the fence and never touches the live filter. On success the caller
// must release the shared gate.
func (c *Coordinator) admit(ctx context.Context) error {
	for {
		c.gate.RLock()
		if !c.fenced.Load() {
			return nil
		}
		c.gate.RUnlock()
		if err := c.Fence(ctx); err != nil {
			return err
		}
	}
}

// Fenced reports whether the filter is fenced (see [Coordinator.Fence])
// without trying to lift the fence. Lookups call it after reading, so a
// lookup that overlapped the publication of a fence reports the error too.
func (c *Coordinator) Fenced() bool {
	return c.fenced.Load()
}

// Fence reports whether the filter may be used. After a rebuild whose commit
// outcome is unknown ([probfilter.ErrCommitIndeterminate]) and whose staging
// filter could not be discarded, a delayed promotion could still replace the
// live filter and lose writes made in the meantime. Until discarding the
// staging filter succeeds — Fence retries it on every call — Fence returns an
// error wrapping ErrCommitIndeterminate and the facades refuse lookups and
// writes. Without such a rebuild Fence is a single atomic load.
func (c *Coordinator) Fence(ctx context.Context) error {
	if !c.fenced.Load() {
		return nil
	}

	c.gate.Lock()
	defer c.gate.Unlock()
	if c.unresolved == nil {
		return nil
	}
	if err := c.unresolved.Abort(context.WithoutCancel(ctx)); err != nil {
		return coreerrs.WrapOperation(errors.Join(probfilter.ErrCommitIndeterminate, err), "discard unresolved rebuilt filter")
	}
	c.unresolved = nil
	c.fenced.Store(false)
	return nil
}

// Close stops rebuilding for good: it cancels a running rebuild, waits for it
// to return, and makes every later Rebuild return [probfilter.ErrFilterClosed].
// No commit happens after Close returns, unless Close returns an error: then a
// rebuilt filter whose commit outcome is unknown could not be discarded (see
// [Coordinator.Fence]). Close is idempotent.
func (c *Coordinator) Close(ctx context.Context) error {
	c.cancelMu.Lock()
	c.closing = true
	if c.cancel != nil {
		c.cancel()
	}
	c.cancelMu.Unlock()

	c.rebuildMu.Lock()
	c.closed = true
	if ch := c.doneChan(); !isClosed(ch) {
		close(ch)
	}
	c.rebuildMu.Unlock()

	return c.Fence(ctx)
}

// Done returns a channel that is closed once [Coordinator.Close] has
// stopped rebuilding, so owners of rebuild schedules can release them.
func (c *Coordinator) Done() <-chan struct{} {
	return c.doneChan()
}

func (c *Coordinator) doneChan() chan struct{} {
	c.doneOnce.Do(func() { c.done = make(chan struct{}) })
	return c.done
}

func isClosed(ch chan struct{}) bool {
	select {
	case <-ch:
		return true
	default:
		return false
	}
}

// Rebuild builds a replacement filter from loader through stage and commits
// it atomically. Lookups keep hitting the live filter until the commit. On any
// error — including a ctx canceled while a loader stopped without reporting
// it — the replacement is aborted and the live filter is unchanged. Adds made
// through the Coordinator while Rebuild runs are replayed onto the
// replacement before the commit. After [Coordinator.Close], Rebuild returns
// [probfilter.ErrFilterClosed]; a rebuild interrupted by Close returns an
// error wrapping it.
func (c *Coordinator) Rebuild(ctx context.Context, loader probfilter.DataLoader, stage StageFunc) error {
	return c.RebuildOrdered(ctx, loader, func(context.Context) (StageFunc, func(context.Context) error, error) {
		return stage, nil, nil
	})
}

// RebuildOrdered is [Coordinator.Rebuild] with a begin step: begin runs after
// the rebuild was admitted and before the loader is consulted, and its
// release function (if any) runs when the rebuild ends, whatever the outcome.
func (c *Coordinator) RebuildOrdered(ctx context.Context, loader probfilter.DataLoader, begin BeginFunc) error {
	c.rebuildMu.Lock()
	defer c.rebuildMu.Unlock()

	ctx, release, err := c.admitRebuild(ctx)
	if err != nil {
		return err
	}
	defer release()

	if err = c.Fence(ctx); err != nil {
		return err
	}

	err = c.rebuild(ctx, loader, begin)
	if err != nil && c.isClosing() {
		return errors.Join(probfilter.ErrFilterClosed, err)
	}
	return err
}

// admitRebuild registers a cancelable context for the rebuild, refusing when
// the coordinator is closed or closing. The caller holds rebuildMu.
func (c *Coordinator) admitRebuild(ctx context.Context) (context.Context, func(), error) {
	if c.closed {
		return nil, nil, probfilter.ErrFilterClosed
	}

	c.cancelMu.Lock()
	defer c.cancelMu.Unlock()
	if c.closing {
		return nil, nil, probfilter.ErrFilterClosed
	}
	ctx, cancel := context.WithCancel(ctx)
	c.cancel = cancel
	return ctx, func() {
		c.cancelMu.Lock()
		c.cancel = nil
		c.cancelMu.Unlock()
		cancel()
	}, nil
}

func (c *Coordinator) isClosing() bool {
	c.cancelMu.Lock()
	defer c.cancelMu.Unlock()
	return c.closing
}

// rebuild runs one admitted rebuild. The caller holds rebuildMu.
func (c *Coordinator) rebuild(ctx context.Context, loader probfilter.DataLoader, begin BeginFunc) error {
	if err := ctx.Err(); err != nil {
		return coreerrs.WrapOperation(err, "rebuild filter")
	}

	c.setActive(true)
	committed := false
	defer func() {
		if !committed {
			c.setActive(false)
		}
	}()

	stage, release, err := begin(ctx)
	if err != nil {
		return err
	}
	if release != nil {
		// A release failure is harmless: a lease expires on its own.
		defer func() { _ = release(context.WithoutCancel(ctx)) }()
	}

	st, err := c.load(ctx, loader, stage)
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return abort(ctx, st, coreerrs.WrapOperation(err, "load values during rebuild"))
	}

	// Replay what accumulated during the load without blocking writers, so
	// the exclusive section below only replays the short tail.
	if err := replay(ctx, st, c.drain()); err != nil {
		return abort(ctx, st, err)
	}

	c.gate.Lock()
	defer c.gate.Unlock()

	if err := replay(ctx, st, c.drain()); err != nil {
		return abort(ctx, st, err)
	}
	if err := ctx.Err(); err != nil {
		return abort(ctx, st, coreerrs.WrapOperation(err, "commit rebuilt filter"))
	}
	if err := st.Commit(ctx); err != nil {
		err = coreerrs.WrapOperation(err, "commit rebuilt filter")
		if !errors.Is(err, probfilter.ErrCommitIndeterminate) {
			return abort(ctx, st, err)
		}
		// The commit may still be promoted later; discarding the replacement
		// makes that harmless. If even that fails, fence the filter.
		if abortErr := st.Abort(context.WithoutCancel(ctx)); abortErr != nil {
			c.unresolved = st
			c.fenced.Store(true)
			return errors.Join(err, coreerrs.WrapOperation(abortErr, "abort rebuilt filter"))
		}
		return err
	}

	c.active = false
	committed = true
	return nil
}

// load creates the replacement filter and streams loader into it.
func (c *Coordinator) load(ctx context.Context, loader probfilter.DataLoader, stage StageFunc) (Staging, error) {
	if count, countErr := loader.Count(ctx); countErr == nil && count > 0 {
		st, err := stage(ctx, count)
		if err != nil {
			return nil, coreerrs.WrapOperation(err, "stage rebuilt filter")
		}

		var loadErr error
		values := func(yield func(string) bool) {
			for v, err := range loader.StreamValues(ctx) {
				if err != nil {
					loadErr = err
					return
				}
				if !yield(v) {
					return
				}
			}
		}
		if err := st.AddBatch(ctx, values); err != nil {
			return nil, abort(ctx, st, coreerrs.WrapOperation(err, "add value during rebuild"))
		}
		if loadErr != nil {
			return nil, abort(ctx, st, coreerrs.WrapOperation(loadErr, "load value during rebuild"))
		}
		return st, nil
	}

	// Count unknown: collect first, since many sources (database cursors,
	// SCAN) can only be iterated once and the replacement must be sized.
	var values []string
	for v, err := range loader.StreamValues(ctx) {
		if err != nil {
			return nil, coreerrs.WrapOperation(err, "load value during rebuild")
		}
		values = append(values, v)
	}

	st, err := stage(ctx, int64(len(values)))
	if err != nil {
		return nil, coreerrs.WrapOperation(err, "stage rebuilt filter")
	}
	if err := st.AddBatch(ctx, slices.Values(values)); err != nil {
		return nil, abort(ctx, st, coreerrs.WrapOperation(err, "add value during rebuild"))
	}
	return st, nil
}

// setActive switches journaling on or off and drops any journal.
func (c *Coordinator) setActive(active bool) {
	c.gate.Lock()
	defer c.gate.Unlock()
	c.active = active
	c.journalMu.Lock()
	c.journal = nil
	c.journalMu.Unlock()
}

// drain takes the journal accumulated so far.
func (c *Coordinator) drain() []string {
	c.journalMu.Lock()
	defer c.journalMu.Unlock()
	values := c.journal
	c.journal = nil
	return values
}

// replay adds journaled values to st in journal order.
func replay(ctx context.Context, st Staging, values []string) error {
	if len(values) == 0 {
		return nil
	}
	if err := st.AddBatch(ctx, slices.Values(values)); err != nil {
		return coreerrs.WrapOperation(err, "replay adds during rebuild")
	}
	return nil
}

// abort discards st and returns cause. The abort runs on a context detached
// from cancellation so a canceled rebuild still cleans up its replacement.
func abort(ctx context.Context, st Staging, cause error) error {
	if err := st.Abort(context.WithoutCancel(ctx)); err != nil {
		return errors.Join(cause, coreerrs.WrapOperation(err, "abort rebuilt filter"))
	}
	return cause
}
