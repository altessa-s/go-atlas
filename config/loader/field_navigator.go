// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package loader

import (
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"
)

// findFieldByPath dynamically locates a field within the configuration structure
// using the configured delimiter. This is used for nested array elements that are
// populated at runtime and not present in the static field list.
func (cf *Config) findFieldByPath(fieldPath string) *field {
	if cf.conf == nil {
		return nil
	}

	// Split the path into components using configured section delimiter
	delimiter := cf.options.envSectionDelimiter
	parts := strings.Split(fieldPath, delimiter)
	if len(parts) == 0 {
		return nil
	}

	// Start with the root configuration value
	currentValue := reflect.ValueOf(cf.conf)
	if currentValue.Kind() == reflect.Pointer {
		currentValue = currentValue.Elem()
	}

	// Navigate through each path component, recording the steps taken.
	// entryMap, entryKey and entryCopy track the last map entry held by value:
	// it is navigated as an addressable copy that must be stored back.
	var (
		steps                         []pathStep
		entryMap, entryKey, entryCopy reflect.Value
	)
	for i, part := range parts {
		if !currentValue.IsValid() {
			return nil
		}

		// Check if this part is an array index
		if arrayIndex, err := strconv.Atoi(part); err == nil {
			// This is an array index - the current value should already be a slice from previous iteration
			if currentValue.Kind() != reflect.Slice {
				return nil
			}

			// Check if the array index is valid
			if arrayIndex < 0 || arrayIndex >= currentValue.Len() {
				return nil
			}

			// Move to the array element
			steps = withStep(steps, pathStep{kind: elemStep, index: arrayIndex})
			currentValue = currentValue.Index(arrayIndex)
			if currentValue.Kind() == reflect.Pointer {
				currentValue = currentValue.Elem()
			}
			continue
		}

		// Check if current value is a map
		if currentValue.Kind() == reflect.Map {
			// Initialize map if nil
			if currentValue.IsNil() && currentValue.CanSet() {
				currentValue.Set(reflect.MakeMap(currentValue.Type()))
			}

			// Get the map value for this key
			mapKey := reflect.ValueOf(part)
			mapValue := currentValue.MapIndex(mapKey)

			if !mapValue.IsValid() {
				// Create new value for this key
				mapValueType := currentValue.Type().Elem()
				newMapValue := reflect.New(mapValueType).Elem()

				// Initialize if it's a pointer
				if mapValueType.Kind() == reflect.Pointer {
					newMapValue.Set(reflect.New(mapValueType.Elem()))
				}

				currentValue.SetMapIndex(mapKey, newMapValue)
				mapValue = currentValue.MapIndex(mapKey)
			}
			steps = withStep(steps, pathStep{kind: keyStep, key: part})

			// If this is the last part, check if we need to access a field within the map value
			if i == len(parts)-1 {
				// Dereference if it's a pointer
				actualValue := mapValue
				var parentStructValue reflect.Value

				if actualValue.Kind() == reflect.Pointer {
					if actualValue.IsNil() {
						// Initialize the pointer
						newValue := reflect.New(actualValue.Type().Elem())
						currentValue.SetMapIndex(mapKey, newValue)
						actualValue = newValue
					}
					// Save the pointer value as parent (before dereferencing)
					parentStructValue = actualValue
					actualValue = actualValue.Elem()
				} else {
					// Not a pointer, save the struct value directly
					parentStructValue = actualValue
				}

				// Store references to parent map and key for later updates
				parentMapCopy := currentValue
				mapKeyCopy := mapKey

				// Return a field descriptor that can set values back to the map value
				// We return the actualValue which is the dereferenced struct
				return &field{
					name:        part,
					fullName:    fieldPath,
					value:       actualValue,
					field:       reflect.StructField{},
					tags:        make(map[string]string),
					parentMap:   &parentMapCopy,
					mapKey:      &mapKeyCopy,
					parentValue: &parentStructValue,
					steps:       steps,
				}
			}

			// Navigate into the map value. A value held by value is not
			// addressable: navigate a copy and store it back later.
			if mapValue.Kind() != reflect.Pointer {
				entryMap, entryKey = currentValue, mapKey
				entryCopy = reflect.New(mapValue.Type()).Elem()
				entryCopy.Set(mapValue)
				mapValue = entryCopy
			}
			currentValue = mapValue
			if currentValue.Kind() == reflect.Pointer {
				if currentValue.IsNil() && currentValue.CanSet() {
					currentValue.Set(reflect.New(currentValue.Type().Elem()))
				}
				currentValue = currentValue.Elem()
			}
			continue
		}

		// This must be a struct field access
		if currentValue.Kind() != reflect.Struct {
			return nil
		}

		// Use findFieldIndex for proper field matching
		fieldValue, index, found := cf.findFieldIndex(currentValue, part)
		if !found {
			return nil
		}
		steps = indexSteps(steps, index)

		// If this is the last part and it's a slice, create a field descriptor for it.
		// Resolve struct field metadata via recursive walk so slices declared inside
		// `,inline` embedded structs (e.g. BaseGrpcInterceptorConfig.IgnorePatterns)
		// also work — direct iteration over NumField() would miss them.
		if i == len(parts)-1 && fieldValue.Kind() == reflect.Slice {
			structField, foundField := cf.findStructFieldRecursive(currentValue.Type(), part)
			if !foundField {
				// Fall back to a synthetic StructField — setSliceElements only
				// reads .value, so this is sufficient to set the slice.
				structField = reflect.StructField{Name: part, Type: fieldValue.Type()}
			}

			fld := &field{
				name:     structField.Name,
				fullName: fieldPath,
				value:    fieldValue,
				field:    structField,
				tags:     make(map[string]string),
				steps:    steps,
			}
			if entryCopy.IsValid() {
				// ensureSliceSize stores the updated entry back into its map.
				fld.parentMap, fld.mapKey, fld.parentValue = &entryMap, &entryKey, &entryCopy
			}
			return fld
		}

		currentValue = fieldValue
		if currentValue.Kind() == reflect.Pointer {
			// Initialize nil pointers so we can navigate into them
			if currentValue.IsNil() {
				if currentValue.CanSet() {
					newValue := reflect.New(currentValue.Type().Elem())
					currentValue.Set(newValue)
				} else {
					return nil
				}
			}
			currentValue = currentValue.Elem()
		}
	}

	return nil
}

