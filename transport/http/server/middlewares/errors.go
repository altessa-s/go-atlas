// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package middlewares

// MiddlewareError is a structured error for HTTP middlewares that carries
// an HTTP status code, a user-facing message, and an optional machine-readable code.
// Create instances with [NewMiddlewareError] or [NewMiddlewareErrorWithDefaults].
//
// It implements the error interface plus the writer package interfaces
// ([writer.Coder], [writer.Messager], [writer.HTTPStatuser]) so that the
// response [writer.Builder] can extract structured error details automatically.
type MiddlewareError struct {
	status int
	msg    string
	code   string
}

// Error implements the error interface.
func (e *MiddlewareError) Error() string { return e.msg }

// Message returns the user-facing error message.
func (e *MiddlewareError) Message() string { return e.msg }

// Code returns the machine-readable error code.
func (e *MiddlewareError) Code() string { return e.code }

// HTTPStatus returns the HTTP status code for the error.
func (e *MiddlewareError) HTTPStatus() int { return e.status }

// NewMiddlewareError creates a new MiddlewareError with the given HTTP status and message.
func NewMiddlewareError(status int, msg string) *MiddlewareError {
	return &MiddlewareError{
		status: status,
		msg:    msg,
	}
}

// NewMiddlewareErrorWithDefaults creates a new MiddlewareError with the given HTTP status, message, and error code.
func NewMiddlewareErrorWithDefaults(status int, msg, code string) *MiddlewareError {
	return &MiddlewareError{
		status: status,
		msg:    msg,
		code:   code,
	}
}
