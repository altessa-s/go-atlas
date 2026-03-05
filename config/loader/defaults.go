// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package loader

import (
	"reflect"

	"github.com/altessa-s/go-atlas/core/types/nilcheck"
)

// loadDefaultValues parses and sets default values for configuration fields.
// It processes both struct tags and Defaulter interface implementations.
func (cf *Config) loadDefaultValues() (err error) {
	if !cf.options.skipDefaults {
		if df, ok := cf.conf.(Defaulter); ok {
			df.Default()
		}
	}

	cf.fields = structFields(cf.conf)
	for f := range cf.fields.All() {
		if !cf.options.skipDefaults && !f.isStructPtr() {
			if err = f.setDefaultValue(defaultValueTagName, cf.options.strict); err != nil {
				return err
			}
		}

		if !cf.options.skipDefaults && f.isStructPtr() && nilcheck.IsNotNilValue(f.value) {
			if df, ok := f.value.Interface().(Defaulter); ok {
				df.Default()
			}
		}
	}

	return
}

// applyDefaultsToMaps applies default values to structs inside maps recursively.
func (cf *Config) applyDefaultsToMaps(value any) error {
	v := reflect.ValueOf(value)
	if v.Kind() == reflect.Pointer {
		v = v.Elem()
	}

	return cf.applyDefaultsToValue(v)
}

// applyDefaultsToValue recursively applies defaults to a reflect.Value.
// Dispatches to type-specific handlers for better code organization.
func (cf *Config) applyDefaultsToValue(v reflect.Value) error {
	if !v.IsValid() || !v.CanInterface() {
		return nil
	}

	switch v.Kind() {
	case reflect.Struct:
		return cf.applyDefaultsToStructFields(v)
	case reflect.Map:
		return cf.applyDefaultsToMap(v)
	case reflect.Slice, reflect.Array:
		return cf.applyDefaultsToSlice(v)
	case reflect.Pointer:
		return cf.applyDefaultsToPointer(v)
	default:
		return nil
	}
}

// applyDefaultsToStructFields applies defaults to all fields in a struct.
func (cf *Config) applyDefaultsToStructFields(v reflect.Value) error {
	for i := range v.NumField() {
		field := v.Field(i)
		if field.CanSet() {
			if err := cf.applyDefaultsToValue(field); err != nil {
				return err
			}
		}
	}
	return nil
}

// applyDefaultsToMap applies defaults to struct values inside a map.
func (cf *Config) applyDefaultsToMap(v reflect.Value) error {
	if v.IsNil() {
		return nil
	}

	valueType := v.Type().Elem()

	// Only process maps with struct values
	if !cf.isStructValueType(valueType) {
		return nil
	}

	// Collect all keys first (we can't modify map while iterating)
	keys := cf.collectMapKeys(v)

	// Process each map entry
	return cf.processMapEntries(v, valueType, keys)
}

// isStructValueType checks if a type is a struct or pointer to struct.
func (cf *Config) isStructValueType(valueType reflect.Type) bool {
	return valueType.Kind() == reflect.Struct ||
		(valueType.Kind() == reflect.Pointer && valueType.Elem().Kind() == reflect.Struct)
}

// collectMapKeys collects all keys from a map.
func (cf *Config) collectMapKeys(v reflect.Value) []reflect.Value {
	keys := make([]reflect.Value, 0)
	iter := v.MapRange()
	for iter.Next() {
		keys = append(keys, iter.Key())
	}
	return keys
}

// processMapEntries processes all entries in a map.
func (cf *Config) processMapEntries(v reflect.Value, valueType reflect.Type, keys []reflect.Value) error {
	for _, mapKey := range keys {
		mapValue := v.MapIndex(mapKey)

		if err := cf.processMapEntry(v, valueType, mapKey, mapValue); err != nil {
			return err
		}
	}
	return nil
}

// processMapEntry processes a single map entry.
func (cf *Config) processMapEntry(v reflect.Value, valueType reflect.Type, mapKey, mapValue reflect.Value) error {
	switch mapValue.Kind() {
	case reflect.Pointer:
		return cf.processPointerMapValue(mapValue)
	case reflect.Struct:
		return cf.processStructMapValue(v, valueType, mapKey, mapValue)
	default:
		return nil
	}
}

// processPointerMapValue processes a pointer value in a map.
func (cf *Config) processPointerMapValue(mapValue reflect.Value) error {
	if mapValue.IsNil() {
		return nil
	}

	actualValue := mapValue.Elem()
	if actualValue.Kind() == reflect.Struct {
		return cf.applyDefaultsToStruct(actualValue)
	}
	return nil
}

// processStructMapValue processes a struct value in a map.
// Creates a modifiable copy since map values are not addressable.
func (cf *Config) processStructMapValue(v reflect.Value, valueType reflect.Type, mapKey, mapValue reflect.Value) error {
	// Create a modifiable copy
	newValue := reflect.New(valueType).Elem()
	newValue.Set(mapValue)

	// Apply defaults to the copy
	if err := cf.applyDefaultsToStruct(newValue); err != nil {
		return err
	}

	// Set the modified copy back to the map
	v.SetMapIndex(mapKey, newValue)
	return nil
}

// applyDefaultsToSlice applies defaults to all elements in a slice or array.
func (cf *Config) applyDefaultsToSlice(v reflect.Value) error {
	for i := range v.Len() {
		elem := v.Index(i)
		if err := cf.applyDefaultsToValue(elem); err != nil {
			return err
		}
	}
	return nil
}

// applyDefaultsToPointer applies defaults to the value pointed to by a pointer.
func (cf *Config) applyDefaultsToPointer(v reflect.Value) error {
	if !v.IsNil() {
		return cf.applyDefaultsToValue(v.Elem())
	}
	return nil
}

// applyDefaultsToStruct applies default values to a struct's fields.
func (cf *Config) applyDefaultsToStruct(structValue reflect.Value) error {
	structType := structValue.Type()

	for i := range structType.NumField() {
		field := structType.Field(i)
		fieldValue := structValue.Field(i)

		// Skip unexported fields
		if !fieldValue.CanSet() {
			continue
		}

		// Get default tag
		defaultTag := field.Tag.Get(defaultValueTagName)
		if defaultTag == "" {
			// No default tag, but recurse into the field
			if err := cf.applyDefaultsToValue(fieldValue); err != nil {
				return err
			}
			continue
		}

		// Check if field is zero value
		if !fieldValue.IsZero() {
			// Field already has a value, skip setting default
			continue
		}

		// Apply environment variable substitution
		if cf.options.strict {
			var subErr error
			defaultTag, subErr = substituteEnvVariablesStrict(defaultTag)
			if subErr != nil {
				return subErr
			}
		} else {
			defaultTag = substituteEnvVariables(defaultTag)
		}

		// Set the default value
		// Use isDefaultValue=false to bypass conditional logic since we already checked IsZero
		if err := set(fieldValue, defaultTag, false, true, cf.options.strict); err != nil {
			return err
		}

		// Recurse into the field after setting defaults
		if err := cf.applyDefaultsToValue(fieldValue); err != nil {
			return err
		}
	}

	return nil
}
