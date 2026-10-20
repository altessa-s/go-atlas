// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package limiters

import (
	"context"

	"github.com/altessa-s/go-atlas/core/types/nilcheck"
)

// Func adapts a plain function to [RequestsLimiter].
type Func func(ctx context.Context, key string) error

// Allow calls f.
func (f Func) Allow(ctx context.Context, key string) error { return f(ctx, key) }

// chain checks its limiters in order and stops at the first refusal.
type chain []RequestsLimiter

// Allow passes key through every limiter of the chain in order and returns
// the first refusal; later limiters are not consulted.
func (c chain) Allow(ctx context.Context, key string) error {
	for _, l := range c {
		if err := l.Allow(ctx, key); err != nil {
			return err
		}
	}
	return nil
}

// Chain combines limiters into one that consults them in order and stops at
// the first refusal, so a request refused by an earlier limiter (a request
// budget) takes nothing from a later one (a per-host pacer). The converse
// does not hold: a refusal by a later limiter does not return what earlier
// ones took. Where the limits live in one storage, admit them together with
// [github.com/altessa-s/go-atlas/data/limiters/storages.AllowAll], which takes
// a slot on every key or on none. Nil limiters are skipped. Chain returns nil when no limiter is left, and the limiter itself
// when only one is, so the result can be passed to client.WithLimiter, which
// ignores nil.
func Chain(limiters ...RequestsLimiter) RequestsLimiter {
	c := make(chain, 0, len(limiters))
	for _, l := range limiters {
		if nilcheck.IsNotNil(l) {
			c = append(c, l)
		}
	}
	switch len(c) {
	case 0:
		return nil
	case 1:
		return c[0]
	default:
		return c
	}
}
