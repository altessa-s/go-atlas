// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package dispatch

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/altessa-s/go-atlas/core/io/wal"

	coretime "github.com/altessa-s/go-atlas/core/time"
)

// Engine errors.
var (
	// ErrEngineClosed is returned by Submit after Shutdown.
	ErrEngineClosed = errors.New("dispatch: engine closed")
	// ErrAlreadyStarted is returned by Start when the engine has already started.
	ErrAlreadyStarted = errors.New("dispatch: already started")
	// ErrCodecRequired is returned when WAL is enabled but no codec is set.
	ErrCodecRequired = errors.New("dispatch: codec required when WAL enabled")
	// ErrSinkRequired is returned by NewEngine when sink is nil.
	ErrSinkRequired = errors.New("dispatch: sink required")
)

// Engine is a generic, non-blocking, batching async dispatcher with optional
// WAL-backed crash safety. See package documentation for details.
type Engine[T any] struct {
	opts    *options[T]
	sink    Sink[T]
	codec   Codec[T]
	metrics *engineMetrics

	log *wal.WAL // nil when WAL disabled

	submitted      chan envelope[T]
	done           chan struct{}
	shutdownCtx    context.Context
	shutdownCancel context.CancelFunc

	started atomic.Bool
	closed  atomic.Bool
	dropped atomic.Int64

	wg sync.WaitGroup
}

type envelope[T any] struct {
	item   T
	offset wal.Offset // zero when WAL disabled
}

// NewEngine creates an Engine with the given Sink and options.
func NewEngine[T any](sink Sink[T], opts ...Option[T]) (*Engine[T], error) {
	if sink == nil {
		return nil, ErrSinkRequired
	}
	o := newOptions(opts...)

	if o.walEnabled && o.codec == nil {
		return nil, ErrCodecRequired
	}

	return &Engine[T]{
		opts:      o,
		sink:      sink,
		codec:     o.codec,
		metrics:   newEngineMetrics(o.collector, o.metricsSubsystem),
		submitted: make(chan envelope[T], o.bufferSize),
		done:      make(chan struct{}),
	}, nil
}

// Start opens the WAL (if configured), replays any unflushed records by
// pushing them through the worker queue, and launches the worker goroutines.
// It is an error to call Start more than once.
func (e *Engine[T]) Start() error {
	if !e.started.CompareAndSwap(false, true) {
		return ErrAlreadyStarted
	}
	e.shutdownCtx, e.shutdownCancel = context.WithCancel(context.Background())

	if e.opts.walEnabled {
		w, recovered, err := wal.Open(e.opts.walDir, e.opts.walOpts...)
		if err != nil {
			return err
		}
		e.log = w

		// Start workers first so the channel can drain while we replay.
		for range e.opts.workers {
			e.wg.Go(e.worker)
		}

		if len(recovered) > 0 {
			e.metrics.replayCount.Add(float64(len(recovered)))
			e.opts.logger.Info("dispatch: replaying WAL records",
				slog.Int("count", len(recovered)))
		}
		for _, r := range recovered {
			item, decErr := e.codec.Decode(r.Payload)
			if decErr != nil {
				e.opts.logger.Error("dispatch: decode replay record failed",
					slog.Any("error", decErr))
				e.log.Ack(r.Offset)
				continue
			}
			// Block-fill: replay must complete before normal Submit, but
			// workers are draining concurrently so this won't deadlock.
			select {
			case e.submitted <- envelope[T]{item: item, offset: r.Offset}:
			case <-e.done:
				return nil
			}
		}
		return nil
	}

	for range e.opts.workers {
		e.wg.Go(e.worker)
	}
	return nil
}

// Submit enqueues item for asynchronous delivery to Sink. The hot path is
// non-blocking: it serializes (only with WAL), appends to the WAL page
// cache (only with WAL), and sends on a buffered channel. It returns false
// if the buffer is full and back-pressure is disabled, or if the engine
// is not running.
func (e *Engine[T]) Submit(item T) bool {
	if !e.started.Load() || e.closed.Load() {
		return false
	}

	var off wal.Offset
	if e.log != nil {
		payload, err := e.codec.Encode(item)
		if err != nil {
			e.metrics.encodeErrors.Inc()
			e.opts.logger.Error("dispatch: encode failed", slog.Any("error", err))
			// Encode failure is a drop: the item never reaches the WAL or
			// the channel, so surface it through the same counter and
			// OnDrop callback as a full-buffer drop.
			return e.dropItem(item)
		}
		off, err = e.log.Append(payload)
		if err != nil {
			e.metrics.walErrors.Inc()
			e.opts.logger.Warn("dispatch: WAL append failed",
				slog.Any("error", err))
			return e.dropItem(item)
		}
		e.metrics.walBytes.Set(float64(e.log.Stats().TotalBytes))
	}

	env := envelope[T]{item: item, offset: off}
	if e.opts.backPressure {
		select {
		case e.submitted <- env:
			e.metrics.enqueued.Inc()
			return true
		case <-e.done:
			return false
		}
	}
	select {
	case e.submitted <- env:
		e.metrics.enqueued.Inc()
		return true
	default:
		// In-memory drop. The WAL record (if any) remains on disk and
		// will be replayed on next start.
		return e.dropItem(item)
	}
}

