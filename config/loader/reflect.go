// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package loader

import (
	"fmt"
	"iter"
	"math"
	"reflect"
	"strconv"
	"strings"
	"time"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
)

const (
	// Tag parsing constants
	skipZeroTag = "skip_zero"

	// Map/slice parsing constants
	mapSeparator    = ":"
	commaSeparator  = ","
	mapKeyValuePair = 2

	// Numeric parsing constants
	decimalBase = 10
)

// Predefined errors for better error handling and type checking
var (
	ErrBoolParseFailure = fmt.Errorf("boolean parse failure")
	ErrSliceItemInvalid = fmt.Errorf("slice item invalid")
	ErrMapItemInvalid   = fmt.Errorf("map item invalid")
	ErrMapKeyInvalid    = fmt.Errorf("map key invalid")
	ErrMapValueInvalid  = fmt.Errorf("map value invalid")
	ErrNumericParse     = fmt.Errorf("numeric parse failure")
	ErrFloatParse       = fmt.Errorf("float parse failure")
	ErrUintParse        = fmt.Errorf("uint parse failure")
)

// wrapError creates a wrapped error with a base error, message template, and optional cause.
// This generic function replaces multiple specialized error wrappers to reduce code duplication.
func wrapError(baseErr error, msgTemplate string, value string, cause error) error {
	if cause != nil {
		return coreerrs.Wrapf(baseErr, msgTemplate+": %v", value, cause)
	}
	return coreerrs.Wrapf(baseErr, msgTemplate, value)
}

// field represents a struct field with its metadata, tags, and reflection information.
// It maintains parent-child relationships for nested structs.
type field struct {
	tags        map[string]string
	parent      *field
	child       []*field
	fullName    string
	name        string
	value       reflect.Value
	field       reflect.StructField
	parentMap   *reflect.Value // Parent map if this field is from a map
	mapKey      *reflect.Value // Key in parent map if this field is from a map
	parentValue *reflect.Value // Parent struct value (for updating map entries)
}

// isStructPtr checks if the field is a pointer to a struct.
func (f *field) isStructPtr() bool {
	if !f.value.IsValid() {
		return false
	}

	if f.value.Kind() != reflect.Pointer {
		return false
	}

	ft := indirectType(f.value.Type())

	return ft.Kind() == reflect.Struct
}

// initializeStruct initializes the parent struct if it's nil and creates child field references.
// It processes anonymous fields and updates child field values accordingly.
// When strict is true, returns an error if anonymous field assignment fails.
func (f *field) initializeStruct(tag string, strict bool) error {
	if f.parent == nil || !f.parent.isStructPtr() {
		return nil
	}

	v := ""
	if vs := strings.Split(f.parent.tags[tag], ","); len(vs) > 0 {
		v = vs[0]
	}

	if f.parent.value.IsNil() && v != "-" {
		vl := reflect.New(indirect(f.parent.value.Type()))
		f.parent.value.Set(vl)
		for indx := range indirectValue(vl).NumField() {
			fl := indirectType(vl.Type()).Field(indx)

			if fl.Anonymous {
				// Safe type initialization for inline fields
				fieldVal := indirectValue(vl).Field(indx)
				newVal := reflect.New(indirect(fl.Type))

				// Check type compatibility before setting
				if safeSetValue(fieldVal, newVal) != nil {
					// If direct assignment fails, try to handle common type mismatches
					switch {
					case fieldVal.Kind() == reflect.Interface && newVal.Type().Implements(fieldVal.Type()):
						fieldVal.Set(newVal.Elem())
					case strict:
						return fmt.Errorf("%w: cannot assign %s to %s", ErrFieldAssignment, newVal.Type(), fieldVal.Type())
					default:
						continue
					}
				}
			}
		}

		for _, cf := range f.parent.child {
			if !cf.value.IsValid() {
				// For inline fields, we need to navigate through the structure
				// to find the actual field, not just use FieldByName
				parentValue := indirectValue(vl)
				if fieldValue := findFieldInValue(parentValue, cf.name); fieldValue.IsValid() {
					cf.value = fieldValue
				}
			}
		}
	}

	return nil
}

