// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package id

import (
	"errors"

	"github.com/altessa-s/go-atlas/core/runtime/appinfo"
)

// ErrInvalidEnv is returned by [NewEnv] and [NewWithEnvProvider] when the
// requested environment variable is not set or contains an empty string.
// Callers of [New] can safely ignore this error because it triggers an
// automatic fallback to the [File] provider.
var ErrInvalidEnv = errors.New("id: environment variable not found or empty")

// Env is a [Provider] that resolves the Service ID from a single environment
// variable at construction time. Once created, the value is immutable and safe
// for concurrent use.
type Env struct {
	id string
}

// NewEnv creates a new [Env] provider by reading the environment variable
// identified by envName. The variable is read once during construction; later
// changes to the environment are not observed.
//
// Returns [ErrInvalidEnv] if the variable is not set or is empty.
func NewEnv(envName string) (*Env, error) {
	val := appinfo.Env(envName)
	if val == "" {
		return nil, ErrInvalidEnv
	}

	return &Env{id: val}, nil
}

// ID returns the Service ID that was captured from the environment variable
// when [NewEnv] was called. The returned value is always non-empty for a
// successfully constructed [Env].
func (s *Env) ID() string {
	return s.id
}

// Runtime check to ensure that the Env type implements the Provider interface.
var _ Provider = (*Env)(nil)
