// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package client

import (
	"fmt"
	"slices"
	"strings"

	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	coreslices "github.com/altessa-s/go-atlas/core/collections/slices"
	coreerrs "github.com/altessa-s/go-atlas/core/errors"
)

// FieldError represents a validation error for a specific field.
type FieldError struct {
	// Field is the field path (e.g., "user.email").
	Field string

	// Message is the validation error message.
	Message string

	// Code is the validation error code.
	Code string
}

// Error returns the string representation of the field error.
func (e *FieldError) Error() string {
	return fmt.Sprintf("[%s]: %s", e.Field, e.Message)
}

// Error is a structured representation of a gRPC error, enriched with request ID,
// error reason, metadata, and per-field validation violations extracted from
// standard google.rpc error details (RequestInfo, ErrorInfo, BadRequest).
//
// Use [ParseError] or [ParseStatusError] to create an Error from a gRPC response,
// and the Is* helpers (e.g. [Error.IsNotFound], [Error.IsValidationError]) to
// inspect the error category without matching on raw status codes.
type Error struct {
	grpcCode codes.Code

	// RequestID is the request ID from the error details (if available).
	RequestID string

	// Fields contains field-level validation errors.
	Fields []FieldError

	// Message is the error message.
	Message string

	// Reason is the error reason from ErrorInfo details.
	Reason string

	// ReasonData contains additional metadata from ErrorInfo details.
	ReasonData map[string]string
}

// Code returns the gRPC status code.
func (e *Error) Code() codes.Code {
	return e.grpcCode
}

// IsNotFound returns true if the error is a NotFound gRPC error.
func (e *Error) IsNotFound() bool {
	return e.grpcCode == codes.NotFound
}

// IsAlreadyExists returns true if the error is an AlreadyExists gRPC error.
func (e *Error) IsAlreadyExists() bool {
	return e.grpcCode == codes.AlreadyExists
}

// IsValidationError returns true if the error contains field validation errors.
func (e *Error) IsValidationError() bool {
	return e.grpcCode == codes.InvalidArgument && len(e.Fields) > 0
}

// IsPermissionDenied returns true if the error is a PermissionDenied gRPC error.
func (e *Error) IsPermissionDenied() bool {
	return e.grpcCode == codes.PermissionDenied
}

// IsUnauthenticated returns true if the error is an Unauthenticated gRPC error.
func (e *Error) IsUnauthenticated() bool {
	return e.grpcCode == codes.Unauthenticated
}

// IsInternal returns true if the error is an Internal gRPC error.
func (e *Error) IsInternal() bool {
	return e.grpcCode == codes.Internal
}

// IsUnavailable returns true if the error is an Unavailable gRPC error.
func (e *Error) IsUnavailable() bool {
	return e.grpcCode == codes.Unavailable
}

// IsFailedPrecondition returns true if the error is a FailedPrecondition gRPC error.
func (e *Error) IsFailedPrecondition() bool {
	return e.grpcCode == codes.FailedPrecondition
}

// IsDeadlineExceeded returns true if the error is a DeadlineExceeded gRPC error.
func (e *Error) IsDeadlineExceeded() bool {
	return e.grpcCode == codes.DeadlineExceeded
}

// IsCanceled returns true if the error is a Canceled gRPC error.
func (e *Error) IsCanceled() bool {
	return e.grpcCode == codes.Canceled
}

// IsResourceExhausted returns true if the error is a ResourceExhausted gRPC error.
func (e *Error) IsResourceExhausted() bool {
	return e.grpcCode == codes.ResourceExhausted
}

// IsInvalidArgument returns true if the error is an InvalidArgument gRPC error.
func (e *Error) IsInvalidArgument() bool {
	return e.grpcCode == codes.InvalidArgument
}

// IsAborted returns true if the error is an Aborted gRPC error.
func (e *Error) IsAborted() bool {
	return e.grpcCode == codes.Aborted
}

// IsOutOfRange returns true if the error is an OutOfRange gRPC error.
func (e *Error) IsOutOfRange() bool {
	return e.grpcCode == codes.OutOfRange
}

// IsUnimplemented returns true if the error is an Unimplemented gRPC error.
func (e *Error) IsUnimplemented() bool {
	return e.grpcCode == codes.Unimplemented
}

// IsDataLoss returns true if the error is a DataLoss gRPC error.
func (e *Error) IsDataLoss() bool {
	return e.grpcCode == codes.DataLoss
}

