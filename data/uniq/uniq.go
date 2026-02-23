// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package uniq

import (
	"context"
	"errors"
	"fmt"

	"github.com/altessa-s/go-atlas/core/encoding/serializer"
	"github.com/altessa-s/go-atlas/data/uniq/providers"
	"github.com/altessa-s/go-atlas/data/uniq/providers/nats"
	"github.com/altessa-s/go-atlas/data/uniq/providers/noop"
	"github.com/altessa-s/go-atlas/data/uniq/providers/redis"

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

	return &Uniq{
		provider:   p,
		serializer: options.serializer,
	}
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
	return s.provider.Add(ctx, key)
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
	return s.provider.AddWithValue(ctx, key, data)
}

// GetValue retrieves the value associated with a key.
// The out parameter must be a pointer.
func (s *Uniq) GetValue(ctx context.Context, key string, out any) error {
	if err := validateKey(key); err != nil {
		return err
	}
	val, err := s.provider.GetValue(ctx, key)
	if err != nil {
		return err
	}

	if val == nil {
		return ErrDoesNotExist
	}

	err = s.serializer.Deserialize(val, out)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrDeserializationFailed, err)
	}
	return nil
}

// Exist checks if a key exists in the set.
func (s *Uniq) Exist(ctx context.Context, key string) (bool, error) {
	if err := validateKey(key); err != nil {
		return false, err
	}
	return s.provider.Exist(ctx, key)
}

// Remove removes a key from the set. No error if key does not exist.
func (s *Uniq) Remove(ctx context.Context, key string) error {
	if err := validateKey(key); err != nil {
		return err
	}
	return s.provider.Remove(ctx, key)
}

// Clear removes all keys from the set.
func (s *Uniq) Clear(ctx context.Context) error {
	return s.provider.Clear(ctx)
}

func validateKey(key string) error {
	// Fast inline validation - most efficient for simple cases
	if len(key) == 0 {
		return fmt.Errorf("%w: key cannot be empty", ErrInvalidKey)
	}
	if len(key) > maxKeyLength {
		return fmt.Errorf("%w: key length %d exceeds maximum %d", ErrInvalidKey, len(key), maxKeyLength)
	}
	return nil
}
