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

	"github.com/altessa-s/proto-gen-go/io/altessa/badrequest/v1"

	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// testFieldError is a domain field error with a snake_case domain path.
type testFieldError struct{ field, reason string }

func (e *testFieldError) Error() string { return "invalid argument: " + e.field + ": " + e.reason }

var errMissing = errors.New("missing")

// callWith runs err through a unary server interceptor built from opts and
// returns the error the client sees.
func callWith(t *testing.T, err error, opts ...Option) error {
	t.Helper()
	unary := ServerInterceptor(opts...).ServerUnaryInterceptor()
	_, got := unary(t.Context(), nil, &grpc.UnaryServerInfo{FullMethod: "/s/M"},
		func(context.Context, any) (any, error) { return nil, err })
	return got
}

// requireBadRequest asserts err is InvalidArgument "<path>: <reason>" whose
// details are exactly one BadRequest violation followed by the finalizer's
// ErrorInfo (INVALID_ARGUMENT, domain).
func requireBadRequest(t *testing.T, err error, path, reason, domain string) {
	t.Helper()
	st, ok := status.FromError(err)
	require.True(t, ok, "err = %v", err)
	require.Equal(t, codes.InvalidArgument, st.Code())
	require.Equal(t, path+": "+reason, st.Message())
	details := st.Details()
	require.Len(t, details, 2)
	br, ok := details[0].(*badrequestv1.BadRequest)
	require.True(t, ok, "first detail = %T", details[0])
	require.Len(t, br.GetFieldViolations(), 1)
	require.Equal(t, path, br.GetFieldViolations()[0].GetFieldPath())
	require.Equal(t, reason, br.GetFieldViolations()[0].GetMessage())
	require.Nil(t, br.GetFieldViolations()[0].Code)
	ei, ok := details[1].(*errdetails.ErrorInfo)
	require.True(t, ok, "second detail = %T", details[1])
	require.Equal(t, "INVALID_ARGUMENT", ei.GetReason())
	require.Equal(t, domain, ei.GetDomain())
}

func TestWithFieldErrorType(t *testing.T) {
	t.Parallel()

	camel := WithFieldErrorType(func(e *testFieldError) (string, string) { return CamelPath(e.field), e.reason })
	final := WithFinalizer(DefaultFinalizerWithDomain("svc"))

	t.Run("wrapped field error becomes a violation", func(t *testing.T) {
		t.Parallel()
		err := fmt.Errorf("create: %w", &testFieldError{field: "attrs.law_code", reason: "required"})
		requireBadRequest(t, callWith(t, err, camel, final), "attrs.lawCode", "required", "svc")
	})

	t.Run("joined field error is found", func(t *testing.T) {
		t.Parallel()
		err := errors.Join(errors.New("other"), &testFieldError{field: "name", reason: "too long"})
		requireBadRequest(t, callWith(t, err, camel, final), "name", "too long", "svc")
	})

	t.Run("other errors fall through", func(t *testing.T) {
		t.Parallel()
		got := callWith(t, errMissing, camel, WithErrorRules(ErrorRule{Target: errMissing, Code: codes.NotFound}))
		require.Equal(t, codes.NotFound, status.Code(got))
	})

	t.Run("a rule registered first wins", func(t *testing.T) {
		t.Parallel()
		err := errors.Join(errMissing, &testFieldError{field: "name", reason: "required"})
		rule := WithErrorRules(ErrorRule{Target: errMissing, Code: codes.NotFound, Message: "not found"})
		require.Equal(t, codes.NotFound, status.Code(callWith(t, err, rule, camel)))
		require.Equal(t, codes.InvalidArgument, status.Code(callWith(t, err, camel, rule)))
	})

	t.Run("a status the handler returned passes through", func(t *testing.T) {
		t.Parallel()
		got := callWith(t, status.Error(codes.Aborted, "busy"), camel)
		require.Equal(t, codes.Aborted, status.Code(got))
	})
}

// TestFieldErrorTypeNeverCached pins that a field violation is never served
// from the conversion cache to another field error, in either cache mode: the
// status depends on the field, which neither cache key captures reliably.
func TestFieldErrorTypeNeverCached(t *testing.T) {
	t.Parallel()

	sentinelErr := errors.New("validation")
	byField := WithFieldErrorType(func(e *wrappingFieldError) (string, string) { return e.field, e.reason })
	for name, mode := range map[string][]Option{
		"text keys":     nil,
		"sentinel keys": {WithCacheOnlySentinel(), WithSentinelErrors(sentinelErr)},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			opts := append([]Option{byField}, mode...)
			unary := ServerInterceptor(opts...).ServerUnaryInterceptor()
			call := func(err error) string {
				_, got := unary(t.Context(), nil, &grpc.UnaryServerInfo{FullMethod: "/s/M"},
					func(context.Context, any) (any, error) { return nil, err })
				return status.Convert(got).Message()
			}
			// Same error text, different fields: a text-keyed cache would
			// confuse them, and so would a sentinel-keyed one.
			a := &wrappingFieldError{field: "a.name", reason: "required", cause: sentinelErr}
			b := &wrappingFieldError{field: "b.name", reason: "required", cause: sentinelErr}
			require.Equal(t, "a.name: required", call(a))
			require.Equal(t, "b.name: required", call(b))
			require.Equal(t, "a.name: required", call(a))
		})
	}
}

// wrappingFieldError is a field error whose text omits the field (like a
// re-rooted domain error that keeps its original text) and that wraps cause.
type wrappingFieldError struct {
	field, reason string
	cause         error
}

func (e *wrappingFieldError) Error() string { return "invalid: " + e.reason }
func (e *wrappingFieldError) Unwrap() error { return e.cause }
