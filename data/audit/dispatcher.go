// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package audit

import (
	"context"
	"log/slog"
	"sync"
	"time"

	coretime "github.com/altessa-s/go-atlas/core/time"
)

// dispatcher handles asynchronous event processing with batching and retry.
type dispatcher struct {
	opts        *options
	storage     Storage
	metrics     *auditMetrics
	events      chan *Event
	done        chan struct{}
	shutdownCtx context.Context
	shutdownFn  context.CancelFunc
	wg          sync.WaitGroup
}

func newDispatcher(storage Storage, opts *options, metrics *auditMetrics) *dispatcher {
	ctx, cancel := context.WithCancel(context.Background())
	return &dispatcher{
		opts:        opts,
		storage:     storage,
		metrics:     metrics,
		events:      make(chan *Event, opts.bufferSize),
		done:        make(chan struct{}),
		shutdownCtx: ctx,
		shutdownFn:  cancel,
	}
}

// emit sends an event to the buffer. Returns false if the buffer is full
// (or shutdown was signaled while blocking in back-pressure mode).
func (d *dispatcher) emit(event *Event) bool {
	if d.opts.backPressure {
		select {
		case d.events <- event:
			return true
		case <-d.done:
			return false
		}
	}
	select {
	case d.events <- event:
		return true
	default:
		return false
	}
}

// start launches worker goroutines.
func (d *dispatcher) start() {
	for range d.opts.workers {
		d.wg.Go(d.worker)
	}
}

// shutdown drains the buffer and waits for workers to finish.
func (d *dispatcher) shutdown(ctx context.Context) error {
	close(d.done)
	d.shutdownFn()

	finished := make(chan struct{})
	go func() {
		d.wg.Wait()
		close(finished)
	}()

	select {
	case <-finished:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (d *dispatcher) worker() {
	d.metrics.workersActive.Inc()
	defer d.metrics.workersActive.Dec()

	batch := make([]*Event, 0, d.opts.batchSize)
	ticker := time.NewTicker(d.opts.flushInterval)
	defer ticker.Stop()

	flush := func() {
		if len(batch) == 0 {
			return
		}
		d.storeBatch(batch)
		batch = batch[:0]
	}

	for {
		select {
		case event, ok := <-d.events:
			if !ok {
				flush()
				return
			}
			batch = append(batch, event)
			if len(batch) >= d.opts.batchSize {
				flush()
			}

		case <-ticker.C:
			flush()

		case <-d.done:
			// Drain remaining events from channel.
			for {
				select {
				case event, ok := <-d.events:
					if !ok {
						flush()
						return
					}
					batch = append(batch, event)
					if len(batch) >= d.opts.batchSize {
						flush()
					}
				default:
					flush()
					return
				}
			}
		}
	}
}

func (d *dispatcher) storeBatch(events []*Event) {
	stop := d.metrics.flushDuration.Start()
	defer stop()

	// Copy the slice to avoid races during retry.
	batch := make([]*Event, len(events))
	copy(batch, events)

	// Create a context that is canceled when the dispatcher shuts down,
	// so storage calls do not hang after the done signal.
	// context.AfterFunc registers the callback without spawning a goroutine upfront;
	// the runtime invokes cancel() only when done is signaled.
	ctx, cancel := context.WithCancel(context.Background())
	ctxStop := context.AfterFunc(d.shutdownCtx, cancel)
	defer ctxStop()
	defer cancel()

	for attempt := range d.opts.retryAttempts + 1 {
		var err error
		if len(batch) == 1 {
			err = d.storage.Store(ctx, batch[0])
		} else {
			err = d.storage.StoreBatch(ctx, batch)
		}

		if err == nil {
			return
		}

		if attempt < d.opts.retryAttempts {
			backoff := d.opts.retryBackoff << uint(attempt)
			d.opts.logger.Warn("audit batch store failed, retrying",
				slog.Int("attempt", attempt+1),
				slog.Int("events", len(batch)),
				slog.Duration("backoff", backoff),
				slog.Any("error", err))

			timer := time.NewTimer(backoff)
			select {
			case <-timer.C:
			case <-d.done:
				coretime.TimerStopAndDrain(timer)
				// Last attempt before shutdown.
				if len(batch) == 1 {
					_ = d.storage.Store(ctx, batch[0]) //nolint:errcheck // best-effort during shutdown
				} else {
					_ = d.storage.StoreBatch(ctx, batch) //nolint:errcheck // best-effort during shutdown
				}
				return
			}
		} else {
			d.metrics.storeErrors.Inc()
			d.opts.logger.Error("audit batch store failed after all retries",
				slog.Int("events", len(batch)),
				slog.Any("error", err))
		}
	}
}
