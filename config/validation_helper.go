// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package config

import (
	"fmt"
	"reflect"

	coreerrors "github.com/altessa-s/go-atlas/core/errors"
	validation "github.com/go-ozzo/ozzo-validation/v4"
)

// ValidateStruct validates a struct and enhances ozzo-validation's internal errors
// (ErrFieldNotFound, ErrFieldPointer) with the struct type name and actionable hints.
//
// Use this instead of validation.ValidateStruct to get clearer error messages when
// a validation.Field pointer does not reference a field in the target struct.
//
// Example:
//
//	func (c *Config) Validate() error {
//	    return config.ValidateStruct(c,
//	        validation.Field(&c.Field1, validation.Required),
//	    )
//	}
func ValidateStruct(structPtr any, fields ...*validation.FieldRules) error {
	err := validation.ValidateStruct(structPtr, fields...)
	if err == nil {
		return nil
	}

	return enhanceInternalError(err, structPtr)
}

// ValidateStructIfEnabled validates a struct only if the enabled flag is true.
// This helper reduces boilerplate in configuration structs that have an "Enable" field.
//
// Example:
//
//	func (c *Config) Validate() error {
//	    return ValidateStructIfEnabled(c.Enable, c,
//	        validation.Field(&c.Field1, validation.Required),
//	    )
//	}
func ValidateStructIfEnabled(enabled bool, structPtr any, fields ...*validation.FieldRules) error {
	if !enabled {
		return nil
	}

	return ValidateStruct(structPtr, fields...)
}

// enhanceInternalError checks if the error is an ozzo-validation InternalError wrapping
// ErrFieldNotFound or ErrFieldPointer, and replaces it with a more descriptive message.
func enhanceInternalError(err error, structPtr any) error {
	ie, ok := err.(validation.InternalError) //nolint:errorlint // ozzo-validation uses type assertion
	if !ok || ie.InternalError() == nil {
		return err
	}

	inner := ie.InternalError()
	typeName := structTypeName(structPtr)

	if fieldNotFound, ok := coreerrors.AsType[validation.ErrFieldNotFound](inner); ok {
		return fmt.Errorf("validation of %s: field rule #%d: "+
			"pointer does not reference a field in the struct (use &s.FieldName, not &s)",
			typeName, int(fieldNotFound))
	}

	if fieldPointer, ok := coreerrors.AsType[validation.ErrFieldPointer](inner); ok {
		return fmt.Errorf("validation of %s: field rule #%d: "+
			"field must be specified as a pointer (use &s.FieldName)",
			typeName, int(fieldPointer))
	}

	return err
}

// structTypeName returns a human-readable type name for the struct pointer.
func structTypeName(structPtr any) string {
	t := reflect.TypeOf(structPtr)
	if t == nil {
		return "<nil>"
	}
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}

	return t.Name()
}