// fieldNavigator encapsulates the logic for navigating nested struct fields.
type fieldNavigator struct {
	cf          *Config
	structValue reflect.Value
	pathParts   []string
	value       string
	// steps locates structValue from the configuration root; the navigator
	// extends it to record which value the environment set.
	steps []pathStep
}

// setNestedStructField sets a nested field value using path parts. steps
// locates structValue from the configuration root.
func (cf *Config) setNestedStructField(structValue reflect.Value, pathParts []string, value string, steps []pathStep) error {
	if len(pathParts) == 0 {
		return nil
	}
	navigator := &fieldNavigator{
		cf:          cf,
		structValue: structValue,
		pathParts:   pathParts,
		value:       value,
		steps:       steps,
	}
	return navigator.navigate()
}

// navigate performs the main navigation through nested structures.
func (fn *fieldNavigator) navigate() error {
	currentValue := fn.structValue
	steps := fn.steps

	for i, part := range fn.pathParts {
		currentValue = fn.dereferencePointer(currentValue)

		if currentValue.Kind() != reflect.Struct {
			return nil
		}

		baseName, indexOrKey, hasIndex := splitFieldNameAndIndex(part)
		fieldValue, index, found := fn.findField(currentValue, part, hasIndex, baseName)
		if !found || !fieldValue.CanSet() {
			if fn.cf.options.strict {
				return fmt.Errorf("%w: %s in path %s", ErrFieldNotFound, part, strings.Join(fn.pathParts, fn.cf.options.envSectionDelimiter))
			}
			return nil
		}
		steps = indexSteps(steps, index)

		done, err := fn.processField(fieldValue, indexOrKey, hasIndex, i, &currentValue, &steps)
		if err != nil || done {
			return err
		}
	}

	return nil
}

