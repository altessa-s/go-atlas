// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"errors"
	"fmt"
)

// Sentinel errors classify why a builder refused to build. Builders return
// them wrapped, so match with [errors.Is] rather than by message text.
var (
	// ErrConfigRequired reports that a builder got no configuration, or that
	// the section its selected backend needs is missing.
	ErrConfigRequired = errors.New("configuration is required")

	// ErrDependencyRequired reports that a dependency the configuration calls
	// for (a client, a connection, a database, a sink) was not supplied.
	ErrDependencyRequired = errors.New("dependency is required")

	// ErrNotRegistered reports that the configuration names a known backend
	// whose builder was not registered on the factory.
	ErrNotRegistered = errors.New("not registered")

	// ErrUnsupported reports that the configuration names a type, provider or
	// backend the factory does not know.
	ErrUnsupported = errors.New("unsupported")
)

// Errorf formats like [fmt.Errorf] and classifies the result as kind: the
// message is exactly the formatted text, and [errors.Is] matches kind as well
// as any error wrapped with %w.
func Errorf(kind error, format string, args ...any) error {
	return &kindError{err: fmt.Errorf(format, args...), kind: kind}
}

// kindError carries the message of err and adds kind to its chain.
type kindError struct {
	err  error
	kind error
}

func (e *kindError) Error() string { return e.err.Error() }

func (e *kindError) Unwrap() []error { return []error{e.kind, e.err} }
