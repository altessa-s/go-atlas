// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package errstatus

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func newClientInterceptor(t *testing.T, opts ...Option) *clientInterceptor {
	t.Helper()
	ci, ok := ClientInterceptor(opts...).(*clientInterceptor)
	require.True(t, ok, "unexpected type")
	return ci
}

func TestClientInterceptor_Name(t *testing.T) {
	i := ClientInterceptor()
	require.Equal(t, "errstatus", i.Name())
}

func TestClientInterceptor_ReturnsInterceptors(t *testing.T) {
	i := ClientInterceptor()
	require.NotNil(t, i.ClientUnaryInterceptor(), "ClientUnaryInterceptor should not be nil")
	require.NotNil(t, i.ClientStreamInterceptor(), "ClientStreamInterceptor should not be nil")
}

func TestClientInterceptor_BuiltInConversions(t *testing.T) {
	ci := newClientInterceptor(t)

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
			result := ci.convertError(t.Context(), tt.err)
			require.True(t, errors.Is(result, tt.wantErr), "got %v, want %v", result, tt.wantErr)
		})
	}
}

func TestClientInterceptor_NilError_ReturnsNil(t *testing.T) {
	ci := newClientInterceptor(t)
	result := ci.convertError(t.Context(), nil)
	require.NoError(t, result)
}

func TestClientInterceptor_NonGRPCError_PassThrough(t *testing.T) {
	ci := newClientInterceptor(t)
	plain := errors.New("plain error")
	result := ci.convertError(t.Context(), plain)
	require.Equal(t, plain, result)
}

func TestClientInterceptor_CustomStatusConverter(t *testing.T) {
	customErr := errors.New("custom app error")
	ci := newClientInterceptor(t, WithStatusMapping(codes.NotFound, customErr))

	result := ci.convertError(t.Context(), status.Error(codes.NotFound, "not found"))
	require.True(t, errors.Is(result, customErr), "got %v, want %v", result, customErr)
}

func TestClientInterceptor_CustomStatusConverterFunc(t *testing.T) {
	customErr := errors.New("func error")
	ci := newClientInterceptor(t, WithStatusConverterFunc(func(ctx context.Context, st *status.Status) error {
		if st.Code() == codes.Aborted {
			return customErr
		}
		return st.Err()
	}))

	result := ci.convertError(t.Context(), status.Error(codes.Aborted, "aborted"))
	require.True(t, errors.Is(result, customErr), "got %v, want %v", result, customErr)
}

func TestClientInterceptor_UnmatchedCode_ReturnsOriginal(t *testing.T) {
	ci := newClientInterceptor(t)
	result := ci.convertError(t.Context(), status.Error(codes.Internal, "internal"))

	st, ok := status.FromError(result)
	require.True(t, ok, "result should be a gRPC status")
	require.Equal(t, codes.Internal, st.Code())
}
