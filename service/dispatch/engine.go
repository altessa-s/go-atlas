// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package dispatch

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/altessa-s/go-atlas/core/io/wal"

	corecontext "github.com/altessa-s/go-atlas/core/context"
	coreretry "github.com/altessa-s/go-atlas/core/retry"
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

	// started guards Start against a second call. It is set before the WAL is
	// opened, so it says "Start has begun", not "the engine is usable".
	started atomic.Bool
	// ready is stored at the end of a successful Start and is what Submit
	// gates on. Publishing it last gives Submit a happens-before edge to the
	// WAL handle — gating on started instead would let a concurrent Submit
	// observe a nil log and silently bypass durability — and it enforces the
	// documented ordering that WAL replay is queued before any new item.
	ready   atomic.Bool
	closed  atomic.Bool
	dropped atomic.Int64

	lifecycleMu sync.Mutex
	submitMu    sync.RWMutex
	drainDone   chan struct{}
	wake        chan struct{}
	claimMu     sync.Mutex
	inFlight    map[wal.Offset]struct{}
	lastError   atomic.Pointer[error]
	wg          sync.WaitGroup
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

	e := &Engine[T]{
		opts:      o,
		sink:      sink,
		codec:     o.codec,
		metrics:   newEngineMetrics(o.collector, o.metricsSubsystem),
		submitted: make(chan envelope[T], o.bufferSize),
		done:      make(chan struct{}),
		drainDone: make(chan struct{}),
		wake:      make(chan struct{}, o.workers),
		inFlight:  make(map[wal.Offset]struct{}),
	}
	// Established here rather than in Start so that Shutdown is always safe to
	// call, including on an engine that was never started.
	e.shutdownCtx, e.shutdownCancel = context.WithCancel(context.Background())

	return e, nil
}

// Start opens the WAL (if configured), replays any unflushed records by
// pushing them through the worker queue, and launches the worker goroutines.
// It is an error to call Start more than once.
func (e *Engine[T]) Start() error {
	e.lifecycleMu.Lock()
	defer e.lifecycleMu.Unlock()
	if e.closed.Load() {
		return ErrEngineClosed
	}
	if !e.started.CompareAndSwap(false, true) {
		return ErrAlreadyStarted
	}
	if e.opts.walEnabled {
		w, recovered, err := wal.Open(e.opts.walDir, e.opts.walOpts...)
		if err != nil {
			return err
		}
		e.log = w
		for _, record := range recovered {
			if _, err := e.codec.Decode(record.Payload); err != nil {
				return errors.Join(fmt.Errorf("%w: %w", ErrReplayDecode, err), w.Close())
			}
		}
		e.metrics.replayCount.Add(float64(len(recovered)))
	}
	for range e.opts.workers {
		// Long-lived batching workers have queue/drain semantics; Process is
		// intended for finite slices and does not provide this lifecycle.
		if e.log != nil {
			e.wg.Go(e.durableWorker)
		} else {
			e.wg.Go(e.worker)
		}
	}
	e.ready.Store(true)
	return nil
}

// Submission describes what accepted an item; neither value means sink delivery.
// Logged records reach stable storage on the WAL's fsync cadence.
type Submission uint8

const (
	// Rejected means the item was not accepted.
	Rejected Submission = iota
	// Queued means the item is in volatile memory only.
	Queued
	// Logged means the item was appended to the WAL for asynchronous delivery.
	Logged
)

var (
	// ErrQueueFull means the volatile queue has no capacity.
	ErrQueueFull = errors.New("dispatch: queue full")
	// ErrBacklog means Shutdown left accepted records undelivered.
	ErrBacklog = errors.New("dispatch: undelivered backlog")
	// ErrReplayDecode means replay cannot decode a retained WAL record.
	ErrReplayDecode = errors.New("dispatch: replay decode failed")
)

// Submit is the fire-and-forget producer interface. It reports acceptance by
// either memory or the WAL. Use Enqueue when the acceptance tier or rejection
// error matters. With WAL, a full memory queue never rejects a logged record.
func (e *Engine[T]) Submit(item T) bool {
	_, err := e.Enqueue(item)
	return err == nil
}

// Enqueue accepts an item and reports its storage tier. It is non-blocking on
// sink I/O; WithBackPressure may block volatile queue admission. WAL admission
// is bounded by the WAL byte budget, not by the volatile queue capacity.
func (e *Engine[T]) Enqueue(item T) (Submission, error) {
	if !e.ready.Load() || e.closed.Load() {
		return Rejected, ErrEngineClosed
	}
	e.submitMu.RLock()
	defer e.submitMu.RUnlock()
	if e.closed.Load() {
		return Rejected, ErrEngineClosed
	}
	if e.log != nil {
		payload, err := e.codec.Encode(item)
		if err != nil {
			e.metrics.encodeErrors.Inc()
			e.dropItem(item)
			return Rejected, err
		}
		if _, err := e.log.Append(payload); err != nil {
			e.metrics.walErrors.Inc()
			e.dropItem(item)
			return Rejected, err
		}
		e.metrics.enqueued.Inc()
		e.notifyWorker()
		return Logged, nil
	}
	env := envelope[T]{item: item}
	if e.opts.backPressure {
		select {
		case e.submitted <- env:
			e.metrics.enqueued.Inc()
			return Queued, nil
		case <-e.done:
			return Rejected, ErrEngineClosed
		}
	}
	select {
	case e.submitted <- env:
		e.metrics.enqueued.Inc()
		return Queued, nil
	default:
		e.dropItem(item)
		return Rejected, ErrQueueFull
	}
}

