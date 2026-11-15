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
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var errScrubDB = errors.New("db failure")

type tenantError struct{ tenant string }

func (e *tenantError) Error() string { return "tenant=" + e.tenant + " sql=SELECT secret" }

// valueError is an error held by value, which has no address to key a cache on.
type valueError struct{ msg string }

func (e valueError) Error() string { return e.msg }

// A converter that maps to a server-fault code must not publish the error
// text, a DebugInfo or diagnostic details, and one request's text must never
// be served to another from the conversion cache.
func TestServerInterceptor_ConverterOutputIsScrubbed(t *testing.T) {
	t.Parallel()

	debugConverter := WithErrorConverters(ErrorConverter{
		Matcher: func(_ context.Context, err error) bool { return errors.Is(err, errScrubDB) },
		Convert: func(_ context.Context, err error) *status.Status {
			return statusWith(t, codes.Internal, err.Error(), debugInfo(), &errdetails.ErrorInfo{Reason: "DB"})
		},
	})
	tests := []struct {
		name string
		opts []Option
		code codes.Code
		want string
	}{
		{"error mapping, empty message", []Option{WithErrorMapping(errScrubDB, codes.Internal, "")}, codes.Internal, MessageInternal},
		{"error mapping, explicit message", []Option{WithErrorMapping(errScrubDB, codes.Unknown, "db is down")}, codes.Unknown, "Unknown"},
		{
			"error type mapping",
			[]Option{WithErrorTypeMapping(codes.DataLoss, func(e *tenantError) string { return e.Error() })},
			codes.DataLoss, "DataLoss",
		},
		{
			"exposed error rule",
			[]Option{WithErrorRules(ErrorRule{Target: errScrubDB, Code: codes.Internal, Expose: true})},
			codes.Internal, MessageInternal,
		},
		{"custom converter with details", []Option{debugConverter}, codes.Internal, MessageInternal},
	}

	for _, tc := range tests {
		for _, cacheOpts := range [][]Option{nil, {WithCacheDisabled()}} {
			t.Run(fmt.Sprintf("%s/cache-disabled=%t", tc.name, cacheOpts != nil), func(t *testing.T) {
				t.Parallel()
				si := ServerInterceptor(append(tc.opts, cacheOpts...)...).(*interceptor)

				for _, tenant := range []string{"A", "B", "A"} {
					_, err := callUnary(t, si, fmt.Errorf("%w: %w", errScrubDB, &tenantError{tenant: tenant}))

					st := status.Convert(err)
					require.Equal(t, tc.code, st.Code())
					require.Equal(t, tc.want, st.Message())
					require.Empty(t, st.Details())
					require.NotContains(t, st.Proto().String(), "tenant=")
					require.NotContains(t, st.Proto().String(), "hunter2")
				}
			})
		}
	}
}

// A converter whose message comes from the matched error must not have it
// cached for the next error wrapping the same sentinel.
func TestServerInterceptor_TypeMappingIsNotCached(t *testing.T) {
	t.Parallel()
	si := ServerInterceptor(
		WithErrorTypeMapping(codes.FailedPrecondition, func(e *tenantError) string { return "tenant " + e.tenant }),
	).(*interceptor)

	for _, tenant := range []string{"A", "B"} {
		_, err := callUnary(t, si, fmt.Errorf("%w: %w", errScrubDB, &tenantError{tenant: tenant}))
		require.Equal(t, "tenant "+tenant, status.Convert(err).Message())
	}
}

// A converter's client-facing status loses only its DebugInfo.
func TestServerInterceptor_ConverterClientStatusDropsDebugInfo(t *testing.T) {
	t.Parallel()
	si := ServerInterceptor(WithErrorConverters(ErrorConverter{
		Matcher: func(_ context.Context, err error) bool { return errors.Is(err, errScrubDB) },
		Convert: func(context.Context, error) *status.Status {
			return statusWith(t, codes.NotFound, "row not found", debugInfo(), &errdetails.ErrorInfo{Reason: "ROW"})
		},
	})).(*interceptor)

	for range 2 { // the second call is served from the cache
		_, err := callUnary(t, si, errScrubDB)

		require.Equal(t, "row not found", status.Convert(err).Message())
		require.Equal(t, []string{"google.rpc.ErrorInfo"}, detailNames(t, err))
	}
}

// Conversion must cope with errors and mapping targets held by value, which
// have no address.
func TestServerInterceptor_ValueErrors(t *testing.T) {
	t.Parallel()
	sentinel := valueError{msg: "value sentinel"}
	si := ServerInterceptor(WithErrorMapping(sentinel, codes.NotFound, "missing")).(*interceptor)

	for range 2 {
		code, _ := callUnary(t, si, sentinel)
		require.Equal(t, codes.NotFound, code)
	}
	code, _ := callUnary(t, si, valueError{msg: "other"})
	require.Equal(t, codes.Internal, code)
}
