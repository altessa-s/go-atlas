// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

//go:build !go1.26

package errors

import (
	"errors"
)

// AsType is a generic alternative to [errors.As] that returns the matched error
// value directly, eliminating the need for a separate target variable declaration.
//
// It recursively unwraps the error chain (including multi-errors from
// [errors.Join]) and supports custom As(any) bool methods.
//
// On Go 1.26+ this delegates to [errors.AsType] from the standard library.
//
// Instead of:
//
//	var target *AppError
//	if errors.As(err, &target) {
//	    fmt.Println("application error:", target)
//	}
//
// You can write:
//
//	if target, ok := errors.AsType[*AppError](err); ok {
//	    fmt.Println("application error:", target)
//	}
func AsType[E error](err error) (E, bool) {
	// Fast path: check if the error is already the target type.
	// This avoids reflection overhead in the common case.
	if e, ok := err.(E); ok {
		return e, true
	}

	var target E
	// Use the standard library's errors.As which handles unwrapping,
	// errors.Join (Unwrap() []error), and As() method correctly.
	if errors.As(err, &target) {
		return target, true
	}

	var zero E
	return zero, false
}
