// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package normalizer

import (
	"fmt"
	"reflect"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
)

const (
	// TagValueCustom is the special `normalize:"custom"` tag value. Fields with
	// this tag skip all built-in modifier processing; only the parent struct's
	// [CustomNormalizer.Normalize] method (if implemented) may modify them.
	TagValueCustom = "custom"
)

// Normalize applies modifiers to struct fields based on normalize tags.
// It supports nested structs, slices/arrays, and both string and *string fields.
// Returns an error if input is nil or not a pointer to a struct.
//
// Example:
//
//	type User struct { Name string `normalize:"trim,lowercase"` }
//	user := &User{Name: "  JOHN  "}
//	normalizer.Normalize(user) // user.Name becomes "john"
func Normalize(s any) error {
	if s == nil {
		return fmt.Errorf("normalizer: input cannot be nil")
	}

	v := reflect.ValueOf(s)
	if v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return fmt.Errorf("normalizer: input pointer cannot be nil")
		}
		v = v.Elem()
	} else {
		return fmt.Errorf("normalizer: input must be a pointer to a struct")
	}

	return normalizeValue(v, "")
}

// normalizeValue recursively normalizes a reflect.Value.
func normalizeValue(v reflect.Value, parentPath string) error {
	if !v.IsValid() {
		return nil
	}

	// Cache Kind() to avoid repeated reflection calls
	vKind := v.Kind()

	switch vKind {
	case reflect.Struct:
		return normalizeStructFields(v, parentPath)

	case reflect.Slice, reflect.Array:
		return normalizeArray(v, parentPath)

	case reflect.Pointer:
		// Check nil once and cache Elem() result
		if !v.IsNil() {
			vElem := v.Elem()
			if err := normalizeValue(vElem, parentPath); err != nil {
				return err
			}
		}
	default:
		// For all other types (primitives, maps, channels, etc.) do nothing
		// This is intentional - we only normalize structs and slices/arrays
	}

	return nil
}

// normalizeArray handles array/slice normalization with batch processing optimization
func normalizeArray(v reflect.Value, parentPath string) error {
	vLen := v.Len()
	if vLen == 0 {
		return nil
	}

	// Cache element type's kind once for the entire slice
	elemType := v.Type().Elem()
	elemKind := elemType.Kind()

	// Determine if elements are structs that need processing
	needsRecursion := false
	isPtrToStruct := false
	switch elemKind { //nolint:exhaustive // only Struct and Pointer need recursion
	case reflect.Struct:
		needsRecursion = true
	case reflect.Pointer:
		ptrElemKind := elemType.Elem().Kind()
		if ptrElemKind == reflect.Struct {
			needsRecursion = true
			isPtrToStruct = true
		}
	}

	if !needsRecursion {
		return nil
	}

	// For batch processing: if all elements are the same struct type,
	// we can reuse resources across all elements
	var structType reflect.Type
	if elemKind == reflect.Struct {
		structType = elemType
	} else if isPtrToStruct {
		structType = elemType.Elem()
	}

	// Get cached struct field information once for all elements
	var cache *StructFieldCache
	if structType != nil {
		cache = GetStructFieldCache(structType)
		if cache == nil {
			cache = BuildStructFieldCache(structType)
			if cache != nil {
				SetStructFieldCache(structType, cache)
			}
		}
	}

	// If we have cached field info and items to process, do batch processing
	if cache != nil && len(cache.Fields) > 0 {
		// Process all array elements with batch optimization
		return batchProcessArrayElements(v, cache, parentPath, isPtrToStruct)
	}

	// Fallback to element-by-element processing for non-struct elements
	for i := range vLen {
		elem := v.Index(i)

		// Build path efficiently using optimized path builder
		elemPath := buildArrayPath(parentPath, i)

		// Handle based on cached element kind
		if elemKind == reflect.Struct {
			if err := normalizeValue(elem, elemPath); err != nil {
				return err
			}
		} else if isPtrToStruct {
			// Check nil once and cache Elem() result
			if !elem.IsNil() {
				elemElem := elem.Elem()
				if err := normalizeValue(elemElem, elemPath); err != nil {
					return err
				}
			}
		}
	}

	return nil
}

