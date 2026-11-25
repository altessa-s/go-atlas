// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package requestidcapture

import (
	"context"
	"sync"
)

type requestIDKey struct{}

type requestIDCapture struct {
	context.Context
	mu sync.Mutex
	id string
}

// WithCapture lets HTTP recovery observe the request ID resolved
// inside its handler, including when a timeout handler uses a separate goroutine.
func WithCapture(ctx context.Context) context.Context {
	return &requestIDCapture{Context: ctx}
}

// Value makes the capture discoverable through derived request contexts.
func (c *requestIDCapture) Value(key any) any {
	if key == (requestIDKey{}) {
		return c
	}
	return c.Context.Value(key)
}

// ID returns the ID published by the request ID middleware.
func ID(ctx context.Context) string {
	c, ok := ctx.Value(requestIDKey{}).(*requestIDCapture)
	if !ok {
		return ""
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.id
}

// Record publishes the ID if recovery is observing this request.
func Record(ctx context.Context, id string) {
	if c, ok := ctx.Value(requestIDKey{}).(*requestIDCapture); ok {
		c.mu.Lock()
		c.id = id
		c.mu.Unlock()
	}
}
