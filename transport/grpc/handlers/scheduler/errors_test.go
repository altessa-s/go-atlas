// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package scheduler

import (
	"errors"
	"testing"

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
			if !ok {
				t.Fatal("expected gRPC status error")
			}
			if st.Code() != tt.code {
				t.Fatalf("code = %v, want %v", st.Code(), tt.code)
			}
		})
	}
}
