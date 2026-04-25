// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package recovery

import (
	"context"
	"sync"

	"github.com/altessa-s/go-atlas/transport/broker"
)

// ManagedSubscription represents a subscription managed by the recovery [Manager].
// It wraps a [broker.Subscriber] and enables automatic recovery when the
// underlying consumer or stream is deleted.
//
// All exported methods are safe for concurrent use.
type ManagedSubscription struct {
	manager      *Manager
	stream       string
	consumerName string
	handler      broker.SubscriberHandler
	factory      broker.SubscriberFactory

	mu         sync.RWMutex
	subscriber broker.Subscriber
	closed     bool
}

// Stream returns the name of the stream this subscription is attached to.
func (s *ManagedSubscription) Stream() string {
	return s.stream
}

// Consumer returns the name of the consumer.
func (s *ManagedSubscription) Consumer() string {
	return s.consumerName
}

// Subject returns the subject/topic for this subscription.
func (s *ManagedSubscription) Subject() string {
	return s.handler.Topic()
}

// IsClosed returns true if the subscription has been closed.
func (s *ManagedSubscription) IsClosed() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.closed
}

// Unsubscribe stops the subscription and removes it from the manager.
// After calling Unsubscribe, the subscription will no longer be recovered
// if the consumer is deleted.
func (s *ManagedSubscription) Unsubscribe() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return
	}

	s.closed = true

	if s.subscriber != nil {
		s.subscriber.Unsubscribe()
	}

	// Remove from registry so it won't be recovered.
	s.manager.registry.UnregisterSubscription(s.stream, s.consumerName)
}

// Closed returns a channel that closes when the subscription has fully stopped.
func (s *ManagedSubscription) Closed() <-chan struct{} {
	s.mu.RLock()
	sub := s.subscriber
	s.mu.RUnlock()

	if sub == nil {
		ch := make(chan struct{})
		close(ch)
		return ch
	}
	return sub.Closed()
}

// resubscribe recreates the subscriber and re-establishes the subscription.
// This is called by the supervisor during recovery.
func (s *ManagedSubscription) resubscribe(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return ErrManagerClosed
	}

	// Unsubscribe old subscriber if exists.
	if s.subscriber != nil {
		s.subscriber.Unsubscribe()
	}

	// Create new subscriber via factory.
	s.subscriber = s.factory(s.manager.provider)

	// Subscribe with the handler.
	return s.subscriber.Subscribe(ctx, s.handler)
}

// setSubscriber updates the subscriber.
// Used during initial subscription setup.
func (s *ManagedSubscription) setSubscriber(sub broker.Subscriber) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.subscriber = sub
}
