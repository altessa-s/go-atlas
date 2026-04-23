// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package errstatus

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/health"

	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// callUnary is a test helper that invokes a unary server interceptor with the
// given handler error and returns the resulting gRPC status code.
func callUnary(t *testing.T, si *interceptor, handlerErr error) (codes.Code, error) {
	t.Helper()
	unary := si.ServerUnaryInterceptor()
	_, err := unary(t.Context(), nil, &grpc.UnaryServerInfo{}, func(ctx context.Context, req any) (any, error) {
		return nil, handlerErr
	})
	require.NotNil(t, err, "expected error from handler")
	st, ok := status.FromError(err)
	require.True(t, ok, "error should be a gRPC status")
	return st.Code(), err
}

func TestServerInterceptor_Name(t *testing.T) {
	i := ServerInterceptor()
	require.Equal(t, "errstatus", i.Name())
}

func TestServerInterceptor_Dependencies(t *testing.T) {
	si := ServerInterceptor()
	i, ok := si.(*interceptor)
	require.True(t, ok, "unexpected type")
	deps := i.Dependencies()
	require.ElementsMatch(t, []string{"requestid", "logger"}, deps)
}

func TestServerInterceptor_ReturnsInterceptors(t *testing.T) {
	i := ServerInterceptor()
	require.NotNil(t, i.ServerUnaryInterceptor(), "ServerUnaryInterceptor should not be nil")
	require.NotNil(t, i.ServerStreamInterceptor(), "ServerStreamInterceptor should not be nil")
}

func TestServerInterceptor_NoError_PassThrough(t *testing.T) {
	i := ServerInterceptor()
	unary := i.ServerUnaryInterceptor()
	resp, err := unary(t.Context(), "req", &grpc.UnaryServerInfo{}, func(ctx context.Context, req any) (any, error) {
		return "ok", nil
	})
	require.NoError(t, err)
	require.Equal(t, "ok", resp)
}

func TestServerInterceptor_ErrorConversion(t *testing.T) {
	errCustom := errors.New("custom domain error")
	si := ServerInterceptor(
		WithCacheDisabled(),
		WithErrorMapping(errCustom, codes.FailedPrecondition, "precondition failed"),
	).(*interceptor)

	tests := []struct {
		name     string
		err      error
		wantCode codes.Code
	}{
		{"plain_error_to_internal", errors.New("something broke"), codes.Internal},
		{"context_deadline_exceeded", context.DeadlineExceeded, codes.DeadlineExceeded},
		{"context_canceled", context.Canceled, codes.Canceled},
		{"health_unavailable", health.ErrServiceUnavailable, codes.Unavailable},
		{"wrapped_deadline", fmt.Errorf("wrapping: %w", context.DeadlineExceeded), codes.DeadlineExceeded},
		{"custom_error_mapping", errCustom, codes.FailedPrecondition},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, _ := callUnary(t, si, tt.err)
			require.Equal(t, tt.wantCode, got)
		})
	}
}

func TestServerInterceptor_GRPCStatusError_PreservesCode(t *testing.T) {
	si := ServerInterceptor().(*interceptor)
	got, _ := callUnary(t, si, status.Error(codes.PermissionDenied, "denied"))
	require.Equal(t, codes.PermissionDenied, got)
}

func TestServerInterceptor_Finalizer(t *testing.T) {
	t.Run("called_on_error", func(t *testing.T) {
		var called bool
		i := ServerInterceptor(WithFinalizer(func(ctx context.Context, err error) error {
			called = true
			return err
		}))
		unary := i.ServerUnaryInterceptor()
		_, _ = unary(t.Context(), nil, &grpc.UnaryServerInfo{}, func(ctx context.Context, req any) (any, error) {
			return nil, errors.New("trigger")
		})
		require.True(t, called, "finalizer should have been called")
	})

	t.Run("not_called_on_success", func(t *testing.T) {
		var called bool
		i := ServerInterceptor(WithFinalizer(func(ctx context.Context, err error) error {
			called = true
			return err
		}))
		unary := i.ServerUnaryInterceptor()
		_, err := unary(t.Context(), nil, &grpc.UnaryServerInfo{}, func(ctx context.Context, req any) (any, error) {
			return "ok", nil
		})
		require.NoError(t, err)
		require.False(t, called, "finalizer should not be called on success")
	})
}

func TestServerInterceptor_WithDomain_AutoWraps(t *testing.T) {
	i := ServerInterceptor(
		WithFinalizer(DefaultFinalizer),
		WithDomain[string]("auto.example.com"),
	)
	unary := i.ServerUnaryInterceptor()
	_, err := unary(t.Context(), nil, &grpc.UnaryServerInfo{}, func(ctx context.Context, req any) (any, error) {
		return nil, status.Error(codes.NotFound, "not found")
	})

	st, ok := status.FromError(err)
	require.True(t, ok, "error should be a gRPC status")

	for _, d := range st.Details() {
		if ei, ok := d.(*errdetails.ErrorInfo); ok {
			require.Equal(t, "auto.example.com", ei.Domain)
			return
		}
	}
	require.Fail(t, "ErrorInfo detail not found")
}

func TestServerInterceptor_StatusError_Interface(t *testing.T) {
	svc := &mockStatusErrorService{
		convertFn: func(ctx context.Context, err error) error {
			return status.Error(codes.AlreadyExists, "already exists")
		},
	}

	i := ServerInterceptor()
	unary := i.ServerUnaryInterceptor()
	_, err := unary(t.Context(), nil, &grpc.UnaryServerInfo{Server: svc}, func(ctx context.Context, req any) (any, error) {
		return nil, errors.New("some error")
	})

	st, ok := status.FromError(err)
	require.True(t, ok, "error should be a gRPC status")
	require.Equal(t, codes.AlreadyExists, st.Code())
}

func TestServerInterceptor_Cache(t *testing.T) {
	t.Run("hit_on_repeat_error", func(t *testing.T) {
		sentinel := errors.New("cached error")
		si := ServerInterceptor(
			WithErrorMapping(sentinel, codes.NotFound, "not found"),
			WithSentinelErrors(sentinel),
			WithCacheOnlySentinel(),
		).(*interceptor)

		code1, _ := callUnary(t, si, sentinel)
		code2, _ := callUnary(t, si, sentinel)

		require.Equal(t, code2, code1)
	})

	t.Run("disabled", func(t *testing.T) {
		si := ServerInterceptor(WithCacheDisabled()).(*interceptor)
		require.Equal(t, nil, si.cache)
	})
}

type mockStatusErrorService struct {
	convertFn func(context.Context, error) error
}

func (m *mockStatusErrorService) StatusErrorConvert(ctx context.Context, err error) error {
	return m.convertFn(ctx, err)
}
