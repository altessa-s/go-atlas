// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package errstatus

import (
	"context"
	"errors"
	"testing"

	"github.com/altessa-s/go-atlas/transport/internal/requestid"

	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// requireErrorInfo extracts errdetails.ErrorInfo from a gRPC status error.
// Fails the test if the error is not a gRPC status or has no ErrorInfo detail.
func requireErrorInfo(t *testing.T, err error) *errdetails.ErrorInfo {
	t.Helper()
	st, ok := status.FromError(err)
	if !ok {
		t.Fatal("result is not a gRPC status error")
	}
	for _, d := range st.Details() {
		if ei, ok := d.(*errdetails.ErrorInfo); ok {
			return ei
		}
	}
	t.Fatal("ErrorInfo detail not found")
	return nil
}

// requireRequestInfo extracts errdetails.RequestInfo from a gRPC status error.
// Returns nil if not found (does not fail).
func findRequestInfo(t *testing.T, err error) *errdetails.RequestInfo {
	t.Helper()
	st, ok := status.FromError(err)
	if !ok {
		t.Fatal("result is not a gRPC status error")
	}
	for _, d := range st.Details() {
		if ri, ok := d.(*errdetails.RequestInfo); ok {
			return ri
		}
	}
	return nil
}

func TestGrpcStatusToReasonCode(t *testing.T) {
	tests := []struct {
		code codes.Code
		want string
	}{
		{codes.NotFound, "NOT_FOUND"},
		{codes.PermissionDenied, "FORBIDDEN"},
		{codes.InvalidArgument, "INVALID_ARGUMENT"},
		{codes.Unavailable, "TEMPORARY_UNAVAILABLE"},
		{codes.Internal, "INTERNAL_ERROR"},
		{codes.Unimplemented, "UNIMPLEMENTED"},
		{codes.Aborted, "ABORTED"},
		{codes.Unauthenticated, "UNAUTHENTICATED"},
		{codes.FailedPrecondition, "FAILED_PRECONDITION"},
		{codes.DeadlineExceeded, "DEADLINE_EXCEEDED"},
		{codes.ResourceExhausted, "RESOURCE_EXHAUSTED"},
		{codes.Canceled, "CANCELED"},
		{codes.OK, "UNKNOWN"},
		{codes.AlreadyExists, "UNKNOWN"},
	}
	for _, tt := range tests {
		t.Run(tt.code.String(), func(t *testing.T) {
			got := GrpcStatusToReasonCode(status.New(tt.code, "test"))
			if got != tt.want {
				t.Fatalf("GrpcStatusToReasonCode(%v) = %q, want %q", tt.code, got, tt.want)
			}
		})
	}
}

func TestDefaultFinalizer(t *testing.T) {
	t.Run("adds_error_info", func(t *testing.T) {
		result := DefaultFinalizer(t.Context(), status.New(codes.NotFound, "not found").Err())
		ei := requireErrorInfo(t, result)
		if ei.Reason != "NOT_FOUND" {
			t.Fatalf("ErrorInfo.Reason = %q, want %q", ei.Reason, "NOT_FOUND")
		}
		if ei.Domain != "" {
			t.Fatalf("ErrorInfo.Domain = %q, want empty", ei.Domain)
		}
	})

	t.Run("injects_request_info", func(t *testing.T) {
		ctx := requestid.NewContext(t.Context(), "req-abc-123")
		result := DefaultFinalizer(ctx, status.New(codes.InvalidArgument, "bad").Err())
		ri := findRequestInfo(t, result)
		if ri == nil {
			t.Fatal("RequestInfo detail not found")
		}
		if ri.RequestId != "req-abc-123" {
			t.Fatalf("RequestInfo.RequestId = %q, want %q", ri.RequestId, "req-abc-123")
		}
	})

	t.Run("no_request_id_no_request_info", func(t *testing.T) {
		result := DefaultFinalizer(t.Context(), status.New(codes.NotFound, "not found").Err())
		if ri := findRequestInfo(t, result); ri != nil {
			t.Fatal("RequestInfo should not be present without request ID in context")
		}
	})

	t.Run("non_grpc_error_pass_through", func(t *testing.T) {
		plain := errors.New("plain error")
		if result := DefaultFinalizer(t.Context(), plain); result != plain {
			t.Fatal("non-gRPC error should be returned as-is")
		}
	})
}

func TestDefaultFinalizerWithDomain(t *testing.T) {
	t.Run("sets_domain", func(t *testing.T) {
		finalizer := DefaultFinalizerWithDomain("myservice.example.com")
		result := finalizer(t.Context(), status.New(codes.Internal, "oops").Err())
		ei := requireErrorInfo(t, result)
		if ei.Domain != "myservice.example.com" {
			t.Fatalf("ErrorInfo.Domain = %q, want %q", ei.Domain, "myservice.example.com")
		}
	})

	t.Run("backfills_empty_domain", func(t *testing.T) {
		st, err := status.New(codes.NotFound, "not found").WithDetails(
			&errdetails.ErrorInfo{Reason: "NOT_FOUND", Domain: ""},
		)
		if err != nil {
			t.Fatal(err)
		}

		result := DefaultFinalizerWithDomain("backfill.example.com")(t.Context(), st.Err())
		ei := requireErrorInfo(t, result)
		if ei.Domain != "backfill.example.com" {
			t.Fatalf("ErrorInfo.Domain = %q, want %q", ei.Domain, "backfill.example.com")
		}
	})

	t.Run("preserves_existing_domain", func(t *testing.T) {
		st, err := status.New(codes.NotFound, "not found").WithDetails(
			&errdetails.ErrorInfo{Reason: "CUSTOM", Domain: "original.example.com"},
		)
		if err != nil {
			t.Fatal(err)
		}

		result := DefaultFinalizerWithDomain("should-not-override.example.com")(t.Context(), st.Err())
		ei := requireErrorInfo(t, result)
		if ei.Domain != "original.example.com" {
			t.Fatalf("ErrorInfo.Domain = %q, want %q (should not be overridden)", ei.Domain, "original.example.com")
		}
	})
}

func TestWithDomainFinalizer(t *testing.T) {
	t.Run("empty_domain_unchanged", func(t *testing.T) {
		var called bool
		original := Finalizer(func(ctx context.Context, err error) error {
			called = true
			return err
		})
		wrapped := withDomainFinalizer(original, "")
		_ = wrapped(t.Context(), errors.New("test"))
		if !called {
			t.Fatal("original finalizer should have been called")
		}
	})

	t.Run("non_empty_domain", func(t *testing.T) {
		wrapped := withDomainFinalizer(DefaultFinalizer, "test.example.com")
		result := wrapped(t.Context(), status.New(codes.Internal, "fail").Err())
		ei := requireErrorInfo(t, result)
		if ei.Domain != "test.example.com" {
			t.Fatalf("ErrorInfo.Domain = %q, want %q", ei.Domain, "test.example.com")
		}
	})
}

func TestWithErrorMapping(t *testing.T) {
	sentinel := errors.New("not found")
	opts := newOptions(WithErrorMapping(sentinel, codes.NotFound, "resource not found"))

	if len(opts.errorConverters) != 1 {
		t.Fatalf("errorConverters len = %d, want 1", len(opts.errorConverters))
	}

	ctx := t.Context()
	converter := opts.errorConverters[0]

	if !converter.Matcher(ctx, sentinel) {
		t.Fatal("Matcher should match sentinel error")
	}
	if converter.Matcher(ctx, errors.New("other")) {
		t.Fatal("Matcher should not match different error")
	}

	st := converter.Convert(ctx, sentinel)
	if st.Code() != codes.NotFound {
		t.Fatalf("code = %v, want %v", st.Code(), codes.NotFound)
	}
	if st.Message() != "resource not found" {
		t.Fatalf("message = %q, want %q", st.Message(), "resource not found")
	}
}

func TestWithErrorMapping_EmptyMessage_UsesErrorString(t *testing.T) {
	sentinel := errors.New("my error text")
	opts := newOptions(WithErrorMapping(sentinel, codes.InvalidArgument, ""))
	st := opts.errorConverters[0].Convert(t.Context(), sentinel)
	if st.Message() != "my error text" {
		t.Fatalf("message = %q, want %q", st.Message(), "my error text")
	}
}

func TestWithStatusMapping(t *testing.T) {
	targetErr := errors.New("app error")
	opts := newOptions(WithStatusMapping(codes.NotFound, targetErr))

	if len(opts.statusConverter) != 1 {
		t.Fatalf("statusConverter len = %d, want 1", len(opts.statusConverter))
	}

	ctx := t.Context()
	converter := opts.statusConverter[0]

	if !converter.Matcher(ctx, status.New(codes.NotFound, "")) {
		t.Fatal("Matcher should match NotFound status")
	}
	if converter.Matcher(ctx, status.New(codes.Internal, "")) {
		t.Fatal("Matcher should not match Internal status")
	}

	if err := converter.Convert(ctx, status.New(codes.NotFound, "")); !errors.Is(err, targetErr) {
		t.Fatalf("Convert returned %v, want %v", err, targetErr)
	}
}

func TestWithStatusConverterFunc(t *testing.T) {
	opts := newOptions(WithStatusConverterFunc(func(ctx context.Context, st *status.Status) error {
		return errors.New("custom")
	}))

	if len(opts.statusConverter) != 1 {
		t.Fatalf("statusConverter len = %d, want 1", len(opts.statusConverter))
	}

	// matcher always returns true
	if !opts.statusConverter[0].Matcher(t.Context(), status.New(codes.OK, "")) {
		t.Fatal("Matcher should always return true")
	}
}