// dereferencePointer dereferences pointer values and initializes if nil.
func (fn *fieldNavigator) dereferencePointer(value reflect.Value) reflect.Value {
	if value.Kind() == reflect.Pointer {
		if value.IsNil() {
			value.Set(reflect.New(value.Type().Elem()))
		}
		return value.Elem()
	}
	return value
}

// findField finds a field in the struct using the appropriate name and
// returns its struct index path.
func (fn *fieldNavigator) findField(currentValue reflect.Value, part string, hasIndex bool, baseName string) (reflect.Value, []int, bool) {
	if hasIndex {
		return fn.cf.findFieldIndex(currentValue, baseName)
	}
	return fn.cf.findFieldIndex(currentValue, part)
}

// processField processes a field based on its type and position in the path.
// It reports done when it handled the rest of the path itself; otherwise
// navigation continues from *currentValue, located by *steps.
func (fn *fieldNavigator) processField(
	fieldValue reflect.Value,
	indexOrKey string,
	hasIndex bool,
	partIndex int,
	currentValue *reflect.Value,
	steps *[]pathStep,
) (bool, error) {
	// Handle indexed slice (e.g., "BACK_OFF_0" or "BACK_OFF[0]")
	if hasIndex && fieldValue.Kind() == reflect.Slice {
		return fn.handleIndexedSlice(fieldValue, indexOrKey, partIndex, currentValue, steps)
	}

	// Handle map navigation
	if fieldValue.Kind() == reflect.Map && partIndex+1 < len(fn.pathParts) {
		return true, fn.handleMapField(fieldValue, partIndex, *steps)
	}

	// Handle slice with numeric index in next part
	if fieldValue.Kind() == reflect.Slice && partIndex+1 < len(fn.pathParts) {
		if err := fn.handleSliceWithNumericIndex(fieldValue, partIndex, *steps); !errors.Is(err, errNotNumericIndex) {
			return true, err
		}
	}

	// Handle last part or continue navigation
	if partIndex == len(fn.pathParts)-1 {
		return true, fn.cf.setFieldValue(fieldValue, fn.value, *steps)
	}

	*currentValue = fieldValue
	return false, nil
}

// handleIndexedSlice handles slice fields with index suffix in field name.
func (fn *fieldNavigator) handleIndexedSlice(
	fieldValue reflect.Value,
	indexOrKey string,
	partIndex int,
	currentValue *reflect.Value,
	steps *[]pathStep,
) (bool, error) {
	// Parse index from string
	arrayIndex, isNumeric := parseIndexOrKey(indexOrKey)
	if !isNumeric {
		return true, nil // Skip if not a numeric index
	}

	if err := fn.cf.ensureSliceSizeForValue(fieldValue, arrayIndex+1); err != nil {
		return true, err
	}
	*steps = withStep(*steps, pathStep{kind: elemStep, index: arrayIndex})

	if partIndex == len(fn.pathParts)-1 {
		return true, fn.cf.setFieldValue(fieldValue.Index(arrayIndex), fn.value, *steps)
	}

	*currentValue = fieldValue.Index(arrayIndex)
	return false, nil
}

// handleMapField handles map field navigation. steps locates the map.
func (fn *fieldNavigator) handleMapField(fieldValue reflect.Value, partIndex int, steps []pathStep) error {
	if fieldValue.IsNil() {
		fieldValue.Set(reflect.MakeMap(fieldValue.Type()))
	}

	mapKey := fn.pathParts[partIndex+1]
	mapValueType := fieldValue.Type().Elem()
	entrySteps := withStep(steps, pathStep{kind: keyStep, key: mapKey})

	if fn.isMapValuePrimitive(mapValueType) {
		return fn.handleMapWithPrimitiveValue(fieldValue, mapKey, mapValueType, partIndex, entrySteps)
	}
	return fn.handleMapWithStructValue(fieldValue, mapKey, mapValueType, partIndex, entrySteps)
}

