// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package recovery

import (
	"iter"
	"sync"

	"github.com/nats-io/nats.go/jetstream"

	"github.com/altessa-s/go-atlas/transport/broker"
)

// streamEntry holds stream configuration and its recovery strategy.
type streamEntry struct {
	config   jetstream.StreamConfig
	strategy RecoveryStrategy
}

// subscriptionEntry holds subscription data for recovery.
type subscriptionEntry struct {
	stream   string
	consumer string
	handler  broker.SubscriberHandler
	factory  broker.SubscriberFactory
}

// Registry stores stream and subscription configurations for recovery.
// All exported methods are safe for concurrent use; internal state is
// protected by a [sync.RWMutex].
type Registry struct {
	mu            sync.RWMutex
	streams       map[string]*streamEntry                  // stream name -> entry
	subscriptions map[string]map[string]*subscriptionEntry // stream name -> consumer name -> entry
	handlers      map[string]map[string]func() error       // stream name -> consumer name -> resubscribe func
}

// NewRegistry creates a new empty Registry.
func NewRegistry() *Registry {
	return &Registry{
		streams:       make(map[string]*streamEntry),
		subscriptions: make(map[string]map[string]*subscriptionEntry),
		handlers:      make(map[string]map[string]func() error),
	}
}

// RegisterStream adds a stream configuration to the registry.
// If a stream with the same name already exists, it will be updated.
func (r *Registry) RegisterStream(name string, cfg jetstream.StreamConfig, strategy RecoveryStrategy) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.streams[name] = &streamEntry{
		config:   cfg,
		strategy: strategy,
	}

	// Initialize subscription map for this stream if not exists.
	if r.subscriptions[name] == nil {
		r.subscriptions[name] = make(map[string]*subscriptionEntry)
	}
	if r.handlers[name] == nil {
		r.handlers[name] = make(map[string]func() error)
	}
}

// UnregisterStream removes a stream and all its subscriptions from the registry.
func (r *Registry) UnregisterStream(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.streams, name)
	delete(r.subscriptions, name)
	delete(r.handlers, name)
}

// HasStream returns true if the stream is registered.
func (r *Registry) HasStream(name string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	_, ok := r.streams[name]
	return ok
}

// GetStreamConfig returns the stream configuration if registered.
func (r *Registry) GetStreamConfig(name string) (jetstream.StreamConfig, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	entry, ok := r.streams[name]
	if !ok {
		return jetstream.StreamConfig{}, false
	}
	return entry.config, true
}

// GetRecoveryStrategy returns the recovery strategy for a stream.
// Returns RecoveryStrategyAuto if the stream is not registered.
func (r *Registry) GetRecoveryStrategy(name string) RecoveryStrategy {
	r.mu.RLock()
	defer r.mu.RUnlock()

	entry, ok := r.streams[name]
	if !ok {
		return RecoveryStrategyAuto
	}
	return entry.strategy
}

// StreamNames returns an iterator over all registered stream names.
func (r *Registry) StreamNames() iter.Seq[string] {
	return func(yield func(string) bool) {
		r.mu.RLock()
		defer r.mu.RUnlock()

		for name := range r.streams {
			if !yield(name) {
				return
			}
		}
	}
}

// StreamCount returns the number of registered streams.
func (r *Registry) StreamCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return len(r.streams)
}

// RegisterSubscription adds a subscription to the registry for recovery.
// The stream must be registered before registering subscriptions.
func (r *Registry) RegisterSubscription(stream, consumer string, handler broker.SubscriberHandler, factory broker.SubscriberFactory) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.streams[stream]; !ok {
		return ErrStreamNotRegistered
	}

	if consumer == "" {
		return ErrInvalidConsumerConfig
	}

	if r.subscriptions[stream] == nil {
		r.subscriptions[stream] = make(map[string]*subscriptionEntry)
	}

	r.subscriptions[stream][consumer] = &subscriptionEntry{
		stream:   stream,
		consumer: consumer,
		handler:  handler,
		factory:  factory,
	}

	return nil
}

// UnregisterSubscription removes a subscription from the registry.
func (r *Registry) UnregisterSubscription(stream, consumer string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if subs, ok := r.subscriptions[stream]; ok {
		delete(subs, consumer)
	}
	if handlers, ok := r.handlers[stream]; ok {
		delete(handlers, consumer)
	}
}

// HasConsumer returns true if the consumer is registered for the stream.
func (r *Registry) HasConsumer(stream, consumer string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	subs, ok := r.subscriptions[stream]
	if !ok {
		return false
	}
	_, ok = subs[consumer]
	return ok
}

// ConsumerNames returns an iterator over all consumer names for a stream.
func (r *Registry) ConsumerNames(stream string) iter.Seq[string] {
	return func(yield func(string) bool) {
		r.mu.RLock()
		defer r.mu.RUnlock()

		subs, ok := r.subscriptions[stream]
		if !ok {
			return
		}

		for name := range subs {
			if !yield(name) {
				return
			}
		}
	}
}

// ConsumerCount returns the number of registered consumers for a stream.
func (r *Registry) ConsumerCount(stream string) int {
	r.mu.RLock()
	defer r.mu.RUnlock()

	subs, ok := r.subscriptions[stream]
	if !ok {
		return 0
	}
	return len(subs)
}

// RegisterResubscribeHandler registers a function to recreate a subscription.
// This function is called during recovery to restore the subscription.
func (r *Registry) RegisterResubscribeHandler(stream, consumer string, handler func() error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.handlers[stream] == nil {
		r.handlers[stream] = make(map[string]func() error)
	}
	r.handlers[stream][consumer] = handler
}

// GetResubscribeHandler returns the resubscribe handler for a consumer.
func (r *Registry) GetResubscribeHandler(stream, consumer string) (func() error, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	handlers, ok := r.handlers[stream]
	if !ok {
		return nil, false
	}

	handler, ok := handlers[consumer]
	return handler, ok
}

// GetStreamSubscriptions returns all subscription entries for a stream.
// Returns a copy to avoid holding the lock during iteration.
func (r *Registry) GetStreamSubscriptions(stream string) []subscriptionEntry {
	r.mu.RLock()
	defer r.mu.RUnlock()

	subs, ok := r.subscriptions[stream]
	if !ok {
		return nil
	}

	result := make([]subscriptionEntry, 0, len(subs))
	for _, entry := range subs {
		result = append(result, *entry)
	}
	return result
}

// Clear removes all registered streams and subscriptions.
func (r *Registry) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.streams = make(map[string]*streamEntry)
	r.subscriptions = make(map[string]map[string]*subscriptionEntry)
	r.handlers = make(map[string]map[string]func() error)
}
