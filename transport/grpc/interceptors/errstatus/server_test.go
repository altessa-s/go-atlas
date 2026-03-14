// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package errstatus

import (
	"context"
	"errors"
	"fmt"
	"testing"

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
	if err == nil {
		t.Fatal("expected error from handler")
	}
	st, ok := status.FromError(err)
	if !ok {
		t.Fatal("error should be a gRPC status")
	}
	return st.Code(), err
}

func TestServerInterceptor_Name(t *testing.T) {
	i := ServerInterceptor()
	if i.Name() != "errstatus" {
		t.Fatalf("Name() = %q, want %q", i.Name(), "errstatus")
	}
}

func TestServerInterceptor_Dependencies(t *testing.T) {
	si := ServerInterceptor()
	i, ok := si.(*interceptor)
	if !ok {
		t.Fatal("unexpected type")
	}
	deps := i.Dependencies()
	if len(deps) != 1 || deps[0] != "requestid" {
		t.Fatalf("Dependencies() = %v, want [requestid]", deps)
	}
}

func TestServerInterceptor_ReturnsInterceptors(t *testing.T) {
	i := ServerInterceptor()
	if i.ServerUnaryInterceptor() == nil {
		t.Fatal("ServerUnaryInterceptor should not be nil")
	}
	if i.ServerStreamInterceptor() == nil {
		t.Fatal("ServerStreamInterceptor should not be nil")
	}
}

func TestServerInterceptor_NoError_PassThrough(t *testing.T) {
	i := ServerInterceptor()
	unary := i.ServerUnaryInterceptor()
	resp, err := unary(t.Context(), "req", &grpc.UnaryServerInfo{}, func(ctx context.Context, req any) (any, error) {
		return "ok", nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp != "ok" {
		t.Fatalf("resp = %v, want ok", resp)
	}
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
			if got != tt.wantCode {
				t.Fatalf("code = %v, want %v", got, tt.wantCode)
			}
		})
	}
}

func TestServerInterceptor_GRPCStatusError_PreservesCode(t *testing.T) {
	si := ServerInterceptor().(*interceptor)
	got, _ := callUnary(t, si, status.Error(codes.PermissionDenied, "denied"))
	if got != codes.PermissionDenied {
		t.Fatalf("code = %v, want %v", got, codes.PermissionDenied)
	}
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
		if !called {
			t.Fatal("finalizer should have been called")
		}
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
		if err != nil {
			t.Fatal(err)
		}
		if called {
			t.Fatal("finalizer should not be called on success")
		}
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
	if !ok {
		t.Fatal("error should be a gRPC status")
	}

	for _, d := range st.Details() {
		if ei, ok := d.(*errdetails.ErrorInfo); ok {
			if ei.Domain != "auto.example.com" {
				t.Fatalf("ErrorInfo.Domain = %q, want %q", ei.Domain, "auto.example.com")
			}
			return
		}
	}
	t.Fatal("ErrorInfo detail not found")
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
	if !ok {
		t.Fatal("error should be a gRPC status")
	}
	if st.Code() != codes.AlreadyExists {
		t.Fatalf("code = %v, want %v", st.Code(), codes.AlreadyExists)
	}
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

		if code1 != code2 {
			t.Fatalf("cached code %v != first code %v", code2, code1)
		}
	})

	t.Run("disabled", func(t *testing.T) {
		si := ServerInterceptor(WithCacheDisabled()).(*interceptor)
		if si.cache != nil {
			t.Fatal("cache should be nil when disabled")
		}
	})
}

type mockStatusErrorService struct {
	convertFn func(context.Context, error) error
}

func (m *mockStatusErrorService) StatusErrorConvert(ctx context.Context, err error) error {
	return m.convertFn(ctx, err)
}