// isMapValuePrimitive checks if map value type is primitive.
func (fn *fieldNavigator) isMapValuePrimitive(valueType reflect.Type) bool {
	return valueType.Kind() != reflect.Struct &&
		(valueType.Kind() != reflect.Pointer || valueType.Elem().Kind() != reflect.Struct)
}

// handleMapWithPrimitiveValue handles maps with primitive values. steps
// locates the entry.
func (fn *fieldNavigator) handleMapWithPrimitiveValue(
	fieldValue reflect.Value,
	mapKey string,
	mapValueType reflect.Type,
	partIndex int,
	steps []pathStep,
) error {
	if partIndex+2 != len(fn.pathParts) {
		return nil
	}

	mapKeyValue := reflect.ValueOf(mapKey)
	newValue := reflect.New(mapValueType).Elem()
	if _, err := set(newValue, fn.value, false, true, fn.cf.options.strict); err != nil {
		return err
	}
	fieldValue.SetMapIndex(mapKeyValue, newValue)
	fn.cf.markEnv(steps)
	return nil
}

// handleMapWithStructValue handles maps with struct values. Map values are not
// addressable, so an existing entry is copied, updated and stored back. steps
// locates the entry.
func (fn *fieldNavigator) handleMapWithStructValue(
	fieldValue reflect.Value,
	mapKey string,
	mapValueType reflect.Type,
	partIndex int,
	steps []pathStep,
) error {
	mapKeyValue := reflect.ValueOf(mapKey)
	structValue := reflect.New(mapValueType).Elem()
	if existingValue := fieldValue.MapIndex(mapKeyValue); existingValue.IsValid() {
		structValue.Set(existingValue)
	}

	remainingParts := fn.pathParts[partIndex+2:]
	if err := fn.cf.setNestedStructField(structValue, remainingParts, fn.value, steps); err != nil {
		return err
	}

	fieldValue.SetMapIndex(mapKeyValue, structValue)
	return nil
}

var errNotNumericIndex = errors.New("not a numeric index")

// handleSliceWithNumericIndex handles slices with numeric index in the path.
// steps locates the slice.
func (fn *fieldNavigator) handleSliceWithNumericIndex(fieldValue reflect.Value, partIndex int, steps []pathStep) error {
	index, err := strconv.Atoi(fn.pathParts[partIndex+1])
	if err != nil {
		return errNotNumericIndex
	}

	elemSteps := withStep(steps, pathStep{kind: elemStep, index: index})
	elementType := fieldValue.Type().Elem()
	if fn.isSliceElementPrimitive(elementType) {
		return fn.handlePrimitiveSlice(fieldValue, index, partIndex, elemSteps)
	}
	return fn.handleStructSlice(fieldValue, index, partIndex, elemSteps)
}

// isSliceElementPrimitive checks if slice element type is primitive.
func (fn *fieldNavigator) isSliceElementPrimitive(elementType reflect.Type) bool {
	return elementType.Kind() != reflect.Struct &&
		(elementType.Kind() != reflect.Pointer || elementType.Elem().Kind() != reflect.Struct)
}

// handlePrimitiveSlice handles slices with primitive elements. steps locates
// the element.
func (fn *fieldNavigator) handlePrimitiveSlice(fieldValue reflect.Value, index, partIndex int, steps []pathStep) error {
	if err := fn.cf.ensureSliceSizeForValue(fieldValue, index+1); err != nil {
		return err
	}

	if partIndex+2 != len(fn.pathParts) {
		return nil
	}

	return fn.cf.setFieldValue(fieldValue.Index(index), fn.value, steps)
}

// handleStructSlice handles slices with struct elements. steps locates the
// element.
func (fn *fieldNavigator) handleStructSlice(fieldValue reflect.Value, index, partIndex int, steps []pathStep) error {
	if err := fn.cf.ensureSliceSizeForValue(fieldValue, index+1); err != nil {
		return err
	}

	remainingParts := fn.pathParts[partIndex+2:]
	return fn.cf.setNestedStructField(fieldValue.Index(index), remainingParts, fn.value, steps)
}

