// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package modifiers

import "reflect"

func init() {
	RegisterModifier("nil_on_empty", NilOnEmpty)
}

// NilOnEmpty returns nil if the input string is nil or empty.
// Only works with *string fields; string fields are returned unchanged.
//
// Example:
//
//	type User struct { Email *string `normalize:"trim,nil_on_empty"` }
func NilOnEmpty(v reflect.Value, _ map[string]string) ModifierResult {
	switch v.Kind() {
	case reflect.String:
		// For string fields, cannot make them nil, return unchanged
		return NewModifierResult(v, nil)

	case reflect.Pointer:
		if v.IsNil() {
			return NewModifierResult(v, nil)
		}

		// Handle *string
		if v.Type().Elem().Kind() == reflect.String {
			if strPtr, ok := v.Interface().(*string); ok && strPtr != nil {
				if *strPtr == "" {
					// Return a nil *string
					nilValue := reflect.Zero(v.Type())
					return NewModifierResult(nilValue, nil)
				}
			}
		}
		return NewModifierResult(v, nil)

	default:
		// For other types, return unchanged
		return NewModifierResult(v, nil)
	}
}
