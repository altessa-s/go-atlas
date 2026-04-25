// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package writer

// Coder is an interface for errors that provide a machine-readable error code.
// If an error implements this interface, the code is automatically extracted
// during error response building.
//
// Example:
//
//	type ValidationError struct {
//	    Field string
//	}
//
//	func (e *ValidationError) Error() string { return "validation failed" }
//	func (e *ValidationError) Code() string  { return "VALIDATION_ERROR" }
type Coder interface {
	// Code returns the machine-readable error code (e.g., "VALIDATION_ERROR", "NOT_FOUND").
	Code() string
}

// Messager is an interface for errors that provide a custom user-facing message.
// If an error implements this interface, the message is used instead of Error().
// This allows separating internal error details from user-facing messages.
//
// Example:
//
//	type NotFoundError struct {
//	    Resource string
//	}
//
//	func (e *NotFoundError) Error() string   { return fmt.Sprintf("resource %s not found in database", e.Resource) }
//	func (e *NotFoundError) Message() string { return "The requested resource was not found" }
type Messager interface {
	// Message returns the user-facing error message.
	Message() string
}

// HTTPStatuser is an interface for errors that specify their HTTP status code.
// If an error implements this interface, the status code is automatically extracted.
//
// Example:
//
//	type NotFoundError struct{}
//
//	func (e *NotFoundError) Error() string      { return "not found" }
//	func (e *NotFoundError) HTTPStatus() int    { return http.StatusNotFound }
type HTTPStatuser interface {
	// HTTPStatus returns the appropriate HTTP status code for this error.
	HTTPStatus() int
}