// setDefaultValue sets the default value for the field from struct tags.
// It supports the skip_zero option to control when zero values should be replaced.
// When strict is true, returns an error if environment variable substitution references undefined variables.
func (f *field) setDefaultValue(tag string, strict bool) (err error) {
	if err = f.initializeStruct(tag, strict); err != nil {
		return err
	}

	replaceZeroValue := true

	if f.value.IsValid() {
		value := f.tags[tag]
		vs := strings.Split(value, ",")

		if len(vs) > 1 && vs[len(vs)-1] == skipZeroTag {
			replaceZeroValue = true
			value = strings.Join(vs[:len(vs)-1], "")
		}

		// Apply environment variable substitution to default values
		if strict {
			value, err = substituteEnvVariablesStrict(value)
			if err != nil {
				return err
			}
		} else {
			value = substituteEnvVariables(value)
		}

		err = set(f.value, value, true, replaceZeroValue, strict)
	}

	return
}

// setValue sets the field value from a string, typically from environment variables.
// When strict is true, returns an error for unsupported types or assignment failures.
func (f *field) setValue(v, tag string, strict bool) (err error) {
	if err = f.initializeStruct(tag, strict); err != nil {
		return err
	}

	if f.value.IsValid() {
		err = set(f.value, v, false, true, strict)
	}

	return
}

// preparePointerField prepares a pointer field for value assignment.
// It dereferences nested pointers and initializes nil pointers if needed.
func preparePointerField(fieldValue reflect.Value, isDefaultValue bool) reflect.Value {
	for fieldValue.Type().Kind() == reflect.Pointer {
		if fieldValue.IsNil() {
			if isDefaultValue {
				return reflect.Value{} // Return invalid value to signal skip
			}
			fieldValue.Set(reflect.New(fieldValue.Type().Elem()))
		}
		fieldValue = fieldValue.Elem()
	}
	return fieldValue
}

// set assigns a string value to a reflect.Value based on the value's type.
// It handles all supported types including numbers, booleans, strings, slices, and maps.
// Uses a single switch instead of nested switches for better performance and maintainability.
// When strict is true, returns an error for unsupported field types instead of silently skipping.
func set(fieldValue reflect.Value, defaultValue string, isDefaultValue, replaceDefaultValue, strict bool) error {
	// Prepare pointer fields
	preparedValue := preparePointerField(fieldValue, isDefaultValue)
	if !preparedValue.IsValid() {
		return nil // Skip invalid values (happens for nil pointers with default values)
	}

	fieldValue = preparedValue

	// Direct dispatch to type-specific handlers without nested switches
	switch fieldValue.Kind() {
	// Integer types - all handled by setIntValue
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return setIntValue(fieldValue, defaultValue, isDefaultValue, replaceDefaultValue)
	// Unsigned integer types - all handled by setUintValue
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return setUintValue(fieldValue, defaultValue, isDefaultValue, replaceDefaultValue)
	// Floating point types - all handled by setFloatValue
	case reflect.Float32, reflect.Float64:
		return setFloatValue(fieldValue, defaultValue, isDefaultValue, replaceDefaultValue)
	// Other types
	case reflect.Bool:
		return setBool(fieldValue, defaultValue, isDefaultValue, replaceDefaultValue)
	case reflect.String:
		return setString(fieldValue, defaultValue, isDefaultValue, replaceDefaultValue)
	case reflect.Interface:
		return setInterface(fieldValue, defaultValue)
	case reflect.Slice:
		return setSlice(fieldValue, defaultValue, isDefaultValue, replaceDefaultValue, strict)
	case reflect.Map:
		return setMap(fieldValue, defaultValue, isDefaultValue, replaceDefaultValue, strict)
	case reflect.Struct:
		// Structs are handled by recursive field processing, not by set()
		return nil
	default:
		if strict {
			return fmt.Errorf("%w: kind %s", ErrUnsupportedFieldType, fieldValue.Kind())
		}
		return nil
	}
}