// setNestedFieldValue navigates through nested fields and sets the final
// value. steps locates fld from the configuration root.
func (cf *Config) setNestedFieldValue(fld *field, pathParts []string, value string, steps []pathStep) error {
	if len(pathParts) == 0 {
		return nil
	}

	currentValue := fld.value

	// Navigate through the path
	for i, part := range pathParts {
		currentValue = cf.dereferencePointer(currentValue)

		// Check if current value is a map
		if currentValue.Kind() == reflect.Map {
			// Initialize map if nil
			if currentValue.IsNil() {
				currentValue.Set(reflect.MakeMap(currentValue.Type()))
			}

			// Get map key
			mapKey := part
			mapKeyValue := reflect.ValueOf(mapKey)
			mapValueType := currentValue.Type().Elem()
			entrySteps := withStep(steps, pathStep{kind: keyStep, key: mapKey})

			// Check if this is the last part or if we need to navigate deeper
			if i == len(pathParts)-1 {
				// This is the final key, set the map value
				newValue := reflect.New(mapValueType).Elem()
				if _, err := set(newValue, value, false, true, cf.options.strict); err != nil {
					return err
				}
				currentValue.SetMapIndex(mapKeyValue, newValue)
				cf.markEnv(entrySteps)
				return nil
			}

			// Not the last part - need to navigate into map value
			// Check if map value type is struct
			if mapValueType.Kind() == reflect.Struct ||
				(mapValueType.Kind() == reflect.Pointer && mapValueType.Elem().Kind() == reflect.Struct) {
				// Map values are not addressable: update a copy of the
				// existing entry, or a new one, and store it back.
				structValue := reflect.New(mapValueType).Elem()
				if existingValue := currentValue.MapIndex(mapKeyValue); existingValue.IsValid() {
					structValue.Set(existingValue)
				}

				// Recursively set nested field in the struct
				remainingParts := pathParts[i+1:]
				// Create a temporary field descriptor for recursion
				tempField := &field{
					value: structValue,
				}
				if err := cf.setNestedFieldValue(tempField, remainingParts, value, entrySteps); err != nil {
					return err
				}

				// Set the updated struct back to the map
				currentValue.SetMapIndex(mapKeyValue, tempField.value)
				return nil
			}

			// Map value is not a struct, cannot navigate further
			return nil
		}

		if currentValue.Kind() != reflect.Struct {
			return nil // Can't navigate further
		}

		// Find the field in the current struct (including inline structs)
		fieldValue, index, found := cf.findFieldIndex(currentValue, part)
		if !found {
			if cf.options.strict {
				return fmt.Errorf("%w: %s", ErrFieldNotFound, part)
			}
			return nil
		}
		steps = indexSteps(steps, index)

		if i == len(pathParts)-1 {
			// This is the final field, set its value
			if fieldValue.CanSet() {
				return cf.setFieldValue(fieldValue, value, steps)
			}
		} else {
			// Continue navigation
			currentValue = fieldValue
		}
	}

	return nil
}

// splitFieldNameAndIndex splits a field name into the base field name and an optional array index or map key.
// Supports two syntaxes:
//  1. Square brackets: "CONSUMERS[keycloak_sync]" returns ("CONSUMERS", "keycloak_sync", true)
//     "BACK_OFF[0]" returns ("BACK_OFF", "0", true)
//  2. Double underscore (legacy): "BACK_OFF__0" returns ("BACK_OFF", "0", true)
//
// Returns: (baseName, indexOrKey, hasIndexOrKey)
func splitFieldNameAndIndex(fieldName string) (string, string, bool) {
	// First, check for square bracket syntax: FIELD[key] or FIELD[0]
	if bracketStart := strings.LastIndex(fieldName, "["); bracketStart != -1 {
		if bracketEnd := strings.LastIndex(fieldName, "]"); bracketEnd > bracketStart {
			if bracketEnd == len(fieldName)-1 {
				// Valid bracket syntax
				baseName := fieldName[:bracketStart]
				indexOrKey := fieldName[bracketStart+1 : bracketEnd]
				if indexOrKey != "" {
					return baseName, indexOrKey, true
				}
			}
		}
	}

	// Fallback to legacy double underscore syntax
	delimiter := "__"
	lastDelimiter := strings.LastIndex(fieldName, delimiter)
	if lastDelimiter == -1 || lastDelimiter == len(fieldName)-len(delimiter) {
		return fieldName, "", false
	}

	// Check if everything after the last delimiter is a number
	indexStr := fieldName[lastDelimiter+len(delimiter):]
	if _, err := strconv.Atoi(indexStr); err != nil {
		return fieldName, "", false
	}

	// Extract the base field name (everything before the last delimiter)
	baseName := fieldName[:lastDelimiter]
	return baseName, indexStr, true
}

