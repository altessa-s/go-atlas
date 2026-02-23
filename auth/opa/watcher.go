// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package opa

import (
	"cmp"
	"sync"
)

// watchManager manages subscriptions to policy events.
// It distributes events to all registered subscribers.
type watchManager struct {
	mu          sync.RWMutex
	subscribers map[*subscriber]struct{}
	closed      bool
}

// subscriber represents a single event subscription.
type subscriber struct {
	ch             chan PolicyEvent
	done           chan struct{}
	eventTypes     map[EventType]struct{}
	overflowPolicy BufferOverflowPolicy
	closedOnce     sync.Once
}

// newWatchManager creates a new watch manager.
func newWatchManager() *watchManager {
	return &watchManager{
		subscribers: make(map[*subscriber]struct{}),
	}
}

// subscribe creates a new subscription with the given options.
func (w *watchManager) subscribe(opts WatchOptions) *WatchResult {
	bufferSize := cmp.Or(opts.BufferSize, DefaultWatchBufferSize)

	sub := &subscriber{
		ch:             make(chan PolicyEvent, bufferSize),
		done:           make(chan struct{}),
		overflowPolicy: opts.BufferOverflowPolicy,
	}

	if len(opts.EventTypes) > 0 {
		sub.eventTypes = make(map[EventType]struct{}, len(opts.EventTypes))
		for _, et := range opts.EventTypes {
			sub.eventTypes[et] = struct{}{}
		}
	}

	w.mu.Lock()
	if w.closed {
		w.mu.Unlock()
		close(sub.ch)
		close(sub.done)
		return &WatchResult{
			Events: sub.ch,
			Stop:   func() {},
			Done:   sub.done,
		}
	}
	w.subscribers[sub] = struct{}{}
	w.mu.Unlock()

	return &WatchResult{
		Events: sub.ch,
		Stop: func() {
			w.unsubscribe(sub)
		},
		Done: sub.done,
	}
}

// unsubscribe removes a subscription and closes its channels.
func (w *watchManager) unsubscribe(sub *subscriber) {
	w.mu.Lock()
	_, exists := w.subscribers[sub]
	if exists {
		delete(w.subscribers, sub)
	}
	w.mu.Unlock()

	if exists {
		sub.closedOnce.Do(func() {
			close(sub.ch)
			close(sub.done)
		})
	}
}

// broadcast sends an event to all subscribers.
func (w *watchManager) broadcast(event PolicyEvent) {
	w.mu.RLock()
	defer w.mu.RUnlock()

	for sub := range w.subscribers {
		if sub.eventTypes != nil {
			if _, ok := sub.eventTypes[event.Type]; !ok {
				continue
			}
		}

		w.sendEvent(sub, event)
	}
}

// sendEvent sends an event to a subscriber according to its overflow policy.
func (w *watchManager) sendEvent(sub *subscriber, event PolicyEvent) {
	switch sub.overflowPolicy {
	case OverflowPolicyBlock:
		// Block until space is available
		sub.ch <- event

	case OverflowPolicyDropOldest:
		// Try non-blocking send first
		select {
		case sub.ch <- event:
			return
		default:
			// Channel full - drop oldest and retry
		}
		// Drop oldest event and send new one
		for {
			select {
			case <-sub.ch:
				// Dropped oldest, try to send again
				select {
				case sub.ch <- event:
					return
				default:
					// Still full, continue dropping
					continue
				}
			default:
				// Channel became available
				select {
				case sub.ch <- event:
					return
				default:
					// Race condition, retry
					continue
				}
			}
		}

	default: // OverflowPolicyDropNewest
		select {
		case sub.ch <- event:
		default:
			// Drop event if channel is full
		}
	}
}

// close closes all subscriptions and prevents new ones.
func (w *watchManager) close() {
	w.mu.Lock()
	w.closed = true
	subscribers := make([]*subscriber, 0, len(w.subscribers))
	for sub := range w.subscribers {
		subscribers = append(subscribers, sub)
	}
	w.subscribers = make(map[*subscriber]struct{})
	w.mu.Unlock()

	for _, sub := range subscribers {
		sub.closedOnce.Do(func() {
			close(sub.ch)
			close(sub.done)
		})
	}
}
