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

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var errSecret = errors.New("db row 42 locked by tenant acme")

func TestErrorRules(t *testing.T) {
	t.Parallel()

	wrapped := fmt.Errorf("load: %w", errSecret)
	tests := []struct {
		name       string
		rule       ErrorRule
		wantMsg    string
		wantReason string
	}{
		{"hidden_by_default", ErrorRule{Target: errSecret, Code: codes.Aborted}, "Aborted", "ABORTED"},
		{"explicit_message", ErrorRule{Target: errSecret, Code: codes.Aborted, Message: "try again", Expose: true}, "try again", "ABORTED"},
		{"exposed", ErrorRule{Target: errSecret, Code: codes.Aborted, Expose: true}, wrapped.Error(), "ABORTED"},
		{"custom_reason", ErrorRule{Target: errSecret, Code: codes.Aborted, Reason: "ROW_LOCKED"}, "Aborted", "ROW_LOCKED"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			o := newOptions(WithErrorRules(tc.rule))
			require.Len(t, o.errorConverters, 1)
			c := o.errorConverters[0]
			require.True(t, c.Matcher(t.Context(), wrapped))
			require.False(t, c.Matcher(t.Context(), errors.New("other")))

			st := c.Convert(t.Context(), wrapped)
			require.Equal(t, codes.Aborted, st.Code())
			require.Equal(t, tc.wantMsg, st.Message())

			// The finalizer keeps a rule's reason and fills in the domain.
			final := DefaultFinalizerWithDomain("svc.example.com")(t.Context(), st.Err())
			ei := requireErrorInfo(t, final)
			require.Equal(t, tc.wantReason, ei.GetReason())
			require.Equal(t, "svc.example.com", ei.GetDomain())
		})
	}
}

func TestErrorRulesOrder(t *testing.T) {
	t.Parallel()

	o := newOptions(WithErrorRules(
		ErrorRule{Target: errSecret, Code: codes.NotFound},
		ErrorRule{Target: errSecret, Code: codes.Internal},
	))
	require.Len(t, o.errorConverters, 2)
	st, ok := status.FromError(o.errorConverters[0].Convert(t.Context(), errSecret).Err())
	require.True(t, ok)
	require.Equal(t, codes.NotFound, st.Code(), "each rule keeps its own target and code")
	require.Equal(t, codes.Internal, o.errorConverters[1].Convert(t.Context(), errSecret).Code())
}

// TestExposedRuleNotCachedAcrossErrors pins that an exposed error text is
// never served from the sentinel cache to another error wrapping the same
// sentinel.
func TestExposedRuleNotCachedAcrossErrors(t *testing.T) {
	t.Parallel()

	for name, opt := range map[string]Option{
		"rule":    WithErrorRules(ErrorRule{Target: errSecret, Code: codes.Aborted, Expose: true}),
		"mapping": WithErrorMapping(errSecret, codes.Aborted, ""),
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			unary := ServerInterceptor(opt, WithCacheOnlySentinel(), WithSentinelErrors(errSecret)).ServerUnaryInterceptor()
			call := func(err error) string {
				_, got := unary(t.Context(), nil, &grpc.UnaryServerInfo{FullMethod: "/s/M"},
					func(context.Context, any) (any, error) { return nil, err })
				st, ok := status.FromError(got)
				require.True(t, ok)
				return st.Message()
			}
			a := fmt.Errorf("tenant A: %w", errSecret)
			b := fmt.Errorf("tenant B: %w", errSecret)
			require.Equal(t, a.Error(), call(a))
			require.Equal(t, b.Error(), call(b), "tenant A's text must not leak to tenant B")
		})
	}
}

type tenantKey struct{}

// TestNoCacheConverterNotPreemptedByCache pins that a cached status of a
// later converter never answers an error a NoCache converter matches.
func TestNoCacheConverterNotPreemptedByCache(t *testing.T) {
	t.Parallel()

	unary := ServerInterceptor(
		WithErrorConverters(ErrorConverter{
			Matcher: func(ctx context.Context, err error) bool {
				return ctx.Value(tenantKey{}) == "vip" && errors.Is(err, errSecret)
			},
			Convert: func(context.Context, error) *status.Status { return status.New(codes.Unavailable, "vip") },
			NoCache: true,
		}),
		WithErrorRules(ErrorRule{Target: errSecret, Code: codes.Aborted}),
	).ServerUnaryInterceptor()
	call := func(ctx context.Context) codes.Code {
		_, got := unary(ctx, nil, &grpc.UnaryServerInfo{FullMethod: "/s/M"},
			func(context.Context, any) (any, error) { return nil, errSecret })
		return status.Code(got)
	}
	require.Equal(t, codes.Aborted, call(t.Context()))
	require.Equal(t, codes.Unavailable, call(context.WithValue(t.Context(), tenantKey{}, "vip")))
}
