// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package health

import (
	"sync"
	"sync/atomic"
)

// Subscription is the concrete [Subscriber] returned by [Coordinator.Subscribe].
// Always call [Subscription.Close] when done to release resources.
// Close is safe to call multiple times.
type Subscription struct {
	initialStatus ServingStatus
	updates       <-chan ServingStatus

	cancelOnce sync.Once
	cancel     func()
}

// InitialStatus returns the [ServingStatus] captured at subscription time.
func (s *Subscription) InitialStatus() ServingStatus {
	return s.initialStatus
}

// Updates returns a read-only channel receiving [ServingStatus] changes.
// The channel is closed when [Subscription.Close] is called.
func (s *Subscription) Updates() <-chan ServingStatus {
	return s.updates
}

// Close terminates the subscription and releases resources.
func (s *Subscription) Close() {
	s.cancelOnce.Do(func() {
		if s.cancel != nil {
			s.cancel()
		}
	})
}

type watcherShard struct {
	mu       sync.RWMutex
	watchers map[string]map[*watcher]struct{}
}

type cachedStatus struct {
	status    ServingStatus
	timestamp atomic.Int64
}

type watcher struct {
	ch         chan ServingStatus
	lastStatus atomic.Int32 // ServingStatus; atomic so the notify loop and readers don't race

	// mu makes sends and the close mutually exclusive: notify holds it for
	// reading around its non-blocking send, close holds it for writing.
	mu     sync.RWMutex
	closed bool
}

func newWatcher(buffer int, initialStatus ServingStatus) *watcher {
	w := &watcher{
		ch: make(chan ServingStatus, buffer),
	}
	w.lastStatus.Store(int32(initialStatus))
	return w
}

// notify delivers status without blocking and reports whether it was sent.
// It returns false when the watcher is closed or its buffer is full.
func (w *watcher) notify(status ServingStatus) bool {
	w.mu.RLock()
	defer w.mu.RUnlock()

	if w.closed {
		return false
	}

	select {
	case w.ch <- status:
		return true
	default:
		return false
	}
}

func (w *watcher) close() {
	w.mu.Lock()
	defer w.mu.Unlock()

	if !w.closed {
		w.closed = true
		close(w.ch)
	}
}
