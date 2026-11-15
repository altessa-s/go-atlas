// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package natsprovider

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// White-box synchronization holds a drain until Unsubscribe has superseded Subscribe.
func TestSubscribeSupersededByUnsubscribe(t *testing.T) {
	t.Parallel()
	drain := &heldDrain{entered: make(chan struct{}), resume: make(chan struct{}), closed: make(chan struct{})}
	t.Cleanup(func() {
		select {
		case <-drain.resume:
		default:
			close(drain.resume)
		}
	})
	ss := &streamSubscriber{consumeContext: drain}
	subscribed := make(chan error, 1)
	go func() { subscribed <- ss.Subscribe(t.Context(), &contextHandler{}) }()
	<-drain.entered
	unsubscribed := make(chan struct{})
	go func() { ss.Unsubscribe(); close(unsubscribed) }()
	// Observe the generation under the lifecycle lock before releasing the drain.
	require.Eventually(t, func() bool {
		ss.mu.Lock()
		defer ss.mu.Unlock()
		return ss.generation == 2
	}, time.Second, time.Millisecond)
	close(drain.resume)
	err := <-subscribed
	<-unsubscribed
	require.ErrorIs(t, err, ErrSubscribeSuperseded)
	require.Nil(t, ss.consumeContext)
	require.Nil(t, ss.handlerCtxCancel)
	require.Nil(t, ss.draining)
}