// setInterface sets an any value to a reflect.Value.
// It safely handles different interface types and only sets values for empty interfaces.
func setInterface(fieldValue reflect.Value, defaultValue string) error {
	if defaultValue == "" {
		return nil // Skip empty values for interfaces
	}

	// Only handle any (empty interface) types
	interfaceType := fieldValue.Type()
	if interfaceType.NumMethod() == 0 {
		// This is any - safe to assign string or []byte
		if defaultValue != "" {
			fieldValue.Set(reflect.ValueOf([]byte(defaultValue)))
		}
		return nil
	}

	// For other interfaces, we can't safely assign arbitrary values
	// Skip setting values for non-empty interfaces
	return nil
}

// setString sets a string value to a reflect.Value.
// It respects the replaceDefaultValue flag when dealing with default values.
func setString(fieldValue reflect.Value, defaultValue string, isDefaultValue, replaceDefaultValue bool) error {
	if !isDefaultValue {
		fieldValue.SetString(defaultValue)
		return nil
	}

	if fieldValue.Len() == 0 && replaceDefaultValue {
		fieldValue.SetString(defaultValue)
	}
	return nil
}

// setBool parses and sets a boolean value from a string.
// It returns an error if the string cannot be parsed as a boolean.
func setBool(fieldValue reflect.Value, defaultValue string, isDefaultValue, replaceDefaultValue bool) error {
	if defaultValue == "" {
		return nil
	}

	boolValue, err := strconv.ParseBool(defaultValue)
	if err != nil {
		return wrapError(ErrBoolParseFailure, "a boolean value expected, got %q", defaultValue, err)
	}

	if !isDefaultValue {
		fieldValue.SetBool(boolValue)
		return nil
	}

	if !fieldValue.Bool() && replaceDefaultValue {
		fieldValue.SetBool(boolValue)
	}

	return nil
}

// setSlice parses and sets a slice value from a comma-separated string.
// It handles special cases like byte slices and nested struct defaults.
func setSlice(fieldValue reflect.Value, defaultValue string, isDefaultValue, replaceDefaultValue, strict bool) error {
	if defaultValue == "" {
		if indirectType(fieldValue.Type().Elem()).Kind() == reflect.Struct {
			for i := range fieldValue.Len() {
				e := fieldValue.Index(i)
				eInterface := e.Interface()

				structFields(eInterface).each(func(f *field) bool {
					if !f.isStructPtr() {
						if err := f.setDefaultValue(defaultValueTagName, strict); err != nil {
							return false
						}
					}
					return true
				})

				if df, ok := eInterface.(Defaulter); ok {
					df.Default()
				}
			}
		}
		return nil
	}

	if isDefaultValue && !fieldValue.IsNil() {
		return nil
	}

	if fieldValue.Type().Elem().Kind() == reflect.Uint8 {
		value := reflect.ValueOf([]byte(defaultValue))

		fieldValue.Set(value)

		return nil
	}

	// Remove surrounding brackets if present: [value1,value2] -> value1,value2
	trimmedValue := strings.TrimSpace(defaultValue)
	if strings.HasPrefix(trimmedValue, "[") && strings.HasSuffix(trimmedValue, "]") {
		trimmedValue = trimmedValue[1 : len(trimmedValue)-1]
	}

	vals := strings.Split(trimmedValue, commaSeparator)

	slice := reflect.MakeSlice(fieldValue.Type(), len(vals), len(vals))

	for i, val := range vals {
		trimmedVal := strings.TrimSpace(val)
		// Strip surrounding quotes from JSON-style string values
		if len(trimmedVal) >= 2 && trimmedVal[0] == '"' && trimmedVal[len(trimmedVal)-1] == '"' {
			trimmedVal = trimmedVal[1 : len(trimmedVal)-1]
		}
		if err := set(slice.Index(i), trimmedVal, isDefaultValue, replaceDefaultValue, strict); err != nil {
			return wrapError(ErrSliceItemInvalid, "incorrect slice item %q", val, err)
		}
	}

	fieldValue.Set(slice)

	return nil
}

