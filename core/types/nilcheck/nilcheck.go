// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package nilcheck

import (
	"reflect"
	"unicode"
)

// IsNil reports whether v is nil or holds a nil pointer, map, slice, channel, or
// function. Unlike a plain == nil check, IsNil detects the common Go pitfall where
// a non-nil interface wraps a nil concrete value. It uses reflection to inspect the
// underlying value and handles nested interfaces recursively.
// See also [IsNotNil] for the inverse check and [IsNilValue] for reflect.Value inputs.
//
// Example:
//
//	var p *MyStruct = nil
//	var i any = p
//	i != nil           // true (interface is not nil)
//	IsNil(i)           // true (underlying value is nil)
func IsNil(v any) bool {
	if v == nil {
		return true
	}

	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Ptr, reflect.Map, reflect.Slice, reflect.Chan, reflect.Func:
		return rv.IsNil()
	case reflect.Interface:
		if rv.IsNil() {
			return true
		}
		// Recursive check for nested interfaces
		return IsNil(rv.Elem().Interface())
	default:
		return false
	}
}

// IsNotNil reports whether v is a non-nil value that does not wrap a nil pointer,
// map, slice, channel, or function. It is the logical inverse of [IsNil] and is
// provided for readability in guard clauses.
//
// Example:
//
//	if nilcheck.IsNotNil(handler) {
//	    handler.Handle(ctx)
//	}
func IsNotNil(v any) bool {
	return !IsNil(v)
}

// IsNilValue reports whether a reflect.Value represents a nil value. Returns true
// if v is invalid (zero Value) or if v holds a nil pointer, interface, map, slice,
// channel, or function. Unlike [IsNil], this accepts a reflect.Value directly,
// avoiding the boxing overhead of converting to any. For non-nillable kinds (structs,
// integers, etc.), it always returns false. See also [IsNotNilValue].
//
// Example:
//
//	var p *MyStruct = nil
//	rv := reflect.ValueOf(p)
//	IsNilValue(rv)  // true
func IsNilValue(v reflect.Value) bool {
	if !v.IsValid() {
		return true
	}
	switch v.Kind() {
	case reflect.Ptr, reflect.Interface, reflect.Map, reflect.Slice, reflect.Chan, reflect.Func:
		return v.IsNil()
	default:
		return false
	}
}

// IsNotNilValue reports whether a reflect.Value represents a valid, non-nil value.
// It is the logical inverse of [IsNilValue] and is provided for readability in
// guard clauses that operate on reflected values.
//
// Example:
//
//	rv := reflect.ValueOf(handler)
//	if nilcheck.IsNotNilValue(rv) {
//	    handler.Handle(ctx)
//	}
func IsNotNilValue(v reflect.Value) bool {
	return !IsNilValue(v)
}

// IsEmptyValue reports whether a reflect.Value is considered empty. A value is
// empty if it is invalid (zero reflect.Value), is the zero value for its type,
// or -- for *string pointers -- if the pointed-to string is empty or contains
// only whitespace characters (as defined by unicode.IsSpace). This is useful
// for validation logic that treats whitespace-only strings as absent.
//
// Example:
//
//	rv := reflect.ValueOf("")
//	IsEmptyValue(rv)  // true
//
//	rv2 := reflect.ValueOf((*string)(nil))
//	IsEmptyValue(rv2)  // true
//
//	var s = "  "
//	rv3 := reflect.ValueOf(&s)
//	IsEmptyValue(rv3)  // true (whitespace-only string)
func IsEmptyValue(v reflect.Value) bool {
	if !v.IsValid() {
		return true
	}

	// For zero values, return true
	if v.IsZero() {
		return true
	}

	// For non-nil *string, check if the string itself is empty (including whitespace)
	if v.Kind() == reflect.Pointer && v.Type().Elem().Kind() == reflect.String {
		if strPtr, ok := v.Interface().(*string); ok && strPtr != nil {
			return isStringEmptyOrWhitespace(*strPtr)
		}
	}

	return false
}

// isStringEmptyOrWhitespace checks if a string is empty or contains only whitespace.
// Uses unicode.IsSpace from stdlib to comply with core/ package stdlib-only policy.
func isStringEmptyOrWhitespace(s string) bool {
	if len(s) == 0 {
		return true
	}
	for _, r := range s {
		if !unicode.IsSpace(r) {
			return false
		}
	}
	return true
}

// RequireNotNil validates that value is not nil using [IsNil], returning a
// [*RequiredError] if the check fails. The fieldName identifies the dependency
// in the error message. Returns nil when value is non-nil. For validating
// multiple dependencies at once, consider using [Checker] instead.
//
// Example:
//
//	if err := nilcheck.RequireNotNil(handler, "handler"); err != nil {
//	    return err
//	}
func RequireNotNil(value any, fieldName string) error {
	if IsNil(value) {
		return NewRequiredError(fieldName)
	}
	return nil
}
