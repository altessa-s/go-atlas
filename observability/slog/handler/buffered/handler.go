// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package buffered

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"

	"github.com/altessa-s/go-atlas/observability/slog/handler/internal/base"
)

// Handler wraps another [slog.Handler] and processes log records asynchronously
// via a background goroutine. Safe for concurrent use.
// Call [Handler.Shutdown] to drain the buffer and stop the worker.
type Handler struct {
	base.Base
	opts    options
	records chan slog.Record
	flushCh chan chan struct{}
	stop    chan struct{}
	wg      sync.WaitGroup
	stopped atomic.Bool
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

	h := &Handler{
		Base:    base.NewBase(inner),
		opts:    o,
		records: make(chan slog.Record, o.bufferSize),
		flushCh: make(chan chan struct{}),
		stop:    make(chan struct{}),
	}

	h.wg.Go(h.worker)

	return h
}

// Enabled delegates to the inner handler.
func (h *Handler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.Base.Enabled(ctx, level)
}

// Handle processes the record.
// If the record level meets the bypass level, it flushes the buffer and writes synchronously.
// Otherwise, it sends the record to the buffer.
func (h *Handler) Handle(ctx context.Context, r slog.Record) error {
	// Check if already stopped
	if h.stopped.Load() {
		return h.Inner().Handle(ctx, r)
	}

	// Bypass buffer for high severity logs to avoid loss on crash
	if r.Level >= h.opts.bypassLevel {
		h.flush()
		return h.Inner().Handle(ctx, r)
	}

	// Try to send to buffer
	select {
	case h.records <- base.CloneRecord(r):
		return nil
	default:
		// Buffer full - fallback to synchronous write to avoid dropping
		// Alternatively could implement drop logic or blocking
		return h.Inner().Handle(ctx, r)
	}
}

// WithAttrs returns a new generic handler because we can't easily clone the worker logic.
// However, the standard pattern for stateful handlers is tricky.
// For a buffered handler, usually it sits at the top or bottom.
// If we return a new BufferedHandler, it would need its own goroutine.
// Instead, we delegate WithAttrs to the inner handler and keep using THIS buffered handler instance?
// No, the slog contract expects immutable handlers.
// So we create a new buffered handler that shares nothing with the old one,
// but wraps the NEW inner handler.
func (h *Handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return NewHandler(h.Inner().WithAttrs(attrs), WithBufferSize(h.opts.bufferSize), WithBypassLevel(h.opts.bypassLevel))
}

// WithGroup returns a new buffered handler.
func (h *Handler) WithGroup(name string) slog.Handler {
	return NewHandler(h.Inner().WithGroup(name), WithBufferSize(h.opts.bufferSize), WithBypassLevel(h.opts.bypassLevel))
}

// Shutdown flushes the buffer and stops the worker.
func (h *Handler) Shutdown(ctx context.Context) error {
	if !h.stopped.CompareAndSwap(false, true) {
		return nil
	}

	close(h.stop)

	done := make(chan struct{})
	go func() {
		h.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (h *Handler) worker() {
	for {
		select {
		case r := <-h.records:
			h.Inner().Handle(context.Background(), r) //nolint:errcheck // best-effort async write
		case done := <-h.flushCh:
			// Drain buffer
		FlushLoop:
			for {
				select {
				case r := <-h.records:
					h.Inner().Handle(context.Background(), r) //nolint:errcheck // best-effort flush
				default:
					close(done)
					break FlushLoop
				}
			}
		case <-h.stop:
			// Drain buffer
			for {
				select {
				case r := <-h.records:
					h.Inner().Handle(context.Background(), r) //nolint:errcheck // best-effort drain on shutdown
				default:
					return
				}
			}
		}
	}
}

// flush sends a signal to worker to drain the buffer.
// It blocks until the worker confirms the buffer is empty.
func (h *Handler) flush() {
	done := make(chan struct{})
	select {
	case h.flushCh <- done:
		<-done
	case <-h.stop:
		// If stopped, we assume it's flushing/flushed or we can't do much
	}
}
