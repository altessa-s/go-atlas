// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package errstatus

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/transport/grpc/interceptors"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func convertClientError(t testing.TB, ci interceptors.ClientInterceptor, err error) error {
	t.Helper()
	return ci.ClientUnaryInterceptor()(t.Context(), "/example/Method", nil, nil, nil,
		func(context.Context, string, any, any, *grpc.ClientConn, ...grpc.CallOption) error {
			return err
		})
}

func TestClientInterceptor_Name(t *testing.T) {
	t.Parallel()
	i := ClientInterceptor()
	require.Equal(t, "errstatus", i.Name())
}

func TestClientInterceptor_ReturnsInterceptors(t *testing.T) {
	t.Parallel()
	i := ClientInterceptor()
	require.NotNil(t, i.ClientUnaryInterceptor(), "ClientUnaryInterceptor should not be nil")
	require.NotNil(t, i.ClientStreamInterceptor(), "ClientStreamInterceptor should not be nil")
}

func TestClientInterceptor_BuiltInConversions(t *testing.T) {
	t.Parallel()
	ci := ClientInterceptor()

	tests := []struct {
		name    string
		err     error
		wantErr error
	}{
		{"deadline_exceeded", status.Error(codes.DeadlineExceeded, "timeout"), context.DeadlineExceeded},
		{"canceled", status.Error(codes.Canceled, "canceled"), context.Canceled},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := convertClientError(t, ci, tt.err)
			require.True(t, errors.Is(result, tt.wantErr), "got %v, want %v", result, tt.wantErr)
		})
	}
}

func TestClientInterceptor_NilError_ReturnsNil(t *testing.T) {
	t.Parallel()
	ci := ClientInterceptor()
	result := convertClientError(t, ci, nil)
	require.NoError(t, result)
}

func TestClientInterceptor_NonGRPCError_PassThrough(t *testing.T) {
	t.Parallel()
	ci := ClientInterceptor()
	plain := errors.New("plain error")
	result := convertClientError(t, ci, plain)
	require.Equal(t, plain, result)
}

func TestClientInterceptor_CustomStatusConverter(t *testing.T) {
	t.Parallel()
	customErr := errors.New("custom app error")
	ci := ClientInterceptor(WithStatusMapping(codes.NotFound, customErr))

	result := convertClientError(t, ci, status.Error(codes.NotFound, "not found"))
	require.True(t, errors.Is(result, customErr), "got %v, want %v", result, customErr)
}

func TestClientInterceptor_CustomStatusConverterFunc(t *testing.T) {
	t.Parallel()
	customErr := errors.New("func error")
	ci := ClientInterceptor(WithStatusConverterFunc(func(ctx context.Context, st *status.Status) error {
		if st.Code() == codes.Aborted {
			return customErr
		}
		return st.Err()
	}))

	result := convertClientError(t, ci, status.Error(codes.Aborted, "aborted"))
	require.True(t, errors.Is(result, customErr), "got %v, want %v", result, customErr)
}

func TestClientInterceptor_UnmatchedCode_ReturnsOriginal(t *testing.T) {
	t.Parallel()
	ci := ClientInterceptor()
	result := convertClientError(t, ci, status.Error(codes.Internal, "internal"))

	st, ok := status.FromError(result)
	require.True(t, ok, "result should be a gRPC status")
	require.Equal(t, codes.Internal, st.Code())
}

func TestClientInterceptor_StatusMatchersRunOnRequestsInOrder(t *testing.T) {
	t.Parallel()
	first := errors.New("domain failure")
	second := errors.New("code mapping")
	calls := 0
	ci := ClientInterceptor(
		WithStatusConverters(StatusConverter{
			Matcher: func(ctx context.Context, st *status.Status) bool {
				calls++
				require.Equal(t, t.Context(), ctx)
				return st.Code() == codes.NotFound || st.Message() == "domain"
			},
			Convert: func(context.Context, *status.Status) error { return first },
		}),
		WithStatusMapping(codes.Aborted, second),
	)
	require.Zero(t, calls)
	require.ErrorIs(t, convertClientError(t, ci, status.Error(codes.Aborted, "domain")), first)
	require.ErrorIs(t, convertClientError(t, ci, status.Error(codes.Aborted, "other")), second)
	require.Equal(t, 2, calls)
}
