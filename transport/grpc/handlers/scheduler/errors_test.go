// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package scheduler

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-aip/filter"
	"github.com/altessa-s/go-horai/query"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	horai "github.com/altessa-s/go-horai"
)

func TestMapError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		code codes.Code
	}{
		{"not_found", horai.ErrTaskNotFound, codes.NotFound},
		{"page_tokens_not_configured", ErrPageTokensNotConfigured, codes.FailedPrecondition},
		{"not_registered", horai.ErrTaskNotRegistered, codes.NotFound},
		{"unmanaged", horai.ErrTaskUnmanaged, codes.FailedPrecondition},
		{"not_paused", horai.ErrTaskNotPaused, codes.FailedPrecondition},
		{"not_disabled", horai.ErrTaskNotDisabled, codes.FailedPrecondition},
		{"disabled", horai.ErrTaskDisabled, codes.FailedPrecondition},
		{"completed", horai.ErrTaskCompleted, codes.FailedPrecondition},
		{"schedule_conflict", horai.ErrScheduleConflict, codes.InvalidArgument},
		{"invalid_argument", horai.ErrInvalidArgument, codes.InvalidArgument},
		{"invalid_cursor", invalidCursor(errors.New("bad")), codes.InvalidArgument},
		{"invalid_query", fmt.Errorf("list: %w", query.ErrInvalid), codes.InvalidArgument},
		{"filter_parse", fmt.Errorf("parse: %w", filter.ErrParseFailed), codes.InvalidArgument},
		{"filter_field_not_allowed", fmt.Errorf("eval: %w", filter.ErrFieldNotAllowed), codes.InvalidArgument},
		{"filter_server_misconfig", filter.ErrAllowlistRequired, codes.Internal},
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
