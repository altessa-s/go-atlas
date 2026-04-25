// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package uniq

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/altessa-s/go-atlas/core/encoding/serializer"
	"github.com/altessa-s/go-atlas/data/uniq/providers"
	"github.com/altessa-s/go-atlas/data/uniq/providers/nats"
	"github.com/altessa-s/go-atlas/data/uniq/providers/noop"
	"github.com/altessa-s/go-atlas/data/uniq/providers/redis"
	"github.com/altessa-s/go-atlas/observability/metrics"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
	natsio "github.com/nats-io/nats.go"
	goredis "github.com/redis/go-redis/v9"
)

// ErrSerializationFailed is returned when value serialization fails.
var ErrSerializationFailed = errors.New("serialization failed")

// ErrDeserializationFailed is returned when value deserialization fails.
var ErrDeserializationFailed = errors.New("deserialization failed")

// ErrDoesNotExist is returned when a key does not exist.
var ErrDoesNotExist = errors.New("value does not exist")

// ErrInvalidKey is returned when a key is empty or too long.
var ErrInvalidKey = errors.New("invalid key")

// maxKeyLength is the maximum allowed key length (1KB).
const maxKeyLength = 1024

// Uniq manages unique keys with optional associated values.
type Uniq struct {
	provider   providers.Provider
	serializer serializer.Serializer
	logger     *slog.Logger
	metrics    *uniqMetrics
}

var _ Uniquer = (*Uniq)(nil)

// New creates a new Uniq with the specified provider.
// Default serializer is JSON.
//
// Example:
//
//	u := uniq.New(provider)
func New(p providers.Provider, opt ...Option) *Uniq {
	options := newOptions(opt...)

	u := &Uniq{
		provider:   p,
		serializer: options.serializer,
		logger:     options.logger,
		metrics:    newUniqMetrics(options.collector),
	}

	if options.healthCoordinator != nil {
		options.healthCoordinator.RegisterService(options.healthServiceName, u)
	}

	return u
}

// NewWithNats creates a Uniq with a NATS JetStream provider.
//
// Example:
//
//	u, err := uniq.NewWithNats(conn)
func NewWithNats(conn *natsio.Conn, opt ...Option) (*Uniq, error) {
	prov, err := nats.New(conn)
	if err != nil {
		return nil, coreerrs.Provider("nats", err)
	}

	return New(prov, opt...), nil
}

// NewWithNoop creates a Uniq with a no-op provider for testing.
func NewWithNoop(opt ...Option) *Uniq {
	return New(noop.New(), opt...)
}

// NewWithRedis creates a Uniq with a Redis provider.
// Panics if client is nil.
//
// Example:
//
//	u := uniq.NewWithRedis(redisClient)
func NewWithRedis(client goredis.UniversalClient, opt ...Option) *Uniq {
	return New(redis.New(client), opt...)
}

// Add adds a key to the set. Overwrites if key already exists.
func (s *Uniq) Add(ctx context.Context, key string) error {
	if err := validateKey(key); err != nil {
		return err
	}
	labels := metrics.Labels{"op": "add"}
	s.metrics.operationsTotal.WithLabels(labels).Inc()
	stop := s.metrics.operationDuration.WithLabels(labels).Start()
	err := s.provider.Add(ctx, key)
	stop()
	if err != nil {
		s.metrics.operationErrors.WithLabels(labels).Inc()
	}
	return err
}

// AddWithValue adds a key with an associated value.
func (s *Uniq) AddWithValue(ctx context.Context, key string, value any) error {
	if err := validateKey(key); err != nil {
		return err
	}
	data, err := s.serializer.Serialize(value)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrSerializationFailed, err)
	}
	labels := metrics.Labels{"op": "add_with_value"}
	s.metrics.operationsTotal.WithLabels(labels).Inc()
	stop := s.metrics.operationDuration.WithLabels(labels).Start()
	err = s.provider.AddWithValue(ctx, key, data)
	stop()
	if err != nil {
		s.metrics.operationErrors.WithLabels(labels).Inc()
	}
	return err
}

// GetValue retrieves the value associated with a key.
// The out parameter must be a pointer.
func (s *Uniq) GetValue(ctx context.Context, key string, out any) error {
	if err := validateKey(key); err != nil {
		return err
	}
	labels := metrics.Labels{"op": "get_value"}
	s.metrics.operationsTotal.WithLabels(labels).Inc()
	stop := s.metrics.operationDuration.WithLabels(labels).Start()
	val, err := s.provider.GetValue(ctx, key)
	stop()
	if err != nil {
		s.metrics.operationErrors.WithLabels(labels).Inc()
		return err
	}

	if val == nil {
		return ErrDoesNotExist
	}

	if err := s.serializer.Deserialize(val, out); err != nil {
		s.metrics.operationErrors.WithLabels(labels).Inc()
		return fmt.Errorf("%w: %w", ErrDeserializationFailed, err)
	}
	return nil
}

// Exist checks if a key exists in the set.
func (s *Uniq) Exist(ctx context.Context, key string) (bool, error) {
	if err := validateKey(key); err != nil {
		return false, err
	}
	labels := metrics.Labels{"op": "exist"}
	s.metrics.operationsTotal.WithLabels(labels).Inc()
	stop := s.metrics.operationDuration.WithLabels(labels).Start()
	exists, err := s.provider.Exist(ctx, key)
	stop()
	if err != nil {
		s.metrics.operationErrors.WithLabels(labels).Inc()
	}
	return exists, err
}

// Remove removes a key from the set. No error if key does not exist.
func (s *Uniq) Remove(ctx context.Context, key string) error {
	if err := validateKey(key); err != nil {
		return err
	}
	labels := metrics.Labels{"op": "remove"}
	s.metrics.operationsTotal.WithLabels(labels).Inc()
	stop := s.metrics.operationDuration.WithLabels(labels).Start()
	err := s.provider.Remove(ctx, key)
	stop()
	if err != nil {
		s.metrics.operationErrors.WithLabels(labels).Inc()
	}
	return err
}

// Clear removes all keys from the set.
func (s *Uniq) Clear(ctx context.Context) error {
	labels := metrics.Labels{"op": "clear"}
	s.metrics.operationsTotal.WithLabels(labels).Inc()
	stop := s.metrics.operationDuration.WithLabels(labels).Start()
	err := s.provider.Clear(ctx)
	stop()
	if err != nil {
		s.metrics.operationErrors.WithLabels(labels).Inc()
	}
	return err
}

func validateKey(key string) error {
	// Fast inline validation - most efficient for simple cases
	if len(key) == 0 {
		return coreerrs.Wrap(ErrInvalidKey, "key cannot be empty")
	}
	if len(key) > maxKeyLength {
		return coreerrs.Wrapf(ErrInvalidKey, "key length %d exceeds maximum %d", len(key), maxKeyLength)
	}
	return nil
}
