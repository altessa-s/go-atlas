// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package interceptors

import (
	"errors"

	"github.com/altessa-s/go-atlas/core/text/strings"

	"google.golang.org/grpc/status"
)

// Error combines a gRPC status with a standard Go error.
// It implements both error and status.GRPCStatus interfaces for seamless integration.
type Error struct {
	// s holds the gRPC status information
	s *status.Status
	// err holds the underlying Go error
	err error
}

// NewError creates an Error combining a gRPC status with an underlying error.
// If err is nil, creates an error from the status message (lowercase).
//
// Example:
//
//	// Wrap existing error
//	err := interceptors.NewError(
//	    status.New(codes.InvalidArgument, "Invalid request"),
//	    validationErr,
//	)
//
// Example with nil error:
//
//	err := interceptors.NewError(
//	    status.New(codes.Unauthenticated, "Missing credentials"),
//	    nil,
//	)
func NewError(st *status.Status, err error) *Error {
	if err == nil {
		err = errors.New(strings.InternLowerString(st.Message()))
	}
	return &Error{
		s:   st,
		err: err,
	}
}

// Error returns the underlying error message.
func (e *Error) Error() string {
	return e.err.Error()
}

// Unwrap returns the underlying error for Go 1.13+ error chains.
//
// Example:
//
//	var validationErr *ValidationError
//	if errors.As(err, &validationErr) {
//	    // Handle validation error
//	}
func (e *Error) Unwrap() error {
	return e.err
}

// Is reports whether this Error matches the target error.
func (e *Error) Is(err error) bool {
	return errors.Is(e.err, err)
}

// GRPCStatus returns the gRPC status for the RPC response.
// Implements status.GRPCStatus interface.
func (e *Error) GRPCStatus() *status.Status {
	return e.s
}
