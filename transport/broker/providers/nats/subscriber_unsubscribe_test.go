// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package natsprovider

import (
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
)

// bufferedConsumeContext emulates a JetStream consume context whose Drain
// leaves one buffered message behind: its callback runs asynchronously and only
// once a caller waits on Closed, after which the context reports closed.
type bufferedConsumeContext struct {
	waited     chan struct{}
	waitedOnce sync.Once
	closed     chan struct{}
	callback   func()
}

func newBufferedConsumeContext(callback func()) *bufferedConsumeContext {
	return &bufferedConsumeContext{
		waited:   make(chan struct{}),
		closed:   make(chan struct{}),
		callback: callback,
	}
}

func (c *bufferedConsumeContext) Stop() {}

func (c *bufferedConsumeContext) Drain() {
	go func() {
		<-c.waited
		c.callback()
		close(c.closed)
	}()
}

func (c *bufferedConsumeContext) Closed() <-chan struct{} {
	c.waitedOnce.Do(func() { close(c.waited) })
	return c.closed
}

// TestStreamSubscriber_UnsubscribeWaitsForBufferedHandlers is the regression
// guard for Unsubscribe returning while drained messages were still queued:
// handlers register with the WaitGroup only once their callback starts, so a
// zero counter right after Drain proved nothing. Unsubscribe must wait for the
// consume context to close, which happens after the last callback returned.
func TestStreamSubscriber_UnsubscribeWaitsForBufferedHandlers(t *testing.T) {
	t.Parallel()

	ss := &streamSubscriber{}
	var handled atomic.Bool
	cc := newBufferedConsumeContext(func() {
		// Mirrors the Consume callback's WaitGroup bracketing.
		ss.handlersWg.Add(1)
		defer ss.handlersWg.Done()
		handled.Store(true)
	})
	// Release the emulated callback goroutine even if Unsubscribe never waits.
	t.Cleanup(func() { <-cc.Closed() })

	canceled := false
	ss.consumeContext = cc
	ss.handlerCtxCancel = func() { canceled = true }

	ss.Unsubscribe()

	require.True(t, canceled, "Unsubscribe must cancel the handler context")
	require.True(t, handled.Load(), "a buffered handler ran after Unsubscribe returned")
}
