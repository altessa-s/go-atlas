// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package errstatus

import (
	"bytes"
	"fmt"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/transport/grpc/interceptors"

	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func withDetail(t *testing.T, code codes.Code, msg string) error {
	t.Helper()
	st, err := status.New(code, msg).WithDetails(&errdetails.ErrorInfo{Reason: "R"})
	require.NoError(t, err)
	return st.Err()
}

// TestServerInterceptor_StatusMessagePassthrough pins which statuses keep the
// handler's message: one with a client-facing code, without any wrapper text.
func TestServerInterceptor_StatusMessagePassthrough(t *testing.T) {
	t.Parallel()

	const secret = "dial tcp 10.0.0.7:5432: password authentication failed"
	tests := []struct {
		name        string
		err         error
		wantCode    codes.Code
		wantMessage string
		wantDetails int
	}{
		{"direct status keeps its message", status.Error(codes.NotFound, "user not found"), codes.NotFound, "user not found", 0},
		{"direct status keeps its details", withDetail(t, codes.NotFound, "user not found"), codes.NotFound, "user not found", 1},
		{
			"direct interceptors.Error keeps its message",
			interceptors.NewError(status.New(codes.PermissionDenied, "denied"), fmt.Errorf("policy: %s", secret)),
			codes.PermissionDenied, "denied", 0,
		},
		{
			"wrapped status loses the wrapper text",
			fmt.Errorf("load user from %s: %w", secret, status.Error(codes.NotFound, "user not found")),
			codes.NotFound, "user not found", 0,
		},
		{
			"wrapped status keeps its client-safe details",
			fmt.Errorf("load from %s: %w", secret, withDetail(t, codes.FailedPrecondition, "not ready")),
			codes.FailedPrecondition, "not ready", 1,
		},
		{
			"wrapped internal status",
			fmt.Errorf("query: %w", status.Error(codes.Internal, secret)),
			codes.Internal, MessageInternal, 0,
		},
		{"direct internal status", status.Error(codes.Internal, secret), codes.Internal, MessageInternal, 0},
		{"direct internal status loses its ErrorInfo", withDetail(t, codes.Internal, secret), codes.Internal, MessageInternal, 0},
		{"direct unknown status", status.Error(codes.Unknown, secret), codes.Unknown, "Unknown", 0},
		{"direct data loss status", status.Error(codes.DataLoss, secret), codes.DataLoss, "DataLoss", 0},
		{
			"direct internal interceptors.Error",
			interceptors.NewError(status.New(codes.Internal, secret), fmt.Errorf("cause: %s", secret)),
			codes.Internal, MessageInternal, 0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var logs bytes.Buffer
			si := ServerInterceptor(WithLogger(slog.New(slog.NewTextHandler(&logs, nil))), WithLogRawErrors(true)).(*interceptor)

			_, err := callUnary(t, si, tc.err)

			st, ok := status.FromError(err)
			require.True(t, ok)
			require.Equal(t, tc.wantCode, st.Code())
			require.Equal(t, tc.wantMessage, st.Message())
			require.NotContains(t, st.Message(), secret)
			require.Len(t, st.Details(), tc.wantDetails)

			if st.Message() != status.Convert(tc.err).Message() {
				require.Contains(t, logs.String(), "gRPC status message replaced", "the original must be logged")
				require.Contains(t, logs.String(), secret)
			} else {
				require.Empty(t, logs.String())
			}
		})
	}
}

// The original error stays reachable for the logger interceptor after the
// client-facing message was replaced.
func TestServerInterceptor_SanitizedStatusKeepsCause(t *testing.T) {
	t.Parallel()
	si := ServerInterceptor().(*interceptor)
	cause := fmt.Errorf("query: %w", status.Error(codes.Internal, "boom"))

	_, err := callUnary(t, si, cause)

	require.ErrorIs(t, err, cause)
}

func TestServerInterceptor_SanitizedStatusThroughFinalizer(t *testing.T) {
	t.Parallel()
	si := ServerInterceptor(WithFinalizer(DefaultFinalizerWithDomain("svc"))).(*interceptor)

	_, err := callUnary(t, si, fmt.Errorf("load 10.0.0.7: %w", status.Error(codes.NotFound, "user not found")))

	st, ok := status.FromError(err)
	require.True(t, ok)
	require.Equal(t, codes.NotFound, st.Code())
	require.Equal(t, "user not found", st.Message())
	require.Len(t, st.Details(), 1)
	ei, ok := st.Details()[0].(*errdetails.ErrorInfo)
	require.True(t, ok)
	require.Equal(t, "NOT_FOUND", ei.GetReason())
	require.Equal(t, "svc", ei.GetDomain())
}
