// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package idempotency

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/idempotency"

	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// findErrorInfo returns the [errdetails.ErrorInfo] detail from st or nil.
func findErrorInfo(t *testing.T, st *status.Status) *errdetails.ErrorInfo {
	t.Helper()
	for _, d := range st.Details() {
		if ei, ok := d.(*errdetails.ErrorInfo); ok {
			return ei
		}
	}
	return nil
}

func TestDefaultStatusCreator(t *testing.T) {
	tests := []struct {
		name       string
		scenario   ErrorScenario
		state      *idempotency.State
		wantCode   codes.Code
		wantMsg    string
		wantReason string
	}{
		{
			name:       "missing",
			scenario:   ErrorIDKMissing,
			wantCode:   codes.InvalidArgument,
			wantMsg:    "Idempotency key is required for this operation",
			wantReason: ReasonIDKMissing,
		},
		{
			name:       "invalid_format",
			scenario:   ErrorIDKInvalidFormat,
			wantCode:   codes.InvalidArgument,
			wantMsg:    "Idempotency key must be a valid lowercase UUID v4",
			wantReason: ReasonIDKInvalidFormat,
		},
		{
			name:       "in_progress",
			scenario:   ErrorIDKInProgress,
			wantCode:   codes.Aborted,
			wantMsg:    "A request with this idempotency key is currently being processed",
			wantReason: ReasonIDKInProgress,
		},
		{
			name:       "already_used",
			scenario:   ErrorIDKAlreadyUsed,
			state:      &idempotency.State{Data: "550e8400-e29b-41d4-a716-446655440000"},
			wantCode:   codes.FailedPrecondition,
			wantMsg:    "This idempotency key has already been used",
			wantReason: ReasonIDKAlreadyUsed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st := defaultStatusCreator(context.Background(), tt.scenario, tt.state)

			require.Equal(t, tt.wantCode, st.Code())
			require.Equal(t, tt.wantMsg, st.Message())

			ei := findErrorInfo(t, st)
			require.NotNil(t, ei, "ErrorInfo detail must be attached")
			require.Equal(t, tt.wantReason, ei.GetReason())
			require.Empty(t, ei.GetMetadata())
		})
	}
}

func TestDefaultStatusCreator_UnknownScenario(t *testing.T) {
	st := defaultStatusCreator(context.Background(), ErrorScenario(999), nil)

	require.Equal(t, codes.Internal, st.Code())
	require.Equal(t, "Unknown idempotency error", st.Message())
	require.Nil(t, findErrorInfo(t, st), "unknown scenario must not attach ErrorInfo")
}
