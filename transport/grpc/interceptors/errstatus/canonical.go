// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package errstatus

import (
	"google.golang.org/grpc/codes"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
)

// Messages and ErrorInfo reasons of the canonical rules ([WithCanonicalRules]).
// They are part of the API contract: clients may match the reasons.
const (
	MessageNotFound           = "not found"
	MessageAlreadyExists      = "already exists"
	MessageConflict           = "changed concurrently, re-read and retry"
	MessageFailedPrecondition = "failed precondition"
	MessageUnavailable        = "temporarily unavailable, retry later"
	MessageBusy               = "the same work is in progress, retry later"

	ReasonNotFound           = "NOT_FOUND"
	ReasonAlreadyExists      = "ALREADY_EXISTS"
	ReasonVersionConflict    = "VERSION_CONFLICT"
	ReasonFailedPrecondition = "FAILED_PRECONDITION"
	ReasonUnavailable        = "TEMPORARY_UNAVAILABLE"
	ReasonBusy               = "BUSY"
)

// WithCanonicalRules adds converters for the canonical domain sentinels of
// core/errors, evaluated in this order after any converters registered before
// them (the first match wins):
//
//	ErrNotFound            NotFound            "not found"                                NOT_FOUND
//	ErrAlreadyExists       AlreadyExists       "already exists"                           ALREADY_EXISTS
//	ErrConflict            Aborted             "changed concurrently, re-read and retry"  VERSION_CONFLICT
//	*FieldError            InvalidArgument     "<path>: <reason>" + BadRequest violation  INVALID_ARGUMENT
//	ErrInvalidArgument     InvalidArgument     the error's text                           INVALID_ARGUMENT
//	ErrFailedPrecondition  FailedPrecondition  "failed precondition"                      FAILED_PRECONDITION
//	ErrUnavailable         Unavailable         "temporarily unavailable, retry later"     TEMPORARY_UNAVAILABLE
//	ErrBusy                Aborted             "the same work is in progress, retry later" BUSY
//
// The messages are fixed and never carry the wrapped text, which may name
// stored documents, with one exception: an ErrInvalidArgument without a
// FieldError publishes the error's text, which describes the client's own
// input (wrap ErrInvalidArgument only with client-safe context). A
// FieldError becomes the [WithFieldErrorType] status with the field mapped
// by [WithFieldPathMapper] (as written when unset); the default finalizer
// derives the INVALID_ARGUMENT reason of both InvalidArgument rows. The
// other reasons are attached by the rules and the finalizer fills in the
// domain.
//
// A sentinel registered before the field-error row wins over it for an
// error that wraps both (a FieldError joined with ErrNotFound is NotFound).
// Service-specific rules go after WithCanonicalRules; rules registered before
// it take precedence over the canonical ones.
func WithCanonicalRules() Option {
	return func(o *options) {
		mapField := func(fe *coreerrs.FieldError) (string, string) {
			if o.fieldPathMapper == nil {
				return fe.Field, fe.Reason
			}
			return o.fieldPathMapper(fe.Field), fe.Reason
		}
		o.errorConverters = append(o.errorConverters,
			ErrorRule{Target: coreerrs.ErrNotFound, Code: codes.NotFound, Message: MessageNotFound, Reason: ReasonNotFound}.converter(),
			ErrorRule{
				Target: coreerrs.ErrAlreadyExists, Code: codes.AlreadyExists, Message: MessageAlreadyExists, Reason: ReasonAlreadyExists,
			}.converter(),
			ErrorRule{Target: coreerrs.ErrConflict, Code: codes.Aborted, Message: MessageConflict, Reason: ReasonVersionConflict}.converter(),
			fieldErrorConverter(mapField),
			ErrorRule{Target: coreerrs.ErrInvalidArgument, Code: codes.InvalidArgument, Expose: true}.converter(),
			ErrorRule{
				Target: coreerrs.ErrFailedPrecondition, Code: codes.FailedPrecondition, Message: MessageFailedPrecondition,
				Reason: ReasonFailedPrecondition,
			}.converter(),
			ErrorRule{Target: coreerrs.ErrUnavailable, Code: codes.Unavailable, Message: MessageUnavailable, Reason: ReasonUnavailable}.converter(),
			ErrorRule{Target: coreerrs.ErrBusy, Code: codes.Aborted, Message: MessageBusy, Reason: ReasonBusy}.converter(),
		)
	}
}
