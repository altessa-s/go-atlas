// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package buffered

import (
	"context"
	"log/slog"
	"sync"

	"github.com/altessa-s/go-atlas/core/runtime/panics"
	"github.com/altessa-s/go-atlas/observability/slog/handler/internal/base"
)

// Handler wraps another [slog.Handler] and processes log records asynchronously
// via a background goroutine. Safe for concurrent use.
// Handlers derived via [Handler.WithAttrs] and [Handler.WithGroup] share the
// worker and buffer of the handler they were derived from.
// Call [Handler.Shutdown] on any of them to drain the buffer and stop the worker.
type Handler struct {
	base.Base
	p *pipeline
}

// pipeline is the buffer and worker shared by a handler and all handlers
// derived from it.
type pipeline struct {
	opts    options
	records chan entry
	flushCh chan chan struct{}
	stop    chan struct{}
	done    chan struct{}

	// mu orders enqueues against shutdown: records are enqueued under the read
	// lock while stopped is false, so every queued record is in the channel
	// before stop is closed and the worker drains it.
	mu       sync.RWMutex
	stopped  bool
	stopOnce sync.Once
}

// entry is a queued record together with the inner handler of the
// (possibly derived) handler that accepted it.
type entry struct {
	handler slog.Handler
	record  slog.Record
}

// options contains configuration for the buffered handler.
type options struct {
	bufferSize  int
	bypassLevel slog.Level
}

// Option configures the buffered handler.
type Option func(*options)

// WithBufferSize sets the size of the log buffer. Default is 100.
func WithBufferSize(size int) Option {
	return func(o *options) {
		o.bufferSize = size
	}
}

// WithBypassLevel sets the minimum level to bypass the buffer (write synchronously).
// Default is slog.LevelError.
func WithBypassLevel(level slog.Level) Option {
	return func(o *options) {
		o.bypassLevel = level
	}
}

// defaultBufferSize is the default number of log records that can be buffered.
const defaultBufferSize = 100

// NewHandler creates a new buffered handler wrapping the given inner handler.
func NewHandler(inner slog.Handler, opts ...Option) *Handler {
	o := options{
		bufferSize:  defaultBufferSize,
		bypassLevel: slog.LevelError,
	}
	for _, opt := range opts {
		opt(&o)
	}

	b := base.NewBase(inner) // panics on a nil inner before the worker starts

	p := &pipeline{
		opts:    o,
		records: make(chan entry, o.bufferSize),
		flushCh: make(chan chan struct{}),
		stop:    make(chan struct{}),
		done:    make(chan struct{}),
	}
	go p.worker()

	return &Handler{Base: b, p: p}
}

// Enabled delegates to the inner handler.
func (h *Handler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.Base.Enabled(ctx, level)
}

// Handle processes the record.
// If the record level meets the bypass level, it flushes the buffer and writes synchronously.
// Otherwise, it sends the record to the buffer.
func (h *Handler) Handle(ctx context.Context, r slog.Record) error {
	p := h.p
	p.mu.RLock()

	// Check if already stopped
	if p.stopped {
		p.mu.RUnlock()
		return h.Inner().Handle(ctx, r)
	}

	// Bypass buffer for high severity logs to avoid loss on crash
	if r.Level >= p.opts.bypassLevel {
		p.mu.RUnlock()
		p.flush()
		return h.Inner().Handle(ctx, r)
	}

	// Try to send to buffer; the send never blocks while holding the lock.
	select {
	case p.records <- entry{handler: h.Inner(), record: r.Clone()}:
		p.mu.RUnlock()
		return nil
	default:
		p.mu.RUnlock()
		// Buffer full - fallback to synchronous write to avoid dropping
		return h.Inner().Handle(ctx, r)
	}
}

// WithAttrs returns a buffered handler wrapping the inner handler's WithAttrs
// result. It shares the buffer and worker of h, so no goroutine is started.
func (h *Handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &Handler{Base: h.WithAttrsBase(attrs), p: h.p}
}

// WithGroup returns a buffered handler wrapping the inner handler's WithGroup
// result. It shares the buffer and worker of h, so no goroutine is started.
func (h *Handler) WithGroup(name string) slog.Handler {
	return &Handler{Base: h.WithGroupBase(name), p: h.p}
}

// Shutdown flushes the buffer and stops the worker shared by h and all
// handlers derived from it. Every caller waits until the worker has drained
// the buffer or ctx is done.
func (h *Handler) Shutdown(ctx context.Context) error {
	p := h.p
	p.stopOnce.Do(func() {
		p.mu.Lock()
		p.stopped = true
		close(p.stop)
		p.mu.Unlock()
	})

	select {
	case <-p.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (p *pipeline) worker() {
	defer close(p.done)
	defer panics.Handle(context.Background())

	for {
		select {
		case e := <-p.records:
			e.write()
		case done := <-p.flushCh:
			// Drain buffer
		FlushLoop:
			for {
				select {
				case e := <-p.records:
					e.write()
				default:
					close(done)
					break FlushLoop
				}
			}
		case <-p.stop:
			// Drain buffer
			for {
				select {
				case e := <-p.records:
					e.write()
				default:
					return
				}
			}
		}
	}
}

// write is a best-effort asynchronous write of the queued record.
func (e entry) write() {
	e.handler.Handle(context.Background(), e.record) //nolint:errcheck // best-effort async write
}

// flush sends a signal to worker to drain the buffer.
// It blocks until the worker confirms the buffer is empty.
func (p *pipeline) flush() {
	done := make(chan struct{})
	select {
	case p.flushCh <- done:
		<-done
	case <-p.stop:
		// If stopped, we assume it's flushing/flushed or we can't do much
	}
}
