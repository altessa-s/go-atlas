// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package nilcheck

import "fmt"

// RequiredError indicates that a required dependency or field was nil.
// It is returned by [RequireNotNil] and can be created directly with
// [NewRequiredError]. The error message follows the format "<FieldName> is required".
type RequiredError struct {
	// FieldName is the name of the nil field or dependency that triggered the error.
	FieldName string
}

// Error implements the error interface.
func (e *RequiredError) Error() string {
	return fmt.Sprintf("%s is required", e.FieldName)
}

// NewRequiredError creates a new [RequiredError] for the given field or dependency name.
// The resulting error message will be formatted as "<fieldName> is required".
func NewRequiredError(fieldName string) *RequiredError {
	return &RequiredError{FieldName: fieldName}
}