// setMap parses and sets a map value from a comma-separated string of key:value pairs.
// Each pair is separated by commas and keys/values are separated by colons.
func setMap(fieldValue reflect.Value, defaultValue string, isDefaultValue, replaceDefaultValue, strict bool) error {
	if defaultValue == "" {
		return nil
	}

	if isDefaultValue && !fieldValue.IsNil() {
		return nil
	}

	vals := strings.Split(defaultValue, commaSeparator)
	reflectMap := reflect.MakeMapWithSize(fieldValue.Type(), len(vals))

	for _, val := range vals {
		entry := strings.SplitN(val, mapSeparator, mapKeyValuePair)
		if len(entry) != mapKeyValuePair {
			return wrapError(ErrMapItemInvalid, "incorrect map item: %s", val, nil)
		}

		mKey := strings.TrimSpace(entry[0])
		mKeyValue := reflect.New(fieldValue.Type().Key()).Elem()

		if err := set(mKeyValue, mKey, isDefaultValue, replaceDefaultValue, strict); err != nil {
			return wrapError(ErrMapKeyInvalid, "incorrect map key %q", mKey, err)
		}

		mVal := strings.TrimSpace(entry[1])
		mValValue := reflect.New(fieldValue.Type().Elem()).Elem()

		if err := set(mValValue, mVal, isDefaultValue, replaceDefaultValue, strict); err != nil {
			return wrapError(ErrMapValueInvalid, "incorrect map value %q", mVal, err)
		}

		reflectMap.SetMapIndex(mKeyValue, mValValue)
	}

	fieldValue.Set(reflectMap)

	return nil
}

// parseDuration parses a string as either a time.Duration or a plain integer.
// It handles time.Duration types specially by parsing duration strings.
// Bare integers without a unit suffix (e.g. "-1", "0") are treated as nanoseconds.
func parseDuration(rvalue reflect.Value, value string) (val int64, err error) {
	switch rvalue.Interface().(type) {
	case time.Duration:
		var dur time.Duration

		if dur, err = time.ParseDuration(value); err != nil {
			// Fallback: parse as raw integer (nanoseconds), consistent with how
			// time.Duration is defined (type Duration int64).
			if val, err = strconv.ParseInt(value, decimalBase, 64); err != nil {
				return 0, fmt.Errorf("failed to parse duration %q: not a valid duration string or integer", value)
			}

			return val, nil
		}

		val = int64(dur)
	default:
		if val, err = strconv.ParseInt(value, decimalBase, rvalue.Type().Bits()); err != nil {
			return 0, err
		}
	}

	return
}

// parseStringInt extracts an int64 value from a reflect.Value.
// It handles both integer and string types (for duration parsing).
func parseStringInt(fv reflect.Value) (val int64) {
	val = fv.Int()
	if vt, ok := fv.Interface().(string); ok {
		val, _ = parseDuration(reflect.Indirect(fv), vt) //nolint:errcheck
	}

	return
}

// setIntValue parses and sets signed integer values, including time.Duration support.
func setIntValue(fieldValue reflect.Value, defaultValue string, isDefaultValue, replaceDefaultValue bool) error {
	if defaultValue == "" {
		return nil
	}

	defaultIntValue, err := parseDuration(reflect.Indirect(fieldValue), defaultValue)
	if err != nil {
		return wrapError(ErrNumericParse, "failed to parse %q", defaultValue, err)
	}

	if !isDefaultValue || (parseStringInt(reflect.Indirect(fieldValue)) == 0 && replaceDefaultValue) {
		fieldValue.SetInt(defaultIntValue)
	}
	return nil
}

