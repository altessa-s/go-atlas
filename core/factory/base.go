// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"fmt"
	"log/slog"

	"github.com/altessa-s/go-atlas/core/types/nilcheck"
)

// Base provides common factory functionality including structured logging,
// dependency validation, and standardized error formatting. Concrete factories
// embed this struct to inherit these capabilities without reimplementing them.
//
// Base is not safe for concurrent modification after construction. However,
// its methods are safe to call concurrently once the struct is initialized
// via [NewBase], since they only read from the stored logger and perform
// stateless operations.
//
// Example usage:
//
//	type MyFactory struct {
//	    factory.Base
//	    // additional fields...
//	}
//
//	func New(opts ...Option) *MyFactory {
//	    cfg := newOptions(opts...)
//	    return &MyFactory{
//	        Base: factory.NewBase(cfg.logger),
//	    }
//	}
type Base struct {
	logger *slog.Logger
}

// NewBase creates a new [Base] initialized with the given logger. If logger is
// nil, a no-op discard logger is used instead, guaranteeing that [Base.Logger]
// never returns nil.
func NewBase(logger *slog.Logger) Base {
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	return Base{
		logger: logger,
	}
}

// Logger returns the [slog.Logger] configured at construction time. The
// returned logger is guaranteed to be non-nil when [Base] was created through
// [NewBase].
func (b *Base) Logger() *slog.Logger {
	return b.logger
}

// RequireDependency validates that dep is not nil, returning an error of the
// form "<depName> is required" when the check fails. The nil check uses
// [nilcheck.IsNil], which correctly detects nil interface values that hold a
// nil concrete pointer (the common Go interface-nil pitfall).
func (b *Base) RequireDependency(dep any, depName string) error {
	if nilcheck.IsNil(dep) {
		return fmt.Errorf("%s is required", depName)
	}
	return nil
}

// RequireAllDependencies validates every entry in deps by delegating to
// [Base.RequireDependency]. It returns the first error encountered, or nil if
// all dependencies are present. Because deps is a map, iteration order is
// nondeterministic, so the specific dependency reported in the error may vary
// across calls when multiple dependencies are nil.
func (b *Base) RequireAllDependencies(deps map[string]any) error {
	for name, dep := range deps {
		if err := b.RequireDependency(dep, name); err != nil {
			return err
		}
	}
	return nil
}

// Errorf returns a new error formatted with [fmt.Errorf] semantics. Unlike
// [Base.WrapError], this method does not use the %w verb, so the returned
// error does not support unwrapping via [errors.Is] or [errors.As]. Use
// [Base.WrapError] when the original error's identity must be preserved.
func (b *Base) Errorf(format string, args ...any) error {
	return fmt.Errorf(format, args...)
}

// WrapError wraps err with a descriptive msg prefix using the %w verb, so the
// returned error supports unwrapping via [errors.Is] and [errors.As]. If err
// is nil, WrapError returns nil, making it safe to call unconditionally.
func (b *Base) WrapError(err error, msg string) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", msg, err)
}
