// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package recovery

import (
	"context"
	"fmt"
	"sync"

	"github.com/altessa-s/go-atlas/messaging/broker"

	natsprovider "github.com/altessa-s/go-atlas/messaging/broker/providers/nats"
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
	factory      natsprovider.SubscriberFactory

	mu         sync.RWMutex
	subscriber broker.Subscriber
	closed     bool
	// key is the name the registry knows this subscription by: the
	// server-side consumer name when the subscriber reports one (an
	// ephemeral consumer is named by the server), else consumerName.
	key string
	// subCancel cancels the context of a subscriber established by
	// resubscribe; nil for the initial subscriber, whose context is the
	// caller's.
	subCancel context.CancelFunc
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
	s.cancelSubscription()

	// Remove from registry so it won't be recovered.
	s.manager.registry.UnregisterSubscription(s.stream, s.key)
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
//
// The new subscription lives in a context derived from parent (the original
// Subscribe context, keeping its values and cancellation); recoveryCtx only
// bounds establishment, so canceling it (Supervisor.Close) interrupts a
// Subscribe in progress but not a subscription already established.
func (s *ManagedSubscription) resubscribe(parent, recoveryCtx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return ErrManagerClosed
	}

	// The replacement is established before the old subscriber stops, so a
	// failed attempt leaves the previous state in place for the next retry.
	next := s.factory(s.manager.provider)

	subCtx, cancel := context.WithCancel(parent)
	stop := context.AfterFunc(recoveryCtx, cancel)
	err := next.Subscribe(subCtx, s.handler)
	if !stop() {
		// Recovery was canceled during establishment: subCtx is dead, so a
		// subscriber that came up anyway would run with a canceled context.
		next.Unsubscribe()
		cancel()
		return fmt.Errorf("resubscribe: %w", recoveryCtx.Err())
	}
	if err != nil {
		cancel()
		return err
	}

	// Stop the previous subscriber first so its in-flight handlers drain with
	// a live context, then cancel that context.
	if s.subscriber != nil {
		s.subscriber.Unsubscribe()
	}
	s.cancelSubscription()
	s.subscriber = next
	s.subCancel = cancel

	// An ephemeral replacement has a new server-assigned name: retire the old
	// registry key and register the new one.
	if key := consumerKey(next, s.consumerName); key != s.key {
		s.manager.registry.RekeySubscription(s.stream, s.key, key)
		s.key = key
	}
	return nil
}

// cancelSubscription cancels the context of a subscriber established by
// resubscribe. Callers must hold s.mu.
func (s *ManagedSubscription) cancelSubscription() {
	if s.subCancel != nil {
		s.subCancel()
		s.subCancel = nil
	}
}

// setSubscriber updates the subscriber and the registry key derived from it.
// Used during initial subscription setup.
func (s *ManagedSubscription) setSubscriber(sub broker.Subscriber) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.subscriber = sub
	s.key = consumerKey(sub, s.consumerName)
}

// activeConsumerNamer is implemented by subscribers that know the server-side
// name of the consumer they consume.
type activeConsumerNamer interface {
	ActiveConsumerName() string
}

// consumerKey returns the name the registry and health checks use for sub:
// its server-side consumer name when it reports one, else fallback.
func consumerKey(sub broker.Subscriber, fallback string) string {
	if namer, ok := sub.(activeConsumerNamer); ok {
		if name := namer.ActiveConsumerName(); name != "" {
			return name
		}
	}
	return fallback
}
