// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package base

import (
	"fmt"

	"golang.org/x/sync/singleflight"

	corestrings "github.com/altessa-s/go-atlas/core/text/strings"
)

// SingleflightGroup wraps singleflight.Group with convenience methods for key generation.
// It can be embedded in storage providers to prevent thundering herd problems.
//
// Example usage:
//
//	type Storage[T any] struct {
//	    base.SingleflightGroup
//	    // ... other fields
//	}
//
//	func (s *Storage[T]) List(ctx context.Context) ([]*secrets.Value[T], error) {
//	    sfKey := s.CreateKey("list", s.projectId)
//	    result, err, _ := s.Do(sfKey, func() (any, error) {
//	        return s.doList(ctx)
//	    })
//	    // ...
//	}
type SingleflightGroup struct {
	group singleflight.Group
}

// Do executes and returns the results of the given function, making sure that
// only one execution is in-flight for a given key at a time.
func (g *SingleflightGroup) Do(key string, fn func() (any, error)) (any, error, bool) {
	return g.group.Do(key, fn)
}

// DoChan is like Do but returns a channel that will receive the results when
// they are ready.
func (g *SingleflightGroup) DoChan(key string, fn func() (any, error)) <-chan singleflight.Result {
	return g.group.DoChan(key, fn)
}

// Forget tells the singleflight to forget about a key.
func (g *SingleflightGroup) Forget(key string) {
	g.group.Forget(key)
}

// CreateKey creates a singleflight key by concatenating components with colons.
// Uses pooled string builder for efficiency.
//
// Example:
//
//	key := sf.CreateKey("vault", mountPath, secretPath) // "vault:secret:path/to/secrets"
func (g *SingleflightGroup) CreateKey(components ...string) string {
	if len(components) == 0 {
		return ""
	}
	if len(components) == 1 {
		return components[0]
	}

	builder := corestrings.GetStringBuilder()
	defer corestrings.PutStringBuilder(builder)

	for i, comp := range components {
		if i > 0 {
			builder.WriteString(":")
		}
		builder.WriteString(comp)
	}

	return builder.String()
}

// DoTyped executes the function and returns the result cast to the expected type.
// This is a convenience wrapper around Do that handles type assertion.
//
// Returns an error if the result cannot be cast to the expected type.
func DoTyped[T any](g *SingleflightGroup, key string, fn func() (T, error)) (T, error) {
	var zero T

	result, err, _ := g.group.Do(key, func() (any, error) {
		return fn()
	})

	if err != nil {
		return zero, err
	}

	typed, ok := result.(T)
	if !ok {
		return zero, fmt.Errorf("unexpected result type from singleflight: got %T, want %T", result, zero)
	}

	return typed, nil
}
