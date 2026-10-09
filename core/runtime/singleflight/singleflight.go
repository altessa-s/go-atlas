// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package singleflight

import (
	"context"
	"errors"
	"sync"
	"time"
)

// ErrOverloaded means the group cannot admit another distinct key. Existing
// computations can still be joined while the group is at capacity.
var ErrOverloaded = errors.New("singleflight: concurrent computation limit reached")

// ErrAborted means a callback exited with runtime.Goexit instead of returning.
var ErrAborted = errors.New("singleflight: callback exited without returning")

type call[V any] struct {
	done       chan struct{}
	value      V
	err        error
	panicValue any
}

// Group shares one computation among callers with equal keys. Key equality is
// Go equality, never string formatting. Create a group with New.
//
// Each caller can stop waiting independently. Shared work retains the first
// caller's context values but uses the group's timeout instead of that caller's
// cancellation. Callbacks must honor this context. Capacity is released only
// when the callback exits, even if every caller has stopped waiting.
type Group[K comparable, V any] struct {
	mu            sync.Mutex
	calls         map[K]*call[V]
	maxConcurrent int
	timeout       time.Duration
}

// New creates a group. Non-positive limits allow unlimited distinct work;
// non-positive timeouts disable the shared deadline.
func New[K comparable, V any](opts ...Option) *Group[K, V] {
	o := newOptions(opts...)
	return &Group[K, V]{calls: make(map[K]*call[V]), maxConcurrent: o.maxConcurrent, timeout: o.timeout}
}

// Do computes fn once per in-flight key, or joins an existing computation.
// There is no background queue: a new key at capacity returns ErrOverloaded.
// Callback panics are re-raised in callers still waiting for the result.
func (g *Group[K, V]) Do(ctx context.Context, key K, fn func(context.Context) (V, error)) (V, error) {
	var zero V
	if err := ctx.Err(); err != nil {
		return zero, err
	}
	g.mu.Lock()
	c, exists := g.calls[key]
	if !exists {
		if g.maxConcurrent > 0 && len(g.calls) >= g.maxConcurrent {
			g.mu.Unlock()
			return zero, ErrOverloaded
		}
		c = &call[V]{done: make(chan struct{})}
		g.calls[key] = c
		// This is keyed fan-in, not slice fan-out: Process cannot preserve
		// shared ownership and independently cancellable waiters.
		go g.run(context.WithoutCancel(ctx), key, c, fn)
	}
	g.mu.Unlock()
	select {
	case <-ctx.Done():
		return zero, ctx.Err()
	case <-c.done:
		if c.panicValue != nil {
			panic(c.panicValue)
		}
		return c.value, c.err
	}
}

func (g *Group[K, V]) run(ctx context.Context, key K, c *call[V], fn func(context.Context) (V, error)) {
	returned := false
	defer func() {
		c.panicValue = recover()
		if !returned && c.panicValue == nil {
			c.err = ErrAborted
		}
		g.mu.Lock()
		delete(g.calls, key)
		close(c.done)
		g.mu.Unlock()
	}()
	if g.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, g.timeout)
		defer cancel()
	}
	c.value, c.err = fn(ctx)
	returned = true
}