func (e *Engine[T]) dropItem(item T) bool {
	e.dropped.Add(1)
	e.metrics.dropped.Inc()
	if e.opts.onDrop != nil {
		e.opts.onDrop(item)
	}
	return false
}

// Dropped returns the total number of items dropped at submit time due to
// a full buffer. It does not include WAL append errors.
func (e *Engine[T]) Dropped() int64 { return e.dropped.Load() }

// WAL returns the underlying WAL handle, or nil when WAL is disabled.
// Intended for metrics scrapes and crash-recovery diagnostics.
func (e *Engine[T]) WAL() *wal.WAL { return e.log }

// Shutdown drains pending items and waits for workers to finish, bounded
// by ctx. Sealed-but-unacked WAL segments remain on disk for replay on the
// next Start. Calling Shutdown more than once is a no-op and returns nil.
//
// The returned error aggregates (via [errors.Join]) the ctx cancellation
// reason — if the deadline expired before workers drained — and any error
// from closing the WAL. A durability primitive must not silently drop
// either signal: the caller needs both to decide whether to retry or
// escalate.
func (e *Engine[T]) Shutdown(ctx context.Context) error {
	if !e.closed.CompareAndSwap(false, true) {
		return nil
	}
	close(e.done)

	finished := make(chan struct{})
	go func() {
		e.wg.Wait()
		close(finished)
	}()

	var errs []error
	select {
	case <-finished:
	case <-ctx.Done():
		e.shutdownCancel()
		<-finished
		errs = append(errs, ctx.Err())
	}
	e.shutdownCancel()

	if e.log != nil {
		if err := e.log.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (e *Engine[T]) worker() {
	e.metrics.workersActive.Inc()
	defer e.metrics.workersActive.Dec()

	batch := make([]T, 0, e.opts.batchSize)
	offsets := make([]wal.Offset, 0, e.opts.batchSize)
	ticker := time.NewTicker(e.opts.flushInterval)
	defer ticker.Stop()

	flush := func() {
		if len(batch) == 0 {
			return
		}
		e.storeBatch(batch, offsets)
		batch = batch[:0]
		offsets = offsets[:0]
	}

	for {
		select {
		case env, ok := <-e.submitted:
			if !ok {
				flush()
				return
			}
			batch = append(batch, env.item)
			offsets = append(offsets, env.offset)
			if len(batch) >= e.opts.batchSize {
				flush()
			}

		case <-ticker.C:
			flush()

		case <-e.done:
			// Drain remaining items synchronously.
			for {
				select {
				case env, ok := <-e.submitted:
					if !ok {
						flush()
						return
					}
					batch = append(batch, env.item)
					offsets = append(offsets, env.offset)
					if len(batch) >= e.opts.batchSize {
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

// storeBatch pushes items to the sink with retry. The caller (a worker
// goroutine) blocks synchronously until this returns, so items/offsets are
// not mutated by anyone else during the call and no defensive copy is needed.
func (e *Engine[T]) storeBatch(items []T, offsets []wal.Offset) {
	stop := e.metrics.flushDuration.Start()
	defer stop()

	ctx, cancel := context.WithCancel(context.Background())
	ctxStop := context.AfterFunc(e.shutdownCtx, cancel)
	defer ctxStop()
	defer cancel()

	ack := func() {
		if e.log == nil {
			return
		}
		for _, off := range offsets {
			e.log.Ack(off)
		}
	}

	for attempt := range e.opts.retryAttempts + 1 {
		err := e.sink.StoreBatch(ctx, items)
		if err == nil {
			ack()
			return
		}

		if attempt < e.opts.retryAttempts {
			backoff := e.opts.retryBackoff << attempt
			e.opts.logger.Warn("dispatch: sink store failed, retrying",
				slog.Int("attempt", attempt+1),
				slog.Int("items", len(items)),
				slog.Duration("backoff", backoff),
				slog.Any("error", err))

			timer := time.NewTimer(backoff)
			select {
			case <-timer.C:
			case <-e.done:
				coretime.TimerStopAndDrain(timer)
				// Last-ditch flush on shutdown: if the sink recovers
				// between the retry sleep starting and the shutdown
				// signal, drain into it so we don't leave durable WAL
				// records that could have been delivered cleanly. If
				// the final attempt fails, leave the records for the
				// next Start to replay and surface the error through
				// metrics + logger — never silently drop it.
				if storeErr := e.sink.StoreBatch(ctx, items); storeErr == nil {
					ack()
				} else {
					e.metrics.sinkErrors.Add(float64(len(items)))
					e.opts.logger.Error("dispatch: final shutdown store failed",
						slog.Int("items", len(items)),
						slog.Any("error", storeErr))
				}
				return
			}
			continue
		}

		// Terminal failure after retryAttempts+1 tries. Count every item
		// as failed — not just the batch — so metric consumers can reason
		// about loss rate at the same granularity as `enqueued`.
		e.metrics.sinkErrors.Add(float64(len(items)))
		e.opts.logger.Error("dispatch: sink store failed after retries",
			slog.Int("items", len(items)),
			slog.Any("error", err))
	}
}
