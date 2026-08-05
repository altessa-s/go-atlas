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

// ShutdownHook is a function executed during application shutdown via
// [RunShutdownHooks] or [HookGroup.Shutdown]. The provided context may carry a
// deadline; implementations should respect cancellation. Returning a non-nil
// error does not prevent subsequent hooks from running.
type ShutdownHook func(ctx context.Context) error

// HookGroup is an independently runnable set of shutdown hooks.
//
// The process-wide hooks reached through [OnShutdown] and [RunShutdownHooks]
// live in one implicit group whose lifetime is the process: it runs at most
// once, and a component that registered there cannot be stopped on its own.
// That is the right default for a resource that lives as long as the program,
// and the wrong one for a background component owned by a factory, a test, or
// a subsystem that is torn down and rebuilt — those need a scope of their own,
// which is what a HookGroup is.
//
// The zero value is ready to use. A group is safe for concurrent use.
//
// Example — a subsystem that owns a background component and can be stopped
// without stopping the process:
//
//	var hooks runtime.HookGroup
//	hooks.OnShutdown(engine.Stop)
//	hooks.OnShutdown(storage.Close)
//	...
//	if err := hooks.Shutdown(ctx); err != nil { // storage.Close, then engine.Stop
//	    return err
//	}
type HookGroup struct {
	mu    sync.Mutex
	hooks []ShutdownHook
	once  sync.Once
}

// OnShutdown registers a hook to run when [HookGroup.Shutdown] is invoked.
// Hooks run in reverse registration order (LIFO), so resources registered
// first are cleaned up last. Safe for concurrent use.
func (g *HookGroup) OnShutdown(hook ShutdownHook) {
	g.mu.Lock()
	defer g.mu.Unlock()

	g.hooks = append(g.hooks, hook)
}

// Shutdown runs the group's hooks in LIFO order, at most once regardless of
// how many times it is called. Errors are collected and joined via
// [errors.Join]; a failing hook does not prevent the rest from running. Each
// hook receives ctx, so a caller can bound the whole sequence with a deadline.
//
// Hooks registered after Shutdown starts are not run: the set is snapshotted
// up front so a hook cannot extend the sequence it is part of.
func (g *HookGroup) Shutdown(ctx context.Context) error {
	var errs []error

	g.once.Do(func() {
		g.mu.Lock()
		hooks := g.hooks
		g.mu.Unlock()

		for i := len(hooks) - 1; i >= 0; i-- {
			if err := hooks[i](ctx); err != nil {
				// Collect and keep going: one broken hook must not strand the
				// resources the others release.
				errs = append(errs, coreerrs.Wrap(err, "shutdown hook failed"))
				// This is the lowest runtime layer and has no logger, and the
				// joined error may never be read on a panic or signal path, so
				// stderr is the only place the failure is guaranteed to surface.
				fmt.Fprintf(os.Stderr, "shutdown hook error: %v\n", err)
			}
		}
	})

	return errors.Join(errs...)
}

// processHooks is the implicit group behind [OnShutdown] and
// [RunShutdownHooks]: the hooks whose scope is the process itself.
var processHooks HookGroup

// OnShutdown registers a [ShutdownHook] to be called when [RunShutdownHooks] is invoked.
// Hooks are executed in reverse registration order (LIFO), so resources registered
// first are cleaned up last. This is safe to call from multiple goroutines concurrently.
//
// Use a [HookGroup] instead when the component's lifetime is shorter than the
// process's — a hook registered here can never be run on its own.
func OnShutdown(hook ShutdownHook) {
	processHooks.OnShutdown(hook)
}

// RunShutdownHooks executes all hooks registered via [OnShutdown] in LIFO order.
// It is safe to call from multiple goroutines; hooks execute at most once regardless
// of how many times this function is called. Errors from individual hooks are
// collected and joined into a single error via [errors.Join]. A failing hook
// does not prevent subsequent hooks from running. Each hook receives ctx, so
// callers can enforce a deadline on the entire shutdown sequence.
func RunShutdownHooks(ctx context.Context) error {
	return processHooks.Shutdown(ctx)
}