// setUintValue parses and sets unsigned integer values.
func setUintValue(fieldValue reflect.Value, defaultValue string, isDefaultValue, replaceDefaultValue bool) error {
	if defaultValue == "" {
		return nil
	}

	uintNumber, err := strconv.ParseUint(defaultValue, decimalBase, fieldValue.Type().Bits())
	if err != nil {
		return wrapError(ErrUintParse, "failed to parse %q", defaultValue, err)
	}

	if !isDefaultValue || (fieldValue.Uint() == 0 && replaceDefaultValue) {
		fieldValue.SetUint(uintNumber)
	}
	return nil
}

// setFloatValue parses and sets floating-point values.
func setFloatValue(fieldValue reflect.Value, defaultValue string, isDefaultValue, replaceDefaultValue bool) error {
	if defaultValue == "" {
		return nil
	}

	floatNumber, err := strconv.ParseFloat(defaultValue, fieldValue.Type().Bits())
	if err != nil {
		return wrapError(ErrFloatParse, "failed to parse %q", defaultValue, err)
	}

	if !isDefaultValue || (math.Float64bits(fieldValue.Float()) == 0 && replaceDefaultValue) {
		fieldValue.SetFloat(floatNumber)
	}
	return nil
}

// fields represents a collection of struct fields.
type fields []*field

// each iterates through the fields and calls the callback function for each field.
// The iteration stops if the callback returns false.
func (fs fields) each(cb func(f *field) bool) {
	for f := range fs.All() {
		if !cb(f) {
			break
		}
	}
}

// All returns an iterator for the fields collection.
func (fs fields) All() iter.Seq[*field] {
	return func(yield func(*field) bool) {
		for _, f := range fs {
			if !yield(f) {
				return
			}
		}
	}
}

// structFields extracts all fields from a struct, including nested and anonymous fields.
// It returns a flattened list of all accessible fields.
func structFields(s any) fields {
	structValue := reflect.ValueOf(s)
	structType := reflect.TypeOf(s)
	return fieldsList(structValue, structType, nil)
}

// fieldsList recursively builds a list of fields from a struct type.
// It handles nested structs and anonymous fields, maintaining parent-child relationships.
func fieldsList(structValue reflect.Value, structType reflect.Type, parent *field) fields {
	structValue = indirectValue(structValue)
	structType = indirectType(structType)

	list := make(fields, 0, structType.NumField())

	for indx := range structType.NumField() {
		structField := structType.Field(indx)
		if structField.PkgPath != "" {
			continue
		}

		var structFieldValue reflect.Value
		if structValue.IsValid() {
			structFieldValue = structValue.Field(indx)
			if !structFieldValue.CanSet() {
				continue
			}
		}

		fld := &field{
			fullName: makeFieldName(structField.Name, parent),
			name:     structField.Name,
			field:    structField,
			value:    structFieldValue,
			parent:   parent,
			tags: map[string]string{
				defaultValueTagName:  structField.Tag.Get(defaultValueTagName),
				envTagName:           structField.Tag.Get(envTagName),
				DefaultStructTagName: structField.Tag.Get(DefaultStructTagName),
			},
		}

		// For inline fields, add them to parent's children for proper initialization
		if structField.Anonymous && parent != nil {
			parent.child = append(parent.child, fld)
		}

		// For non-inline fields, add to list
		if !structField.Anonymous {
			list = append(list, fld)
		}

		if indirectType(structField.Type).Kind() == reflect.Struct {
			pf := fld
			if structField.Anonymous {
				pf = parent
				// Initialize inline pointer struct if needed
				if structFieldValue.Kind() == reflect.Pointer && structFieldValue.IsNil() {
					newVal := reflect.New(structFieldValue.Type().Elem())
					structFieldValue.Set(newVal)
				}
				// Update structFieldValue after initialization
				if structFieldValue.Kind() == reflect.Pointer {
					structFieldValue = structFieldValue.Elem()
				}
			}

			fld.child = fieldsList(structFieldValue, structField.Type, pf)
			list = append(list, fld.child...)

			continue
		}
	}

	return list
}

