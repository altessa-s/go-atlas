// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package outbox

import (
	"context"
	"log/slog"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
)

// Watcher is an optional [Store] capability: a store that can push a signal the
// moment new events land, instead of making the dispatcher wait for its next
// poll tick. A store that cannot do this simply does not implement Watcher, and
// [Outbox.Watch] reports [ErrWatchUnsupported].
//
// See [github.com/altessa-s/go-atlas/data/outbox/store/mongo] for the MongoDB
// change-stream implementation.
type Watcher interface {
	// Watch blocks until ctx is done, calling notify whenever newly saved
	// events may be ready to dispatch. A clean shutdown returns nil.
	//
	// notify is a hint, never a delivery: it carries no payload, may collapse
	// any number of events into a single call, and may fire spuriously. The
	// dispatch cycle remains the source of truth about what is actually due.
	//
	// notify is never nil, is safe to call from the goroutine running Watch,
	// and never blocks — implementations may call it once per change event
	// without buffering or rate-limiting first.
	Watch(ctx context.Context, notify func()) error
}

// Watch drives the dispatch cycle from store notifications instead of waiting
// for the next scheduled poll, which cuts the delay between saving an event and
// publishing it from "up to one poll interval" down to "as fast as the store
// can tell us". It blocks until ctx is done and is meant to run in its own
// goroutine:
//
//	go func() {
//	    switch err := ob.Watch(ctx); {
//	    case err == nil, errors.Is(err, outbox.ErrWatchUnsupported):
//	        // Nothing to do: dispatch continues on the scheduled poll cycle.
//	    default:
//	        logger.Error("outbox watcher stopped", slog.Any("error", err))
//	    }
//	}()
//
// This is an optimization, not a replacement for the dispatch schedule. A
// notification says "something arrived", never "this event is due": a retry
// backoff elapsing and the unlock sweeper reclaiming a stuck lease are both
// time-driven and produce no notification at all, and a notification that
// arrives while a cycle is already running is dropped rather than queued. Keep
// [WithDispatchSchedule] configured — Watch only shortens the common case.
//
// Returns [ErrWatchUnsupported] when the store cannot push notifications, and
// nil when ctx ends.
func (o *Outbox) Watch(ctx context.Context) error {
	watcher, ok := o.store.(Watcher)
	if !ok {
		return ErrWatchUnsupported
	}

	// Capacity one with a non-blocking send: a bulk insert reports thousands of
	// changes that all mean the same thing — "fetch again when you can" — so
	// collapsing them into a single pending wake-up is what keeps the watcher
	// from queueing one cycle per document.
	signal := make(chan struct{}, 1)
	wake := func() {
		select {
		case signal <- struct{}{}:
		default:
		}
	}
	notify := func() {
		o.metrics.watchNotifications.Inc()
		wake()
	}

	// Canceling on the way out stops the store's stream even when we leave
	// because it failed rather than because ctx ended.
	watchCtx, stopWatch := context.WithCancel(ctx)
	defer stopWatch()

	// Buffered so the goroutine can finish and exit even if we return first.
	done := make(chan error, 1)
	go func() { done <- watcher.Watch(watchCtx, notify) }()

	for {
		select {
		case err := <-done:
			if coreerrs.IsContextCanceled(err) {
				return nil // Our own shutdown, reported by the store as cancellation.
			}
			return err
		case <-signal:
			o.dispatchNotified(ctx, wake)
		}
	}
}

// dispatchNotified runs one notification-driven dispatch cycle.
//
// A cycle that came back with a full batch means the store held more work than
// one batch, and nothing further will arrive to say so — the watcher would fall
// silent with the backlog intact, leaving the remainder to the next poll tick.
// Re-arming the signal keeps the drain going until a cycle comes back short.
func (o *Outbox) dispatchNotified(ctx context.Context, wake func()) {
	var fetched int
	err := o.dispatchTask.TryRun(ctx, func(cycleCtx context.Context) error {
		var cycleErr error
		fetched, cycleErr = o.dispatchOnce(cycleCtx)
		return cycleErr
	})
	if err != nil {
		o.logger.ErrorContext(ctx, "notification-driven dispatch cycle failed", slog.Any("error", err))
	}

	if fetched >= int(o.eventsBatchSize) {
		wake()
	}
}