// ReasonIs returns true if the error reason matches the given reason.
func (e *Error) ReasonIs(reason string) bool {
	return e.Reason == reason
}

// ReasonIsOneof returns true if the error reason matches any of the given reasons.
func (e *Error) ReasonIsOneof(reasons ...string) bool {
	return slices.Contains(reasons, e.Reason)
}

// HasFields returns true if the error contains field validation errors.
func (e *Error) HasFields() bool {
	return len(e.Fields) > 0
}

// GetField returns the field error for the given field path, or nil if not found.
func (e *Error) GetField(field string) *FieldError {
	for i := range e.Fields {
		if e.Fields[i].Field == field {
			return &e.Fields[i]
		}
	}
	return nil
}

// Error returns the string representation of the client error.
func (e *Error) Error() string {
	if e.grpcCode == codes.InvalidArgument && len(e.Fields) > 0 {
		parts := slices.Collect(coreslices.Map(e.Fields, func(field FieldError) string {
			return field.Error()
		}))
		return "validation error: " + strings.Join(parts, "; ")
	}

	msg := e.grpcCode.String()
	if e.Message != "" {
		msg += ": " + e.Message
	}

	return msg
}

// GRPCStatus returns the gRPC status representation of the error,
// implementing the grpc-go status.FromError interface so that Error
// values round-trip correctly through the gRPC error chain.
func (e *Error) GRPCStatus() *status.Status {
	return status.New(e.grpcCode, e.Message)
}

// DetailConverter is a hook for extracting [FieldError] values from custom
// protobuf error detail types. Returning nil signals that the detail was
// not recognized, letting the default BadRequest/ErrorInfo/RequestInfo
// processing take over.
type DetailConverter func(detail any) []FieldError

// ParseError converts a gRPC error to Error with additional context.
// Returns nil if the error is not a gRPC error.
// Optional converters can be provided to handle custom detail types.
func ParseError(err error, converters ...DetailConverter) *Error {
	if err == nil {
		return nil
	}

	st, ok := status.FromError(err)
	if !ok {
		return nil
	}
	return ParseStatusError(st, converters...)
}

// ParseStatusError converts a gRPC [status.Status] to an [Error], extracting
// standard google.rpc detail messages:
//
//   - errdetails.RequestInfo  --> [Error.RequestID]
//   - errdetails.ErrorInfo    --> [Error.Reason], [Error.ReasonData]
//   - errdetails.BadRequest   --> [Error.Fields]
//
// Custom detail types can be handled by passing one or more [DetailConverter]
// functions. Converters are tried in order for each detail; the first non-nil
// result wins and the default processing is skipped for that detail.
func ParseStatusError(st *status.Status, converters ...DetailConverter) *Error {
	ce := &Error{
		grpcCode: st.Code(),
		Message:  st.Message(),
		Fields:   []FieldError{},
	}

	for _, detail := range st.Details() {
		// Try custom converters first
		if fields := tryConverters(detail, converters); fields != nil {
			ce.Fields = fields
			continue
		}

		// Default processing
		switch d := detail.(type) {
		case *errdetails.RequestInfo:
			ce.RequestID = d.RequestId
		case *errdetails.ErrorInfo:
			ce.Reason = d.Reason
			ce.ReasonData = d.Metadata
		case *errdetails.BadRequest:
			fields := d.GetFieldViolations()
			ce.Fields = make([]FieldError, 0, len(fields))
			for _, field := range fields {
				ce.Fields = append(ce.Fields, FieldError{
					Field:   field.GetField(),
					Message: field.GetDescription(),
				})
			}
		}
	}

	return ce
}

// tryConverters tries each converter in order and returns the first non-nil result.
func tryConverters(detail any, converters []DetailConverter) []FieldError {
	for _, conv := range converters {
		if fields := conv(detail); fields != nil {
			return fields
		}
	}
	return nil
}

// IsClientError checks if the given error is a ClientError.
func IsClientError(err error) bool {
	_, ok := coreerrs.AsType[*Error](err) //nolint:errcheck // only checking ok
	return ok
}

// AsClientError attempts to convert the given error to a ClientError.
// Returns nil if the error is not a ClientError.
func AsClientError(err error) *Error {
	target, _ := coreerrs.AsType[*Error](err)
	return target
}
