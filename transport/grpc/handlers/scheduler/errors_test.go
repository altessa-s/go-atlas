// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package scheduler

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	sched "github.com/altessa-s/go-atlas/service/scheduler"
)

func TestMapError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		code codes.Code
	}{
		{"not_found", sched.ErrTaskNotFound, codes.NotFound},
		{"not_registered", sched.ErrTaskNotRegistered, codes.NotFound},
		{"unmanaged", sched.ErrTaskUnmanaged, codes.FailedPrecondition},
		{"not_paused", sched.ErrTaskNotPaused, codes.FailedPrecondition},
		{"not_disabled", sched.ErrTaskNotDisabled, codes.FailedPrecondition},
		{"disabled", sched.ErrTaskDisabled, codes.FailedPrecondition},
		{"completed", sched.ErrTaskCompleted, codes.FailedPrecondition},
		{"schedule_conflict", sched.ErrScheduleConflict, codes.InvalidArgument},
		{"generic", errors.New("something"), codes.Internal},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := mapError(tt.err)
			st, ok := status.FromError(result)
			require.True(t, ok, "expected gRPC status error")
			require.Equal(t, tt.code, st.Code())
		})
	}
}

func TestMapError_internal_hides_details(t *testing.T) {
	secret := "connection refused to db-prod.internal:5432"
	result := mapError(errors.New(secret))

	st, ok := status.FromError(result)
	require.True(t, ok, "expected gRPC status error")
	require.Equal(t, codes.Internal, st.Code())
	msg := st.Message()
	require.Equal(t, "internal error", msg)
}
