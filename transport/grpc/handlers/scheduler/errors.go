// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package scheduler

import (
	"errors"

	"github.com/altessa-s/go-aip/filter"
	"github.com/altessa-s/go-horai/query"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	horai "github.com/altessa-s/go-horai"
)

// mapError translates scheduler domain errors into gRPC status errors:
//
//   - ErrTaskNotFound, ErrTaskNotRegistered       --> codes.NotFound
//   - ErrTaskUnmanaged, ErrTaskNotPaused, etc.    --> codes.FailedPrecondition
//   - ErrScheduleConflict                         --> codes.InvalidArgument
//   - client-caused filter errors (see below)     --> codes.InvalidArgument
//   - everything else                             --> codes.Internal
func mapError(err error) error {
	switch {
	case errors.Is(err, horai.ErrTaskNotFound),
		errors.Is(err, horai.ErrTaskNotRegistered):
		return status.Error(codes.NotFound, err.Error())
	case errors.Is(err, horai.ErrTaskUnmanaged),
		errors.Is(err, horai.ErrTaskNotPaused),
		errors.Is(err, horai.ErrTaskNotDisabled),
		errors.Is(err, horai.ErrTaskDisabled),
		errors.Is(err, horai.ErrTaskCompleted):
		return status.Error(codes.FailedPrecondition, err.Error())
	case errors.Is(err, ErrPageTokensNotConfigured):
		return status.Error(codes.FailedPrecondition, err.Error())
	case errors.Is(err, horai.ErrScheduleConflict), errors.Is(err, horai.ErrInvalidArgument),
		errors.Is(err, ErrInvalidCursor), errors.Is(err, query.ErrInvalid), isFilterError(err):
		return status.Error(codes.InvalidArgument, err.Error())
	default:
		return status.Error(codes.Internal, "internal error")
	}
}

// isFilterError reports whether err is a [filter] failure caused by the
// caller's filter expression: syntax, shape, field allowlist, type and
// complexity limits. Server-side misconfiguration (missing allowlist,
// invalid parser options) is deliberately excluded and stays Internal.
func isFilterError(err error) bool {
	for _, target := range []error{
		filter.ErrParseFailed,
		filter.ErrEmptyExpression,
		filter.ErrExpressionTooLong,
		filter.ErrInvalidExpression,
		filter.ErrUnsupportedOperation,
		filter.ErrUnsupportedType,
		filter.ErrFieldNotAllowed,
		filter.ErrFunctionNotAllowed,
		filter.ErrFieldTypeMismatch,
		filter.ErrEnumValueNotAllowed,
		filter.ErrInvalidRegex,
		filter.ErrMaxDepthExceeded,
		filter.ErrMaxOperationsExceeded,
	} {
		if errors.Is(err, target) {
			return true
		}
	}
	return false
}
