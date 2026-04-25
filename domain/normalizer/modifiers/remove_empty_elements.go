// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package modifiers

import (
	"reflect"

	corestrings "github.com/altessa-s/go-atlas/core/text/strings"
)

func init() {
	// Note: This modifier is handled specially for slice types in applyModifierChainTyped
	RegisterModifier("remove_empty_elements", RemoveEmptyElements)
}

// RemoveEmptyElements removes empty and nil elements from string slices.
// Works with []string and []*string slices. Non-slice types are returned unchanged.
//
// Example:
//
//	type Data struct { Tags []string `normalize:"remove_empty_elements"` }
func RemoveEmptyElements(v reflect.Value, _ map[string]string) ModifierResult {
	switch v.Kind() {
	case reflect.Slice:
		// Handle slice types - remove empty elements
		RemoveEmptyElementsFromSlice(v)
		return NewModifierResult(v, nil)

	default:
		// For non-slice types, return original value unchanged
		return NewModifierResult(v, nil)
	}
}

// RemoveEmptyElementsFromSlice removes empty and nil elements from string slices.
// For []string: removes empty strings after trimming whitespace.
// For []*string: removes nil pointers and pointers to empty strings.
// Returns true if the slice was modified, false otherwise.
func RemoveEmptyElementsFromSlice(v reflect.Value) bool {
	if v.Kind() != reflect.Slice {
		return false
	}

	elemType := v.Type().Elem()

	// Check if it's a string slice or pointer to string slice
	isStringSlice := elemType.Kind() == reflect.String
	isStringPtrSlice := elemType.Kind() == reflect.Pointer && elemType.Elem().Kind() == reflect.String

	if !isStringSlice && !isStringPtrSlice {
		return false
	}

	originalLen := v.Len()
	newSlice := reflect.MakeSlice(v.Type(), 0, originalLen)

	for i := range originalLen {
		elem := v.Index(i)

		// Check if element should be kept
		if shouldKeepElement(elem, isStringPtrSlice) {
			newSlice = reflect.Append(newSlice, elem)
		}
	}

	// Only update if something was removed
	if newSlice.Len() != originalLen {
		v.Set(newSlice)
		return true
	}

	return false
}

// shouldKeepElement determines if an element should be kept in the slice
func shouldKeepElement(elem reflect.Value, isPointer bool) bool {
	if isPointer {
		// For *string elements
		if elem.IsNil() {
			return false
		}

		// Check if it's a valid *string and not empty
		if strPtr, ok := elem.Interface().(*string); ok && strPtr != nil {
			return !corestrings.IsEmptyOrWhitespace(*strPtr)
		}
		return false
	}

	// For string elements
	return !corestrings.IsEmptyOrWhitespace(elem.String())
}
