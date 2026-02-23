// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package modifiers

import "reflect"

// ApplyStringModifier applies a string transformation function to a reflect.Value.
// Handles both string and *string types with zero-copy optimizations.
// Returns the original value for non-string types.
//
// Example:
//
//	return ApplyStringModifier(v, func(s string) (string, bool) { return strings.ToLower(s), true })
func ApplyStringModifier(v reflect.Value, transformFunc StringTransformFunc) ModifierResult {
	switch v.Kind() {
	case reflect.String:
		return applyToString(v, transformFunc)
	case reflect.Pointer:
		return applyToPointer(v, transformFunc)
	default:
		// For other types, return unchanged
		return NewModifierResult(v, nil)
	}
}

// applyToString handles the string case
func applyToString(v reflect.Value, transformFunc StringTransformFunc) ModifierResult {
	str := v.String()
	if str == "" {
		return NewModifierResult(v, nil)
	}

	transformedStr, changed := transformFunc(str)
	if !changed {
		return NewModifierResult(v, nil)
	}

	// Reuse the existing reflect.Value if it can be set
	if v.CanSet() {
		v.SetString(transformedStr)
		return NewModifierResult(v, nil)
	}
	// Only create new reflect.Value if we can't set the original
	return NewModifierResult(reflect.ValueOf(transformedStr), nil)
}

// applyToPointer handles the *string case
func applyToPointer(v reflect.Value, transformFunc StringTransformFunc) ModifierResult {
	if v.IsNil() {
		return NewModifierResult(v, nil)
	}

	// Handle *string
	if v.Type().Elem().Kind() == reflect.String {
		if strPtr, ok := v.Interface().(*string); ok && strPtr != nil {
			if *strPtr == "" {
				return NewModifierResult(v, nil)
			}

			transformedStr, changed := transformFunc(*strPtr)
			if !changed {
				return NewModifierResult(v, nil)
			}

			// Reuse the existing pointer value if possible
			*strPtr = transformedStr
			return NewModifierResult(v, nil)
		}
	}

	return NewModifierResult(v, nil)
}
