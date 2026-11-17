// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package purgeloop

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/altessa-s/go-atlas-core/runtime/panics"

	coreerrs "github.com/altessa-s/go-atlas-core/errors"
)

// DefaultStepTimeout bounds one purge statement to 30 seconds when the caller
// passes no positive timeout.
const DefaultStepTimeout = 30 * time.Second

// Loop runs periodic purge passes until permanently stopped. It is safe for
// concurrent use and must be created with New.
type Loop struct {
	started atomic.Bool
	mu      sync.Mutex
	stopped bool
	cancel  context.CancelFunc
	done    chan struct{}
	every   time.Duration
	pass    func(context.Context) (int64, error)
	logger  *slog.Logger
	what    string
}

// New creates a loop without starting it. A nonpositive interval disables it;
// a nil logger discards logs. what names the purged records in log messages.
func New(every time.Duration, pass func(context.Context) (int64, error), logger *slog.Logger, what string) *Loop {
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	return &Loop{every: every, pass: pass, logger: logger, what: what}
}

// Start starts the background loop once, unless disabled or permanently stopped.
// Its context derives from context.Background and carries no caller values.
func (l *Loop) Start() {
	if l.every <= 0 || l.started.Load() {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.started.Load() || l.stopped {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	l.cancel, l.done = cancel, make(chan struct{})
	l.started.Store(true)
	go l.run(ctx)
}

// Stop permanently stops the loop and cancels a pass in progress. It returns a
// channel closed when the loop exits, or nil if the loop never started.
func (l *Loop) Stop() <-chan struct{} {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.stopped = true
	if !l.started.Load() {
		return nil
	}
	l.cancel()
	return l.done
}

// Wait waits within ctx for done. A nil or already closed done satisfies even
// an ended context.
func (l *Loop) Wait(ctx context.Context, done <-chan struct{}) error {
	if done == nil {
		return nil
	}
	select {
	case <-done:
		return nil
	default:
	}
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return coreerrs.Wrap(ctx.Err(), "wait for "+l.what+" purge to stop")
	}
}

// Close permanently stops the loop and waits within ctx for it to exit.
func (l *Loop) Close(ctx context.Context) error { return l.Wait(ctx, l.Stop()) }

func (l *Loop) run(ctx context.Context) {
	defer close(l.done)
	ticker := time.NewTicker(l.every)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			l.runPass(ctx)
		}
	}
}

func (l *Loop) runPass(ctx context.Context) {
	defer panics.HandleWithOpts(ctx, panics.NewHandleOpts().SetReallyPanic(false), func(ctx context.Context, _ any) {
		l.logger.ErrorContext(ctx, l.what+" purge panicked")
	})
	if n, err := l.pass(ctx); err != nil && ctx.Err() == nil {
		l.logger.ErrorContext(ctx, "failed to purge "+l.what, "error", err, "deleted", n)
	}
}

// Drain runs step until it considers fewer than batch rows, deletes nothing,
// ctx ends, or step fails. A step that deletes nothing makes no progress: its
// rows were renewed or removed by another sweeper, or, inside a REPEATABLE READ
// transaction, stay visible in the snapshot after a concurrent delete.
// It returns completed deletions, including those before an error.
// Each step has its own timeout; nonpositive timeout uses DefaultStepTimeout.
// Context errors are returned unwrapped for callers to annotate.
func Drain(ctx context.Context, batch int, timeout time.Duration,
	step func(ctx context.Context) (deleted, considered int64, err error)) (int64, error) {
	if timeout <= 0 {
		timeout = DefaultStepTimeout
	}
	var total int64
	for {
		if err := ctx.Err(); err != nil {
			return total, err
		}
		n, considered, err := func() (int64, int64, error) {
			stepCtx, cancel := context.WithTimeout(ctx, timeout)
			defer cancel()
			return step(stepCtx)
		}()
		total += n
		if err != nil {
			return total, err
		}
		if n == 0 || considered < int64(batch) {
			return total, nil
		}
	}
}
