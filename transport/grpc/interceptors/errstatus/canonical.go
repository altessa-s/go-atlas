// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package errstatus

import (
	"github.com/altessa-s/go-aip/apierror"

	"google.golang.org/grpc/codes"

	coreerrs "github.com/altessa-s/go-atlas-core/errors"
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
			ErrorRule{
				Target: coreerrs.ErrNotFound, Code: codes.NotFound,
				Message: apierror.MessageNotFound, Reason: apierror.ReasonNotFound,
			}.converter(),
			ErrorRule{
				Target: coreerrs.ErrAlreadyExists, Code: codes.AlreadyExists,
				Message: apierror.MessageAlreadyExists, Reason: apierror.ReasonAlreadyExists,
			}.converter(),
			ErrorRule{
				Target: coreerrs.ErrConflict, Code: codes.Aborted,
				Message: apierror.MessageConflict, Reason: apierror.ReasonVersionConflict,
			}.converter(),
			fieldErrorConverter(mapField),
			ErrorRule{Target: coreerrs.ErrInvalidArgument, Code: codes.InvalidArgument, Expose: true}.converter(),
			ErrorRule{
				Target: coreerrs.ErrFailedPrecondition, Code: codes.FailedPrecondition,
				Message: apierror.MessageFailedPrecondition, Reason: apierror.ReasonFailedPrecondition,
			}.converter(),
			ErrorRule{
				Target: coreerrs.ErrUnavailable, Code: codes.Unavailable,
				Message: apierror.MessageUnavailable, Reason: apierror.ReasonUnavailable,
			}.converter(),
			ErrorRule{
				Target: coreerrs.ErrBusy, Code: codes.Aborted,
				Message: apierror.MessageBusy, Reason: apierror.ReasonBusy,
			}.converter(),
		)
	}
}

// WithCanonicalAPIRules is [WithCanonicalRules] with field paths in
// lowerCamelCase, the proto JSON names an API publishes: it also sets
// [WithFieldPathMapper] to [CamelPath], which leaves a path a handler already
// mapped (for example with [Rooted]) unchanged. A later WithFieldPathMapper
// replaces the mapper.
func WithCanonicalAPIRules() Option {
	return func(o *options) {
		WithFieldPathMapper(CamelPath)(o)
		WithCanonicalRules()(o)
	}
}
