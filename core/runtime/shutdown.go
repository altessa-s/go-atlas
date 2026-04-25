// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package runtime

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
)

// ShutdownHook is a function executed during application shutdown via [RunShutdownHooks].
// The provided context may carry a deadline; implementations should respect cancellation.
// Returning a non-nil error does not prevent subsequent hooks from running.
type ShutdownHook func(ctx context.Context) error

var (
	shutdownHooks []ShutdownHook
	shutdownMu    sync.Mutex
	shutdownOnce  sync.Once
)

// OnShutdown registers a [ShutdownHook] to be called when [RunShutdownHooks] is invoked.
// Hooks are executed in reverse registration order (LIFO), so resources registered
// first are cleaned up last. This is safe to call from multiple goroutines concurrently.
func OnShutdown(hook ShutdownHook) {
	shutdownMu.Lock()
	defer shutdownMu.Unlock()
	shutdownHooks = append(shutdownHooks, hook)
}

// RunShutdownHooks executes all registered shutdown hooks in LIFO order.
// It is safe to call from multiple goroutines; hooks execute at most once regardless
// of how many times this function is called. Errors from individual hooks are
// collected and joined into a single error via [errors.Join]. A failing hook
// does not prevent subsequent hooks from running. Each hook receives ctx, so
// callers can enforce a deadline on the entire shutdown sequence.
func RunShutdownHooks(ctx context.Context) error {
	var errs []error
	shutdownOnce.Do(func() {
		shutdownMu.Lock()
		hooks := shutdownHooks
		shutdownMu.Unlock()

		// Execute in reverse order
		for i := len(hooks) - 1; i >= 0; i-- {
			if err := hooks[i](ctx); err != nil {
				// We collect errors but continue shutdown
				errs = append(errs, coreerrs.Wrap(err, "shutdown hook failed"))
				// If we have a logger in previous layers we might log here,
				// but since this is low-level runtime, we might just print to stderr
				// if it's critical, or rely on the returned error.
				fmt.Fprintf(os.Stderr, "shutdown hook error: %v\n", err)
			}
		}
	})

	return errors.Join(errs...)
}
