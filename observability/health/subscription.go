// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package health

import (
	"sync"
	"sync/atomic"
)

// Note: watcher uses atomic.Int32 for thread-safe closed flag management.

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
	timestamp int64
}

type watcher struct {
	ch         chan ServingStatus
	lastStatus ServingStatus
	closed     atomic.Int32
}

func newWatcher(buffer int, initialStatus ServingStatus) *watcher {
	return &watcher{
		ch:         make(chan ServingStatus, buffer),
		lastStatus: initialStatus,
	}
}

func (w *watcher) notify(status ServingStatus) {
	if w.closed.Load() != 0 {
		return
	}

	defer func() {
		if r := recover(); r != nil {
			_ = r
		}
	}()

	select {
	case w.ch <- status:
	default:
	}
}

func (w *watcher) close() {
	if w.closed.CompareAndSwap(0, 1) {
		close(w.ch)
	}
}
