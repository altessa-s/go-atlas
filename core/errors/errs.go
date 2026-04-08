// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package errors

import (
	"context"
	"errors"
	"fmt"
)

// IsContextDeadlineExceeded reports whether err or any error in its chain matches
// [context.DeadlineExceeded]. It uses [errors.Is] internally, so wrapped errors are
// handled transparently. Returns false when err is nil.
//
// See also [IsContextCanceled] and [IsContextCanceledOrDeadlineExceeded].
//
// Example:
//
//	if errors.IsContextDeadlineExceeded(err) { /* operation timed out */ }
func IsContextDeadlineExceeded(err error) bool {
	if err == nil {
		return false
	}
	return errors.Is(err, context.DeadlineExceeded)
}

// IsContextCanceled reports whether err or any error in its chain matches
// [context.Canceled]. It uses [errors.Is] internally, so wrapped errors are
// handled transparently. Returns false when err is nil.
//
// See also [IsContextDeadlineExceeded] and [IsContextCanceledOrDeadlineExceeded].
//
// Example:
//
//	if errors.IsContextCanceled(err) { /* operation was canceled */ }
func IsContextCanceled(err error) bool {
	if err == nil {
		return false
	}
	return errors.Is(err, context.Canceled)
}

// IsContextCanceledOrDeadlineExceeded reports whether err matches [context.Canceled] or
// [context.DeadlineExceeded]. It is a convenience shorthand that combines
// [IsContextCanceled] and [IsContextDeadlineExceeded]. Returns false when err is nil.
//
// This is useful in shutdown paths where both cancellation and deadline expiration
// should be treated identically.
//
// Example:
//
//	if errors.IsContextCanceledOrDeadlineExceeded(err) { /* context done */ }
func IsContextCanceledOrDeadlineExceeded(err error) bool {
	if err == nil {
		return false
	}
	return IsContextCanceled(err) || IsContextDeadlineExceeded(err)
}

// Wrapf wraps err with a formatted context message using [fmt.Errorf] and the %w verb,
// preserving the original error chain for [errors.Is] and [errors.As].
// Returns nil when err is nil, making it safe to call unconditionally.
//
// The format string and args describe the wrapping context; err is automatically
// appended to the argument list and formatted with ": %w".
//
// See also [Wrap] for a simpler variant without format arguments,
// and [WrapOperation] for a structured "failed to X" pattern.
//
// Example:
//
//	if err != nil {
//	    return errors.Wrapf(err, "operation failed: %s", details)
//	}
func Wrapf(err error, format string, args ...any) error {
	if err == nil {
		return nil
	}
	// Append error to format string and args
	allArgs := make([]any, len(args)+1)
	copy(allArgs, args)
	allArgs[len(args)] = err
	return fmt.Errorf(format+": %w", allArgs...)
}

// WrapOperation wraps err with a standardized "failed to <operation>: ..." message
// using the %w verb, preserving the error chain for [errors.Is] and [errors.As].
// Returns nil when err is nil, making it safe to call unconditionally.
//
// Use this for consistent, grep-friendly failure messages across the codebase.
// See also [WrapOperationWithContext] when additional context (e.g., a resource name)
// is needed.
//
// Example:
//
//	if err != nil {
//	    return errors.WrapOperation(err, "publish message")
//	}
//	// Error message: "failed to publish message: <original error>"
func WrapOperation(err error, operation string) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("failed to %s: %w", operation, err)
}

// WrapField wraps err with a "field '<fieldName>': ..." message using the %w verb,
// preserving the error chain for [errors.Is] and [errors.As].
// Returns nil when err is nil, making it safe to call unconditionally.
//
// This is intended for validation and deserialization paths where the field name
// provides essential debugging context.
//
// Example:
//
//	if err != nil {
//	    return errors.WrapField(err, "email")
//	}
//	// Error message: "field 'email': <original error>"
func WrapField(err error, fieldName string) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("field '%s': %w", fieldName, err)
}

// WrapOperationWithContext wraps err with a "failed to <operation> on <context>: ..."
// message using the %w verb, preserving the error chain for [errors.Is] and [errors.As].
// Returns nil when err is nil, making it safe to call unconditionally.
//
// Use this instead of [WrapOperation] when a resource or scope qualifier (e.g., a
// stream name, table, or endpoint) adds meaningful debugging context.
//
// Example:
//
//	if err != nil {
//	    return errors.WrapOperationWithContext(err, "create consumer", "stream 'events'")
//	}
//	// Error message: "failed to create consumer on stream 'events': <original error>"
func WrapOperationWithContext(err error, operation, context string) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("failed to %s on %s: %w", operation, context, err)
}

// Wrap wraps err with a plain context message using the %w verb, preserving the
// error chain for [errors.Is] and [errors.As]. The resulting error message has
// the form "<msg>: <original error>".
// Returns nil when err is nil, making it safe to call unconditionally.
//
// For formatted context messages use [Wrapf]; for structured operation messages
// use [WrapOperation].
//
// Example:
//
//	if err != nil {
//	    return errors.Wrap(err, "database connection failed")
//	}
func Wrap(err error, msg string) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", msg, err)
}

// JoinWrap wraps cause under a sentinel so that [errors.Is] matches both sides
// of the chain. The resulting error reports "<sentinel>: <cause>" and preserves
// both error chains via Go's dual-%w support: a caller can match the high-level
// domain sentinel and still inspect the underlying cause with [errors.Is] or
// [errors.As].
//
// Returns nil when cause is nil (the sentinel alone is rarely useful — the
// caller can return the sentinel directly for that case). Returns a wrapper
// around cause with the zero sentinel when sentinel is nil, mirroring the
// behavior of [fmt.Errorf] with a nil %w target.
//
// Use this for adapter layers that translate errors from a lower-level
// package into a domain-specific sentinel without discarding the original
// error chain — for example, the plugin sandbox wraps rlimits, capabilities,
// and landlock failures under ErrSandboxFailed while still letting callers
// match on the primitive-level sentinels.
//
// Example:
//
//	if err := landlock.Apply(...); err != nil {
//	    return errors.JoinWrap(ErrSandboxFailed, err)
//	}
//	// Both errors.Is(err, ErrSandboxFailed) and
//	// errors.Is(err, landlock.ErrUnsupported) return true.
func JoinWrap(sentinel, cause error) error {
	if cause == nil {
		return nil
	}
	return fmt.Errorf("%w: %w", sentinel, cause)
}

// Required returns an error with the message "<dependency> is required for <context>".
// Use this during initialization or construction to signal that a mandatory
// dependency was not provided.
//
// The returned error is a plain [fmt.Errorf] value; it does not wrap another error.
//
// Example:
//
//	if db == nil {
//	    return errors.Required("database", "UserRepository")
//	}
//	// Error message: "database is required for UserRepository"
func Required(dependency, context string) error {
	return fmt.Errorf("%s is required for %s", dependency, context)
}

// Provider returns an error indicating that a provider of the given type could not
// be created. The returned message has the form "failed to create <providerType>
// provider: <err>". It wraps err with the %w verb, preserving the error chain for
// [errors.Is] and [errors.As].
//
// Example:
//
//	if err != nil {
//	    return errors.Provider("Redis", err)
//	}
//	// Error message: "failed to create Redis provider: <original error>"
func Provider(providerType string, err error) error {
	return fmt.Errorf("failed to create %s provider: %w", providerType, err)
}
