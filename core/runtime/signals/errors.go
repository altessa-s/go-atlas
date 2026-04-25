// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package signals

import (
	"fmt"
	"os"
	"time"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
)

// ErrorHandler is a callback invoked by [Signal] when a registered handler
// returns an error, panics, or exceeds its timeout. It receives the OS signal
// that triggered the handler and the resulting error (which may be a
// [*TimeoutError] or [*PanicError]). Register one via [WithErrorHandler].
type ErrorHandler func(sig os.Signal, err error)

// TimeoutError is returned to the [ErrorHandler] when a [ContextHandler]
// does not complete within its configured timeout (see [WithHandlerTimeout]).
// Use [IsTimeout] to check whether an error is a TimeoutError.
type TimeoutError struct {
	// Signal is the OS signal that was being handled when the timeout occurred.
	Signal os.Signal
	// Timeout is the duration that was exceeded.
	Timeout time.Duration
}

// Error implements the error interface.
func (e *TimeoutError) Error() string {
	return fmt.Sprintf("handler timeout after %v for signal %v", e.Timeout, e.Signal)
}

// IsTimeout reports whether err (or any error in its chain) is a [*TimeoutError].
// It is intended for use within an [ErrorHandler] to distinguish timeout
// errors from other handler failures.
func IsTimeout(err error) bool {
	_, ok := coreerrs.AsType[*TimeoutError](err) //nolint:errcheck // only checking ok
	return ok
}

// PanicError is returned to the [ErrorHandler] when a [ContextHandler]
// panics during execution. The original panic value is preserved in the
// Panic field for inspection or re-throwing.
type PanicError struct {
	// Signal is the OS signal that was being handled when the panic occurred.
	Signal os.Signal
	// Panic is the value recovered from the panicking handler.
	Panic any
}

// Error implements the error interface
func (e *PanicError) Error() string {
	return fmt.Sprintf("handler panic for signal %v: %v", e.Signal, e.Panic)
}

// safeCallErrorHandler safely calls the error handler with panic recovery.
// This ensures that panics in error handlers don't crash the signal processing.
func (s *Signal) safeCallErrorHandler(sig os.Signal, err error) {
	if s.errorHandler == nil {
		return
	}

	defer func() {
		if r := recover(); r != nil {
			// Error handler panicked - log but don't propagate
			// In production, this should be logged to a monitoring system
			_ = r // Explicitly ignore panic to prevent cascading failures
		}
	}()

	s.errorHandler(sig, err)
}
