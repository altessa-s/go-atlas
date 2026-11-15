// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package errstatus

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var (
	errCacheQuota = errors.New("quota exhausted")
	// Two distinct sentinels with the same type and text, as two packages
	// that both declare errors.New("not found") have.
	errCacheNotFoundA = errors.New("not found")
	errCacheNotFoundB = errors.New("not found")
)

// cacheModes are the cache configurations every conversion must be the same under.
func cacheModes() map[string][]Option {
	return map[string][]Option{
		"default cache":  nil,
		"cache disabled": {WithCacheDisabled()},
		"cache size 0":   {WithCacheSize(0)},
	}
}

// A converter's status must be cached under the converter that matched, never
// under another sentinel of the error chain: an error joining a rule's target
// with context.Canceled must not turn every later context.Canceled into the
// rule's status.
func TestServerInterceptor_ConverterStatusIsNotCachedUnderAnotherSentinel(t *testing.T) {
	t.Parallel()
	for name, opts := range cacheModes() {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			si := ServerInterceptor(append(opts,
				WithErrorRules(ErrorRule{Target: errCacheQuota, Code: codes.ResourceExhausted}))...).(*interceptor)
			code := func(err error) codes.Code {
				c, _ := callUnary(t, si, err)
				return c
			}

			for range 2 {
				require.Equal(t, codes.ResourceExhausted, code(errors.Join(errCacheQuota, context.Canceled)))
				require.Equal(t, codes.Canceled, code(context.Canceled))
				require.Equal(t, codes.Canceled, code(fmt.Errorf("op: %w", context.Canceled)))
				require.Equal(t, codes.DeadlineExceeded, code(context.DeadlineExceeded))
				require.Equal(t, codes.ResourceExhausted, code(errCacheQuota))
			}
		})
	}
}

// Errors that share a type and a text but are different values must not share
// a conversion.
func TestServerInterceptor_SameTextErrorsAreConvertedSeparately(t *testing.T) {
	t.Parallel()
	for name, opts := range cacheModes() {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			si := ServerInterceptor(append(opts,
				WithErrorRules(ErrorRule{Target: errCacheNotFoundA, Code: codes.NotFound}))...).(*interceptor)

			for range 2 {
				mapped, _ := callUnary(t, si, errCacheNotFoundA)
				unmapped, _ := callUnary(t, si, errCacheNotFoundB)
				require.Equal(t, codes.NotFound, mapped)
				require.Equal(t, codes.Internal, unmapped)
			}
		})
	}
}

// An error that several converters match gets the first one every time,
// whichever of them was cached first.
func TestServerInterceptor_MultiTargetErrorUsesRegistrationOrder(t *testing.T) {
	t.Parallel()
	si := ServerInterceptor(WithErrorRules(
		ErrorRule{Target: errCacheNotFoundA, Code: codes.NotFound},
		ErrorRule{Target: errCacheQuota, Code: codes.ResourceExhausted},
	)).(*interceptor)
	code := func(err error) codes.Code {
		c, _ := callUnary(t, si, err)
		return c
	}

	require.Equal(t, codes.ResourceExhausted, code(errCacheQuota))
	require.Equal(t, codes.NotFound, code(errors.Join(errCacheQuota, errCacheNotFoundA)))
	require.Equal(t, codes.ResourceExhausted, code(errCacheQuota))
}

// A caller-supplied converter is converted on every call unless it declares
// itself static.
func TestServerInterceptor_OnlyStaticConvertersAreCached(t *testing.T) {
	t.Parallel()
	for _, static := range []bool{false, true} {
		t.Run(fmt.Sprintf("static=%t", static), func(t *testing.T) {
			t.Parallel()
			var converted atomic.Int32
			si := ServerInterceptor(WithErrorConverters(ErrorConverter{
				Static:  static,
				Matcher: func(_ context.Context, err error) bool { return errors.Is(err, errCacheQuota) },
				Convert: func(_ context.Context, err error) *status.Status {
					converted.Add(1)
					return status.New(codes.ResourceExhausted, err.Error())
				},
			})).(*interceptor)

			for _, tenant := range []string{"A", "B", "C"} {
				_, err := callUnary(t, si, fmt.Errorf("tenant %s: %w", tenant, errCacheQuota))
				if !static {
					require.Contains(t, status.Convert(err).Message(), "tenant "+tenant)
				}
			}

			want := int32(3)
			if static {
				want = 1
			}
			require.Equal(t, want, converted.Load())
		})
	}
}

// With debug logging off, converting an error must not allocate for log
// attributes or cache bookkeeping: only the returned error is allocated.
func TestServerInterceptor_ConversionAllocations(t *testing.T) { //nolint:paralleltest // AllocsPerRun forbids parallel tests
	si := ServerInterceptor(WithErrorRules(ErrorRule{Target: errCacheQuota, Code: codes.ResourceExhausted})).(*interceptor)
	ctx := t.Context()

	for name, err := range map[string]error{
		"static rule":   errCacheQuota,
		"built-in":      context.Canceled,
		"plain error":   errors.New("boom"),
		"direct status": status.Error(codes.NotFound, "missing"),
	} {
		allocs := testing.AllocsPerRun(100, func() {
			_ = si.handleServerError(ctx, nil, err)
		})
		require.LessOrEqual(t, allocs, 1.0, name)
	}
}