// batchProcessArrayElements processes all array elements in a batch for better performance
func batchProcessArrayElements(v reflect.Value, cache *StructFieldCache, parentPath string, isPtrToStruct bool) error {
	vLen := v.Len()

	// Build a list of fields that actually need modifiers applied
	var fieldsWithModifiers []int
	for idx, fieldInfo := range cache.Fields {
		if len(fieldInfo.ParsedModifiers) > 0 && fieldInfo.Tag != TagValueCustom {
			fieldsWithModifiers = append(fieldsWithModifiers, idx)
		}
	}

	// Only build paths when errors occur (lazy path building)
	buildElementPath := func(index int) string {
		return buildArrayPath(parentPath, index)
	}

	// Process each element
	for i := range vLen {
		elem := v.Index(i)

		// Handle pointer dereferencing if needed
		var structValue reflect.Value
		if isPtrToStruct {
			if elem.IsNil() {
				continue
			}
			structValue = elem.Elem()
		} else {
			structValue = elem
		}

		// Apply modifiers to fields that need them
		for _, fieldIdx := range fieldsWithModifiers {
			fieldInfo := cache.Fields[fieldIdx]
			fieldValue := structValue.Field(fieldInfo.Index)

			// Skip if field can't be interfaced
			if !fieldValue.CanInterface() {
				continue
			}

			// Apply modifiers (will reuse the chain internally for efficiency)
			if err := applyTagModifiers(fieldValue, fieldInfo.ParsedModifiers); err != nil {
				// Only build path on error
				elemPath := buildElementPath(i)
				fieldPath := buildFieldPath(elemPath, fieldInfo.Name)
				return coreerrs.WrapField(err, fieldPath)
			}
		}

		// Process fields without modifiers for recursive normalization
		for _, fieldInfo := range cache.Fields {
			// Skip fields that were already processed above or have special tags
			if len(fieldInfo.ParsedModifiers) > 0 || fieldInfo.Tag == "-" || fieldInfo.Tag == TagValueCustom {
				continue
			}

			fieldValue := structValue.Field(fieldInfo.Index)
			if !fieldValue.CanInterface() {
				continue
			}

			// Recursively normalize nested structures without building paths
			if fieldInfo.Kind == reflect.Struct || fieldInfo.Kind == reflect.Slice || fieldInfo.Kind == reflect.Array ||
				(fieldInfo.IsPointer && (fieldInfo.ElemKind == reflect.Struct || fieldInfo.ElemKind == reflect.Slice || fieldInfo.ElemKind == reflect.Array)) {
				// Build lazy path only if error occurs
				if err := normalizeValue(fieldValue, ""); err != nil {
					elemPath := buildElementPath(i)
					fieldPath := buildFieldPath(elemPath, fieldInfo.Name)
					return coreerrs.WrapField(err, fieldPath)
				}
			}
		}

		// Call custom normalizer if the struct implements it
		if cache.HasCustomNormalizer {
			if err := callCustomNormalizer(structValue); err != nil {
				return err
			}
		}
	}

	return nil
}

// normalizeStructFields normalizes all fields in a struct.
func normalizeStructFields(v reflect.Value, parentPath string) error {
	t := v.Type()

	// Try to get cached struct field information
	cache := GetStructFieldCache(t)
	if cache == nil {
		// Build and cache struct field information
		cache = BuildStructFieldCache(t)
		if cache != nil {
			SetStructFieldCache(t, cache)
		} else {
			// Not a struct type, nothing to do
			return nil
		}
	}

	// Process cached fields
	for _, fieldInfo := range cache.Fields {
		fieldValue := v.Field(fieldInfo.Index)

		// Skip if field can't be interfaced (shouldn't happen for exported fields)
		if !fieldValue.CanInterface() {
			continue
		}

		// Build field path efficiently using optimized path builder
		fieldPath := buildFieldPath(parentPath, fieldInfo.Name)

		// Handle different tag values
		switch fieldInfo.Tag {
		case "-":
			// Explicitly ignore this field
			continue
		case TagValueCustom:
			// Hard stop for this field: skip modifiers and recursive normalization.
			// Only the parent struct's Normalize() (CustomNormalizer) is allowed to run.
			continue
		case "":
			// No tag, will be handled in recursive processing below
		default:
			// Apply tag-based modifiers using cached parsed modifiers
			if err := applyTagModifiers(fieldValue, fieldInfo.ParsedModifiers); err != nil {
				return coreerrs.WrapField(err, fieldPath)
			}
		}

		// Recursively normalize nested structs
		if err := normalizeValue(fieldValue, fieldPath); err != nil {
			return err
		}
	}

	// Call custom normalizer if the struct implements it (use cached information)
	if cache.HasCustomNormalizer {
		return callCustomNormalizer(v)
	}

	return nil
}

// callCustomNormalizer calls the CustomNormalizer interface on a struct.
func callCustomNormalizer(v reflect.Value) error {
	// Try to call on the value itself (if it has pointer receiver methods)
	if v.CanAddr() {
		if customNormalizer, ok := v.Addr().Interface().(CustomNormalizer); ok {
			return customNormalizer.Normalize()
		}
	}

	// Try to call on the value directly (if it has value receiver methods)
	if v.CanInterface() {
		if customNormalizer, ok := v.Interface().(CustomNormalizer); ok {
			return customNormalizer.Normalize()
		}
	}

	return nil
}
