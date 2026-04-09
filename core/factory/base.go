// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"errors"
	"fmt"
	"log/slog"

	"github.com/altessa-s/go-atlas/core/types/nilcheck"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
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

// SetLogger replaces the logger stored in [Base]. If logger is nil, the call
// is a no-op so that callers do not accidentally downgrade to a nil logger.
func (b *Base) SetLogger(logger *slog.Logger) {
	if logger != nil {
		b.logger = logger
	}
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
// [Base.RequireDependency]. It aggregates all missing dependencies into a
// single error using [errors.Join], so operators see every missing
// dependency at once rather than having to fix them one at a time. Returns
// nil if all dependencies are present.
func (b *Base) RequireAllDependencies(deps map[string]any) error {
	var errs []error
	for name, dep := range deps {
		if err := b.RequireDependency(dep, name); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
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
	return coreerrs.Wrap(err, msg)
}

// JoinErrors returns nil if errs is empty, errs[0] if len(errs)==1, or
// errors.Join(errs...) otherwise. Use in builder Build methods to avoid
// variadic allocation when there are zero or one accumulated errors.
func JoinErrors(errs []error) error {
	switch len(errs) {
	case 0:
		return nil
	case 1:
		return errs[0]
	default:
		return errors.Join(errs...)
	}
}
