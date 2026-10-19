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

	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
)

func TestCanonicalRules(t *testing.T) {
	t.Parallel()

	opts := []Option{WithCanonicalRules(), WithFinalizer(DefaultFinalizerWithDomain("svc"))}
	tests := []struct {
		name       string
		err        error
		wantCode   codes.Code
		wantMsg    string
		wantReason string
	}{
		{"not found", fmt.Errorf("bot b1: %w", coreerrs.ErrNotFound), codes.NotFound, "not found", "NOT_FOUND"},
		{"already exists", fmt.Errorf("bot b1: %w", coreerrs.ErrAlreadyExists), codes.AlreadyExists, "already exists", "ALREADY_EXISTS"},
		{
			"conflict", fmt.Errorf("replace bot b1 at version 3: %w", coreerrs.ErrConflict), codes.Aborted,
			"changed concurrently, re-read and retry", "VERSION_CONFLICT",
		},
		{
			"invalid argument publishes its text", fmt.Errorf("%w: unknown filter field", coreerrs.ErrInvalidArgument),
			codes.InvalidArgument, "invalid argument: unknown filter field", "INVALID_ARGUMENT",
		},
		{
			"failed precondition", fmt.Errorf("chain c1 in use: %w", coreerrs.ErrFailedPrecondition), codes.FailedPrecondition,
			"failed precondition", "FAILED_PRECONDITION",
		},
		{
			"unavailable", fmt.Errorf("dial 10.0.0.1: %w", coreerrs.ErrUnavailable), codes.Unavailable,
			"temporarily unavailable, retry later", "TEMPORARY_UNAVAILABLE",
		},
		{
			"busy", fmt.Errorf("lease of sync-1: %w", coreerrs.ErrBusy), codes.Aborted,
			"the same work is in progress, retry later", "BUSY",
		},
		{"unmatched", errors.New("boom"), codes.Internal, "Internal Server Error", "INTERNAL_ERROR"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := callWith(t, tc.err, opts...)
			st, ok := status.FromError(got)
			require.True(t, ok)
			require.Equal(t, tc.wantCode, st.Code())
			require.Equal(t, tc.wantMsg, st.Message())
			require.Len(t, st.Details(), 1, "only the ErrorInfo")
			ei, ok := st.Details()[0].(*errdetails.ErrorInfo)
			require.True(t, ok)
			require.Equal(t, tc.wantReason, ei.GetReason())
			require.Equal(t, "svc", ei.GetDomain())
		})
	}
}

func TestCanonicalRulesFieldErrors(t *testing.T) {
	t.Parallel()

	final := WithFinalizer(DefaultFinalizerWithDomain("svc"))
	fieldErr := fmt.Errorf("create: %w", coreerrs.NewFieldError("attrs.law_code", "required"))

	t.Run("field kept as written by default", func(t *testing.T) {
		t.Parallel()
		requireBadRequest(t, callWith(t, fieldErr, WithCanonicalRules(), final), "attrs.law_code", "required", "svc")
	})

	t.Run("field path mapper", func(t *testing.T) {
		t.Parallel()
		got := callWith(t, fieldErr, WithCanonicalRules(), WithFieldPathMapper(CamelPath), final)
		requireBadRequest(t, got, "attrs.lawCode", "required", "svc")
	})

	t.Run("mapper set before the rules applies too", func(t *testing.T) {
		t.Parallel()
		got := callWith(t, fieldErr, WithFieldPathMapper(CamelPath), WithCanonicalRules(), final)
		requireBadRequest(t, got, "attrs.lawCode", "required", "svc")
	})

	t.Run("re-rooted field error", func(t *testing.T) {
		t.Parallel()
		err := coreerrs.MapFieldPath(fieldErr, Prefixed("referenceItem"))
		got := callWith(t, err, WithCanonicalRules(), WithFieldPathMapper(CamelPath), final)
		requireBadRequest(t, got, "referenceItem.attrs.lawCode", "required", "svc")
	})
}

func TestCanonicalRulesOrder(t *testing.T) {
	t.Parallel()

	field := coreerrs.NewFieldError("name", "required")
	errQuota := errors.New("quota")
	tests := []struct {
		name     string
		err      error
		opts     []Option
		wantCode codes.Code
	}{
		{"not found before a field error", errors.Join(coreerrs.ErrNotFound, field), nil, codes.NotFound},
		{"conflict before a field error", errors.Join(field, coreerrs.ErrConflict), nil, codes.Aborted},
		{"field error before invalid argument text", errors.Join(field, coreerrs.ErrInvalidArgument), nil, codes.InvalidArgument},
		{"field error before failed precondition", errors.Join(coreerrs.ErrFailedPrecondition, field), nil, codes.InvalidArgument},
		{"invalid argument before busy", errors.Join(coreerrs.ErrBusy, coreerrs.ErrInvalidArgument), nil, codes.InvalidArgument},
		{
			"service rule after the canonical ones", fmt.Errorf("tenant: %w", errQuota),
			[]Option{WithErrorRules(ErrorRule{Target: errQuota, Code: codes.ResourceExhausted})}, codes.ResourceExhausted,
		},
		{
			"canonical rule wins over a later service rule", errors.Join(errQuota, coreerrs.ErrNotFound),
			[]Option{WithErrorRules(ErrorRule{Target: errQuota, Code: codes.ResourceExhausted})}, codes.NotFound,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			opts := append([]Option{WithCanonicalRules()}, tc.opts...)
			require.Equal(t, tc.wantCode, status.Code(callWith(t, tc.err, opts...)))
		})
	}

	t.Run("a rule registered before overrides a canonical one", func(t *testing.T) {
		t.Parallel()
		override := WithErrorRules(ErrorRule{Target: coreerrs.ErrBusy, Code: codes.FailedPrecondition, Message: "already running"})
		got := callWith(t, coreerrs.ErrBusy, override, WithCanonicalRules())
		require.Equal(t, codes.FailedPrecondition, status.Code(got))
		require.Equal(t, "already running", status.Convert(got).Message())
	})
}

// TestCanonicalInvalidArgumentTextNotCached pins that one request's
// InvalidArgument text or field never answers another request, with the
// sentinel cache keyed on ErrInvalidArgument.
func TestCanonicalInvalidArgumentTextNotCached(t *testing.T) {
	t.Parallel()

	unary := ServerInterceptor(
		WithCanonicalRules(), WithFieldPathMapper(CamelPath),
		WithCacheOnlySentinel(), WithSentinelErrors(coreerrs.ErrInvalidArgument),
	).ServerUnaryInterceptor()
	call := func(err error) string {
		_, got := unary(t.Context(), nil, &grpc.UnaryServerInfo{FullMethod: "/s/M"},
			func(context.Context, any) (any, error) { return nil, err })
		return status.Convert(got).Message()
	}
	require.Equal(t, "invalid argument: tenant A", call(fmt.Errorf("%w: tenant A", coreerrs.ErrInvalidArgument)))
	require.Equal(t, "invalid argument: tenant B", call(fmt.Errorf("%w: tenant B", coreerrs.ErrInvalidArgument)))
	require.Equal(t, "baseUrl: required", call(coreerrs.NewFieldError("base_url", "required")))
	require.Equal(t, "provider.baseUrl: required",
		call(coreerrs.MapFieldPath(coreerrs.NewFieldError("base_url", "required"), Prefixed("provider"))))
}
