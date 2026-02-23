// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package scheduler

import (
	"errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	sched "github.com/altessa-s/go-atlas/service/scheduler"
)

// mapError translates scheduler domain errors into gRPC status errors:
//
//   - ErrTaskNotFound, ErrTaskNotRegistered       --> codes.NotFound
//   - ErrTaskUnmanaged, ErrTaskNotPaused, etc.    --> codes.FailedPrecondition
//   - ErrScheduleConflict                         --> codes.InvalidArgument
//   - everything else                             --> codes.Internal
func mapError(err error) error {
	switch {
	case errors.Is(err, sched.ErrTaskNotFound),
		errors.Is(err, sched.ErrTaskNotRegistered):
		return status.Error(codes.NotFound, err.Error())
	case errors.Is(err, sched.ErrTaskUnmanaged),
		errors.Is(err, sched.ErrTaskNotPaused),
		errors.Is(err, sched.ErrTaskNotDisabled),
		errors.Is(err, sched.ErrTaskDisabled),
		errors.Is(err, sched.ErrTaskCompleted):
		return status.Error(codes.FailedPrecondition, err.Error())
	case errors.Is(err, sched.ErrScheduleConflict):
		return status.Error(codes.InvalidArgument, err.Error())
	default:
		return status.Error(codes.Internal, err.Error())
	}
}
