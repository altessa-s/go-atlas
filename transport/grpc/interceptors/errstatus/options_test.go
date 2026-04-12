// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package errstatus

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

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
	require.True(t, ok, "result is not a gRPC status error")
	for _, d := range st.Details() {
		if ei, ok := d.(*errdetails.ErrorInfo); ok {
			return ei
		}
	}
	require.Fail(t, "ErrorInfo detail not found")
	return nil
}

// requireRequestInfo extracts errdetails.RequestInfo from a gRPC status error.
// Returns nil if not found (does not fail).
func findRequestInfo(t *testing.T, err error) *errdetails.RequestInfo {
	t.Helper()
	st, ok := status.FromError(err)
	require.True(t, ok, "result is not a gRPC status error")
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
			require.Equal(t, tt.want, got)
		})
	}
}

func TestDefaultFinalizer(t *testing.T) {
	t.Run("adds_error_info", func(t *testing.T) {
		result := DefaultFinalizer(t.Context(), status.New(codes.NotFound, "not found").Err())
		ei := requireErrorInfo(t, result)
		require.Equal(t, "NOT_FOUND", ei.Reason)
		require.Equal(t, "", ei.Domain)
	})

	t.Run("injects_request_info", func(t *testing.T) {
		ctx := requestid.NewContext(t.Context(), "req-abc-123")
		result := DefaultFinalizer(ctx, status.New(codes.InvalidArgument, "bad").Err())
		ri := findRequestInfo(t, result)
		require.NotNil(t, ri, "RequestInfo detail not found")
		require.Equal(t, "req-abc-123", ri.RequestId)
	})

	t.Run("no_request_id_no_request_info", func(t *testing.T) {
		result := DefaultFinalizer(t.Context(), status.New(codes.NotFound, "not found").Err())
		ri := findRequestInfo(t, result)
		require.Nil(t, ri)
	})

	t.Run("non_grpc_error_pass_through", func(t *testing.T) {
		plain := errors.New("plain error")
		result := DefaultFinalizer(t.Context(), plain)
		require.Equal(t, plain, result)
	})
}

func TestDefaultFinalizerWithDomain(t *testing.T) {
	t.Run("sets_domain", func(t *testing.T) {
		finalizer := DefaultFinalizerWithDomain("myservice.example.com")
		result := finalizer(t.Context(), status.New(codes.Internal, "oops").Err())
		ei := requireErrorInfo(t, result)
		require.Equal(t, "myservice.example.com", ei.Domain)
	})

	t.Run("backfills_empty_domain", func(t *testing.T) {
		st, err := status.New(codes.NotFound, "not found").WithDetails(
			&errdetails.ErrorInfo{Reason: "NOT_FOUND", Domain: ""},
		)
		require.NoError(t, err)

		result := DefaultFinalizerWithDomain("backfill.example.com")(t.Context(), st.Err())
		ei := requireErrorInfo(t, result)
		require.Equal(t, "backfill.example.com", ei.Domain)
	})

	t.Run("preserves_existing_domain", func(t *testing.T) {
		st, err := status.New(codes.NotFound, "not found").WithDetails(
			&errdetails.ErrorInfo{Reason: "CUSTOM", Domain: "original.example.com"},
		)
		require.NoError(t, err)

		result := DefaultFinalizerWithDomain("should-not-override.example.com")(t.Context(), st.Err())
		ei := requireErrorInfo(t, result)
		require.Equal(t, "original.example.com", ei.Domain)
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
		require.True(t, called, "original finalizer should have been called")
	})

	t.Run("non_empty_domain", func(t *testing.T) {
		wrapped := withDomainFinalizer(DefaultFinalizer, "test.example.com")
		result := wrapped(t.Context(), status.New(codes.Internal, "fail").Err())
		ei := requireErrorInfo(t, result)
		require.Equal(t, "test.example.com", ei.Domain)
	})
}

func TestWithErrorMapping(t *testing.T) {
	sentinel := errors.New("not found")
	opts := newOptions(WithErrorMapping(sentinel, codes.NotFound, "resource not found"))

	require.Len(t, opts.errorConverters, 1)

	ctx := t.Context()
	converter := opts.errorConverters[0]

	require.True(t, converter.Matcher(ctx, sentinel), "Matcher should match sentinel error")
	require.False(t, converter.Matcher(ctx, errors.New("other")), "Matcher should not match different error")

	st := converter.Convert(ctx, sentinel)
	require.Equal(t, codes.NotFound, st.Code())
	require.Equal(t, "resource not found", st.Message())
}

func TestWithErrorMapping_EmptyMessage_UsesErrorString(t *testing.T) {
	sentinel := errors.New("my error text")
	opts := newOptions(WithErrorMapping(sentinel, codes.InvalidArgument, ""))
	st := opts.errorConverters[0].Convert(t.Context(), sentinel)
	require.Equal(t, "my error text", st.Message())
}

func TestWithStatusMapping(t *testing.T) {
	targetErr := errors.New("app error")
	opts := newOptions(WithStatusMapping(codes.NotFound, targetErr))

	require.Len(t, opts.statusConverter, 1)

	ctx := t.Context()
	converter := opts.statusConverter[0]

	require.True(t, converter.Matcher(ctx, status.New(codes.NotFound, "")), "Matcher should match NotFound status")
	require.False(t, converter.Matcher(ctx, status.New(codes.Internal, "")), "Matcher should not match Internal status")

	err := converter.Convert(ctx, status.New(codes.NotFound, ""))
	require.True(t, errors.Is(err, targetErr), "Convert returned %v, want %v", err, targetErr)
}

func TestWithStatusConverterFunc(t *testing.T) {
	opts := newOptions(WithStatusConverterFunc(func(ctx context.Context, st *status.Status) error {
		return errors.New("custom")
	}))

	require.Len(t, opts.statusConverter, 1)

	// matcher always returns true
	require.True(t, opts.statusConverter[0].Matcher(t.Context(), status.New(codes.OK, "")), "Matcher should always return true")
}