func (e *Engine[T]) dropItem(item T) {
	e.dropped.Add(1)
	e.metrics.dropped.Inc()
	if e.opts.onDrop != nil {
		e.opts.onDrop(item)
	}
}

// Dropped counts rejected items, including encoding and WAL admission failures.
func (e *Engine[T]) Dropped() int64 { return e.dropped.Load() }

// WAL returns the journal for diagnostics. Callers must not mutate or close it
// while the engine is running; Engine owns its lifecycle and acknowledgements.
func (e *Engine[T]) WAL() *wal.WAL { return e.log }

// Shutdown closes admission, drains accepted items, and closes the WAL. It
// returns ErrBacklog when durable records remain, joined with deadline, sink,
// and journal errors. Callbacks must honor their contexts. Repeated calls are
// no-ops; the first call owns the drain.
func (e *Engine[T]) Shutdown(ctx context.Context) error {
	e.lifecycleMu.Lock()
	if !e.closed.CompareAndSwap(false, true) {
		e.lifecycleMu.Unlock()
		return nil
	}
	close(e.done)
	// Closing admission wakes blocked producers before waiting for their locks.
	e.submitMu.Lock()
	close(e.drainDone)
	e.submitMu.Unlock()
	e.lifecycleMu.Unlock()
	finished := make(chan struct{})
	go func() { e.wg.Wait(); close(finished) }()
	var errs []error
	select {
	case <-finished:
	case <-ctx.Done():
		e.shutdownCancel()
		errs = append(errs, ctx.Err())
	}
	e.shutdownCancel()
	if e.log != nil {
		if pending := e.log.Stats().Pending; pending > 0 {
			errs = append(errs, fmt.Errorf("%w: %d records", ErrBacklog, pending))
		}
		if err := e.log.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if err := e.lastError.Load(); err != nil {
		errs = append(errs, *err)
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
		if err := e.storeBatch(batch, offsets); err != nil {
			e.lastError.Store(&err)
		}
		e.observeWALSize()
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

		case <-e.drainDone:
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

// observeWALSize refreshes the WAL size gauge. It runs once per flushed batch
// rather than once per Submit: Stats takes the same mutex as Append, so
// sampling it inline doubled the WAL lock acquisitions on the hot path — at
// the default batch size that is a hundredfold reduction in contention for a
// gauge nobody reads at per-item resolution.
func (e *Engine[T]) observeWALSize() {
	if e.log == nil {
		return
	}
	e.metrics.walBytes.Set(float64(e.log.Stats().TotalBytes))
}

// storeBatch retries durable records until delivery or shutdown. RetryAttempts
// bounds a volatile batch; durable batches continue in capped backoff rounds.
func (e *Engine[T]) storeBatch(items []T, offsets []wal.Offset) error {
	stop := e.metrics.flushDuration.Start()
	defer stop()
	const backoffFactor = 2
	nextDelay := coreretry.Exponential(coreretry.ExponentialConfig{
		BaseDelay: e.opts.retryBackoff, MaxDelay: e.opts.retryMaxBackoff,
		Factor: backoffFactor, Jitter: e.opts.retryJitter,
	})
	for attempt := 0; ; attempt++ {
		ctx, cancel := corecontext.ApplyTimeout(e.shutdownCtx, e.opts.storeTimeout)
		err := e.sink.StoreBatch(ctx, items)
		cancel()
		if err == nil {
			if e.log != nil {
				for _, off := range offsets {
					e.log.Ack(off)
				}
			}
			return nil
		}
		e.metrics.sinkErrors.Add(float64(len(items)))
		if e.shutdownCtx.Err() != nil || ((e.log == nil || e.closed.Load()) && attempt >= e.opts.retryAttempts) {
			return err
		}
		e.opts.logger.Warn("dispatch: retrying sink batch", slog.Any("error", err), slog.Int("items", len(items)))
		// Saturate the exponent; MaxDelay bounds long outages without overflow.
		const maxBackoffExponent = 30
		backoff := nextDelay(min(attempt, maxBackoffExponent), err)
		if backoff <= 0 {
			backoff = DefaultRetryBackoff
		}
		timer := time.NewTimer(backoff)
		select {
		case <-timer.C:
		case <-e.done:
			coretime.TimerStopAndDrain(timer)
		}
	}
}
