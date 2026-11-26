// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package opa

import (
	"cmp"
	"context"
	"sync"

	coremaps "github.com/altessa-s/go-atlas-core/collections/maps"
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
	eventTypes     *coremaps.ImmutableMap[EventType, struct{}]
	overflowPolicy BufferOverflowPolicy
	inFlight       sync.WaitGroup
	stopping       bool // protected by watchManager.mu
	closedOnce     sync.Once
	stopContext    func() bool
}

// newWatchManager creates a new watch manager.
func newWatchManager() *watchManager {
	return &watchManager{
		subscribers: make(map[*subscriber]struct{}),
	}
}

// subscribe creates a new subscription with the given options.
func (w *watchManager) subscribe(ctx context.Context, opts WatchOptions) *WatchResult {
	bufferSize := cmp.Or(opts.BufferSize, DefaultWatchBufferSize)

	sub := &subscriber{
		ch:             make(chan PolicyEvent, bufferSize),
		done:           make(chan struct{}),
		overflowPolicy: opts.BufferOverflowPolicy,
	}

	if len(opts.EventTypes) > 0 {
		eventTypes := make(map[EventType]struct{}, len(opts.EventTypes))
		for _, et := range opts.EventTypes {
			eventTypes[et] = struct{}{}
		}
		sub.eventTypes = coremaps.NewImmutableMap(eventTypes)
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
	sub.stopContext = context.AfterFunc(ctx, func() { w.unsubscribe(sub) })
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
	sub.stopAdmission()
	w.mu.Unlock()
	sub.finish()
	w.mu.Lock()
	delete(w.subscribers, sub)
	w.mu.Unlock()
}

// stopAdmission is called under the watch manager lock.
func (s *subscriber) stopAdmission() {
	if !s.stopping {
		s.stopping = true
		close(s.done)
	}
}

func (s *subscriber) finish() {
	s.closedOnce.Do(func() {
		s.stopContext()
		s.inFlight.Wait()
		close(s.ch)
	})
}

// broadcast sends an event to all matching subscribers. Subscribers are
// snapshot under RLock and events are delivered after the lock is released,
// so a slow consumer using OverflowPolicyBlock cannot stall the entire
// broadcast path or prevent unsubscribe from acquiring the write lock.
func (w *watchManager) broadcast(ctx context.Context, event PolicyEvent) {
	w.mu.RLock()
	subs := make([]*subscriber, 0, len(w.subscribers))
	for sub := range w.subscribers {
		if sub.stopping || (sub.eventTypes != nil && !sub.eventTypes.Contains(event.Type)) {
			continue
		}
		subs = append(subs, sub)
	}
	w.mu.RUnlock()

	for _, sub := range subs {
		// Admit each send immediately before delivery. A blocked subscriber
		// cannot hold another subscriber's Stop hostage.
		w.mu.RLock()
		if sub.stopping {
			w.mu.RUnlock()
			continue
		}
		sub.inFlight.Add(1)
		w.mu.RUnlock()
		w.sendEvent(ctx, sub, event)
		sub.inFlight.Done()
	}
}

// sendEvent sends an event to a subscriber according to its overflow policy.
// Every path includes <-sub.done so that in-flight sends bail out promptly
// when the subscriber is being closed.
func (w *watchManager) sendEvent(ctx context.Context, sub *subscriber, event PolicyEvent) {
	select {
	case <-sub.done:
		return
	default:
	}

	switch sub.overflowPolicy {
	case OverflowPolicyBlock:
		select {
		case sub.ch <- event:
		case <-ctx.Done():
		case <-sub.done:
		}

	case OverflowPolicyDropOldest:
		select {
		case sub.ch <- event:
			return
		case <-sub.done:
			return
		default:
		}
		for {
			select {
			case <-sub.ch:
				select {
				case sub.ch <- event:
					return
				case <-sub.done:
					return
				default:
					continue
				}
			case <-sub.done:
				return
			default:
				select {
				case sub.ch <- event:
					return
				case <-sub.done:
					return
				default:
					continue
				}
			}
		}

	default: // OverflowPolicyDropNewest
		select {
		case sub.ch <- event:
		case <-sub.done:
		default:
		}
	}
}

// close closes all subscriptions and prevents new ones.
func (w *watchManager) close(join func()) {
	w.mu.Lock()
	w.closed = true
	subscribers := w.subscribers
	w.subscribers = nil
	for sub := range subscribers {
		sub.stopAdmission()
	}
	w.mu.Unlock()
	join()
	for sub := range subscribers {
		sub.finish()
	}
}