// parseIndexOrKey converts indexOrKey string to int if it's numeric, otherwise returns -1.
// This helper function is used when numeric index is needed.
func parseIndexOrKey(indexOrKey string) (int, bool) {
	index, err := strconv.Atoi(indexOrKey)
	if err != nil {
		return -1, false
	}
	return index, true
}

// findFieldIndex searches for a field by name in a struct, including
// inline/anonymous fields, and returns it with its struct index path.
func (cf *Config) findFieldIndex(structValue reflect.Value, fieldName string) (reflect.Value, []int, bool) {
	structType := structValue.Type()

	// First pass: check regular fields
	for i := range structType.NumField() {
		field := structType.Field(i)

		if field.Anonymous {
			continue // Skip anonymous fields in the first pass
		}

		if cf.fieldMatches(field, fieldName) {
			return structValue.Field(i), []int{i}, true
		}
	}

	// Second pass: check inline/anonymous fields
	for i := range structType.NumField() {
		field := structType.Field(i)

		if !field.Anonymous {
			continue // Skip non-anonymous fields in the second pass
		}

		// Initialize and dereference the inline field
		inlineValue := cf.dereferencePointer(structValue.Field(i))

		if inlineValue.Kind() == reflect.Struct {
			// Recursively search in the inline struct
			if fieldValue, index, found := cf.findFieldIndex(inlineValue, fieldName); found {
				return fieldValue, append([]int{i}, index...), true
			}
		}
	}

	return reflect.Value{}, nil, false
}

// findStructFieldRecursive searches a struct type for a field matching name,
// following the same precedence as findFieldIndex: direct fields first,
// then `,inline` embedded structs. Returns the matching reflect.StructField
// (preserving its tags) so callers can use it as field metadata even when
// the field is declared inside an embedded struct.
func (cf *Config) findStructFieldRecursive(structType reflect.Type, fieldName string) (reflect.StructField, bool) {
	for i := range structType.NumField() {
		f := structType.Field(i)
		if f.Anonymous {
			continue
		}
		if cf.fieldMatches(f, fieldName) {
			return f, true
		}
	}
	for i := range structType.NumField() {
		f := structType.Field(i)
		if !f.Anonymous {
			continue
		}
		ft := f.Type
		if ft.Kind() == reflect.Pointer {
			ft = ft.Elem()
		}
		if ft.Kind() != reflect.Struct {
			continue
		}
		if sf, ok := cf.findStructFieldRecursive(ft, fieldName); ok {
			return sf, true
		}
	}
	return reflect.StructField{}, false
}

// fieldMatches checks if a field matches the given name based on tag or field name.
func (cf *Config) fieldMatches(field reflect.StructField, name string) bool {
	// This function doesn't check env tag, so pass empty string
	return cf.fieldNameMatches(
		field.Tag.Get(cf.options.structTag),
		"", // No env tag check in this variant
		field.Name,
		name,
	)
}

// dereferencePointer dereferences a pointer value, initializing it if nil.
func (cf *Config) dereferencePointer(value reflect.Value) reflect.Value {
	if value.Kind() == reflect.Pointer {
		if value.IsNil() {
			value.Set(reflect.New(value.Type().Elem()))
		}
		return value.Elem()
	}
	return value
}
