// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package converter

import (
	"fmt"
	"reflect"
)

// CompileTimeTypeError is panicked (not returned) by [Converter.Convert] when
// the source and destination type combination is unsupported at runtime.
// The Original field preserves the underlying panic value for diagnosis.
type CompileTimeTypeError struct {
	Source      any
	Destination any
	Reason      string
	Original    any
}

// Error returns a detailed error message explaining the type constraint violation.
// The message includes source and destination types along with the original error.
//
// Example:
//
//	msg := err.Error() // "converter: type constraint violation: ..."
func (e CompileTimeTypeError) Error() string {
	return fmt.Sprintf("converter: type constraint violation: %s (source: %T, destination: %T, original: %v)",
		e.Reason, e.Source, e.Destination, e.Original)
}

// OverflowError is panicked by [Converter.Convert] and [PrimitiveRegistry.TryConvert]
// when a narrowing numeric conversion would lose data (e.g. int64 → int8).
// Only raised when [WithOverflowCheck] is enabled.
type OverflowError struct {
	Field string
	From  reflect.Kind
	To    reflect.Kind
	Value any
}

func (e OverflowError) Error() string {
	if e.Field != "" {
		return fmt.Sprintf("converter: integer overflow: field %q: %s value %v overflows %s",
			e.Field, e.From, e.Value, e.To)
	}
	return fmt.Sprintf("converter: integer overflow: %s value %v overflows %s",
		e.From, e.Value, e.To)
}

// Unwrap returns the original error that caused the constraint violation.
// Returns nil if the original error is not an error type.
//
// Example:
//
//	if underlying := errors.Unwrap(err); underlying != nil { ... }
func (e CompileTimeTypeError) Unwrap() error {
	if err, ok := e.Original.(error); ok {
		return err
	}
	return nil
}
