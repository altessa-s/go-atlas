// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package context

import (
	"context"
	"time"
)

// ApplyTimeout returns a derived [context.Context] with the given timeout
// applied, provided the incoming context does not already carry a deadline.
// If timeout is zero or the context already has a deadline, the original
// context is returned alongside a no-op cancel func that is safe to call.
// The caller must always call the returned [context.CancelFunc] to avoid
// resource leaks.
func ApplyTimeout(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if timeout == 0 {
		return ctx, func() {}
	}

	if _, hasDeadline := ctx.Deadline(); hasDeadline {
		return ctx, func() {}
	}

	return context.WithTimeout(ctx, timeout)
}

// WithDefault returns a derived [context.Context] with defaultTimeout applied
// when the incoming context does not already carry a deadline. If ctx is nil,
// [context.Background] is used as the base context. When a deadline is already
// present, the original context is returned with a no-op cancel func.
// The caller must always call the returned [context.CancelFunc] to avoid
// resource leaks.
func WithDefault(ctx context.Context, defaultTimeout time.Duration) (context.Context, context.CancelFunc) {
	if ctx == nil {
		ctx = context.Background()
	}

	if _, hasDeadline := ctx.Deadline(); hasDeadline {
		return ctx, func() {}
	}

	return context.WithTimeout(ctx, defaultTimeout)
}

// OrBackground returns ctx unchanged when it is non-nil; otherwise it returns
// [context.Background]. This is useful as a guard at the top of functions that
// accept an optional context parameter.
func OrBackground(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}

// WithMaxTimeout returns a derived [context.Context] whose deadline is capped at
// maxTimeout from now. If the incoming context already has a deadline that is
// sooner than maxTimeout, the original context is returned unchanged. When the
// context has no deadline, or its deadline exceeds maxTimeout, a new context
// with maxTimeout is created via [context.WithTimeout]. If maxTimeout is zero
// the original context is returned with a no-op cancel func. A nil ctx is
// treated as [context.Background]. The caller must always call the returned
// [context.CancelFunc] to avoid resource leaks.
func WithMaxTimeout(ctx context.Context, maxTimeout time.Duration) (context.Context, context.CancelFunc) {
	if ctx == nil {
		ctx = context.Background()
	}

	if maxTimeout == 0 {
		return ctx, func() {}
	}

	if deadline, hasDeadline := ctx.Deadline(); hasDeadline {
		if time.Until(deadline) <= maxTimeout {
			return ctx, func() {}
		}
	}

	return context.WithTimeout(ctx, maxTimeout)
}