// indirect returns the underlying type, dereferencing pointers.
// It accepts both reflect.Type and any other type.
func indirect(s any) reflect.Type {
	if t, ok := s.(reflect.Type); ok {
		return indirectType(t)
	}
	v := reflect.TypeOf(s)
	return indirectType(v)
}

// indirectType dereferences a reflect.Type if it's a pointer.
func indirectType(t reflect.Type) reflect.Type {
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}

	return t
}

// indirectValue dereferences a reflect.Value through all pointer levels.
func indirectValue(value reflect.Value) reflect.Value {
	for value.Kind() == reflect.Pointer {
		value = value.Elem()
	}

	return value
}

// makeFieldName constructs a full field name including parent hierarchy.
// It creates dot-separated field names for nested structs.
func makeFieldName(name string, parent *field) string {
	if parent == nil {
		return name
	}

	return parent.fullName + "." + name
}

// findFieldInValue recursively searches for a field by name in a struct value,
// including fields in inline/anonymous structs.
func findFieldInValue(structValue reflect.Value, fieldName string) reflect.Value {
	structType := structValue.Type()

	// First, try direct field lookup
	for i := range structType.NumField() {
		field := structType.Field(i)

		if field.Anonymous {
			// Skip anonymous fields in first pass
			continue
		}

		if field.Name == fieldName {
			return structValue.Field(i)
		}
	}

	// Second pass: search in anonymous/inline fields
	for i := range structType.NumField() {
		field := structType.Field(i)

		if !field.Anonymous {
			continue
		}

		fieldValue := structValue.Field(i)
		if fieldValue.Kind() == reflect.Pointer {
			if fieldValue.IsNil() {
				// Initialize the pointer if nil
				fieldValue.Set(reflect.New(fieldValue.Type().Elem()))
			}
			fieldValue = fieldValue.Elem()
		}

		if fieldValue.Kind() == reflect.Struct {
			// Recursively search in the inline struct
			if found := findFieldInValue(fieldValue, fieldName); found.IsValid() {
				return found
			}
		}
	}

	return reflect.Value{}
}

// safeSetValue safely sets a value to a field, checking type compatibility first.
// It handles common type mismatches and returns an error if assignment is not possible.
func safeSetValue(fieldValue, newValue reflect.Value) error {
	if !fieldValue.CanSet() {
		return fmt.Errorf("field cannot be set")
	}

	fieldType := fieldValue.Type()
	newType := newValue.Type()

	// Direct type match - safe to assign
	if fieldType == newType {
		fieldValue.Set(newValue)
		return nil
	}

	// Handle pointer to value assignment
	if fieldType == newType.Elem() && newValue.Kind() == reflect.Pointer {
		if newValue.IsNil() {
			return fmt.Errorf("cannot assign nil pointer")
		}
		fieldValue.Set(newValue.Elem())
		return nil
	}

	// Handle value to pointer assignment
	if newType == fieldType.Elem() && fieldValue.Kind() == reflect.Pointer {
		if fieldValue.IsNil() {
			fieldValue.Set(reflect.New(fieldType.Elem()))
		}
		fieldValue.Elem().Set(newValue)
		return nil
	}

	// Handle interface assignments
	if fieldValue.Kind() == reflect.Interface {
		if newType.Implements(fieldType) {
			fieldValue.Set(newValue)
			return nil
		}
		// Try with pointer receiver
		if reflect.PointerTo(newType).Implements(fieldType) {
			fieldValue.Set(newValue.Addr())
			return nil
		}
	}

	// Handle assignability and convertibility
	if newType.AssignableTo(fieldType) {
		fieldValue.Set(newValue)
		return nil
	}

	if newType.ConvertibleTo(fieldType) {
		fieldValue.Set(newValue.Convert(fieldType))
		return nil
	}

	return fmt.Errorf("cannot assign %s to %s", newType, fieldType)
}
