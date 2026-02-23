// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package id

import (
	"errors"
	"fmt"

	"github.com/altessa-s/go-atlas/core/runtime/panics"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
)

// Provider is the interface implemented by any component that can supply a
// stable Service ID. The package ships three implementations: [Env], [File],
// and [Static]. Custom implementations must ensure that ID returns a
// consistent, non-empty value for the lifetime of the process.
type Provider interface {
	// ID returns the unique Service ID. Implementations must be safe for
	// concurrent use and should return the same value on every call once
	// initialization is complete.
	ID() string
}

// Service is the primary entry point for obtaining a Service ID. It delegates
// to an underlying [Provider] selected at construction time.
//
// Create instances with [New], [NewWithProvider], [NewWithEnvProvider],
// [NewWithFileProvider], or [NewStaticProvider] (and their Must variants).
type Service struct {
	provider Provider
}

// New creates a [Service] with cascading provider selection. When one or more
// env names are supplied, the first non-empty name is looked up in the process
// environment via [NewEnv]. If the variable is unset or empty
// ([ErrInvalidEnv]), the constructor falls back to a [File] provider bound to
// filePath.
//
// Returns an error only when both the environment lookup fails with an
// unexpected error or the [File] provider cannot be created.
//
// Example:
//
//	s, err := id.New("/tmp/service.id", "SERVICE_ID")
func New(filePath string, env ...string) (*Service, error) {
	if len(env) > 0 && env[0] != "" {
		envProvider, err := NewEnv(env[0]) // Use NewEnv directly
		if err == nil {
			return NewWithProvider(envProvider)
		}
		if !errors.Is(err, ErrInvalidEnv) {
			// Unexpected error from environment provider
			return nil, coreerrs.WrapOperation(err, "initialize environment provider")
		}
		// If ErrInvalidEnv, fall through to file provider
	}

	// Fallback to file provider
	fileProvider, err := NewFile(filePath)
	if err != nil {
		return nil, err
	}
	return NewWithProvider(fileProvider)
}

// MustNew is like [New] but panics if the [Service] cannot be created. It is
// intended for top-level initialization where failure is unrecoverable.
//
// Example:
//
//	s := id.MustNew("/tmp/service.id", "SERVICE_ID")
func MustNew(filePath string, env ...string) *Service {
	return panics.MustResult(New(filePath, env...))
}

// NewWithProvider creates a [Service] backed by a caller-supplied [Provider].
// This is useful for custom implementations or when a provider has already been
// constructed independently.
//
// Returns an error if provider is nil.
//
// Example:
//
//	s, err := id.NewWithProvider(myProvider)
func NewWithProvider(provider Provider) (*Service, error) {
	if provider == nil {
		return nil, fmt.Errorf("id: provider cannot be nil")
	}
	return &Service{provider: provider}, nil
}

// MustNewWithProvider is like [NewWithProvider] but panics if the [Service]
// cannot be created (e.g. nil provider).
//
// Example:
//
//	s := id.MustNewWithProvider(myProvider)
func MustNewWithProvider(provider Provider) *Service {
	return panics.MustResult(NewWithProvider(provider))
}

// NewWithEnvProvider creates a [Service] backed by an [Env] provider that
// reads the Service ID from the environment variable named envName.
//
// Returns [ErrInvalidEnv] if the variable is not set or is empty.
//
// Example:
//
//	s, err := id.NewWithEnvProvider("SERVICE_ID")
func NewWithEnvProvider(envName string) (*Service, error) {
	envProvider, err := NewEnv(envName)
	if err != nil {
		return nil, err
	}
	return NewWithProvider(envProvider)
}

// NewWithFileProvider creates a [Service] backed by a [File] provider that
// persists the Service ID at filePath. If the file does not exist, a new
// random ID is generated on the first call to [Service.ID] and written to
// the file.
//
// Returns an error if filePath is the empty string.
//
// Example:
//
//	s, err := id.NewWithFileProvider("/var/lib/app/service.id")
func NewWithFileProvider(filePath string) (*Service, error) {
	fileProvider, err := NewFile(filePath)
	if err != nil {
		return nil, err
	}
	return NewWithProvider(fileProvider)
}

// MustNewWithFileProvider is like [NewWithFileProvider] but panics if the
// [Service] cannot be created.
//
// Example:
//
//	s := id.MustNewWithFileProvider("/var/lib/app/service.id")
func MustNewWithFileProvider(filePath string) *Service {
	return panics.MustResult(NewWithFileProvider(filePath))
}

// NewStaticProvider creates a [Service] backed by a [Static] provider that
// always returns idVal. This is useful for testing or when the ID is known
// at compile time or from external configuration.
//
// Example:
//
//	s, err := id.NewStaticProvider("my-service-12345")
func NewStaticProvider(idVal string) (*Service, error) {
	staticProvider := NewStatic(idVal)
	return NewWithProvider(staticProvider)
}

// MustNewStaticProvider is like [NewStaticProvider] but panics if the [Service]
// cannot be created.
//
// Example:
//
//	s := id.MustNewStaticProvider("my-service-12345")
func MustNewStaticProvider(idVal string) *Service {
	return panics.MustResult(NewStaticProvider(idVal))
}

// ID delegates to the underlying [Provider] and returns the Service ID.
// Returns an empty string if the provider is nil, which should never occur
// when the [Service] is created through the package constructors.
//
// Example:
//
//	fmt.Println("Service ID:", s.ID())
func (s *Service) ID() string {
	if s.provider == nil {
		// Return empty string instead of panicking
		// This should not happen if constructors are used correctly
		return ""
	}
	return s.provider.ID()
}
