// Copyright 2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package idempotency_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"

	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/idempotency"
)

const (
	testOpID        = "op-42"
	testMethod      = "/svc.v1.Users/Update"
	testExplicitKey = "11111111-1111-4111-8111-111111111111"
	testCustomHdr   = "X-Idem-Key"
)

// invokerSpy records the outgoing metadata observed on the call and reports
// whether the invoker was reached.
type invokerSpy struct {
	called bool
	md     metadata.MD
}

// invoke returns a [grpc.UnaryInvoker] that writes into s.
func (s *invokerSpy) invoke() grpc.UnaryInvoker {
	return func(ctx context.Context, _ string, _, _ any, _ *grpc.ClientConn, _ ...grpc.CallOption) error {
		s.called = true
		s.md, _ = metadata.FromOutgoingContext(ctx)
		return nil
	}
}

func TestOperationFromContext(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		ctxFn  func(t *testing.T) context.Context
		wantID string
		wantOK bool
	}{
		{
			name:  "unset",
			ctxFn: func(t *testing.T) context.Context { return t.Context() },
		},
		{
			name: "empty string treated as unset",
			ctxFn: func(t *testing.T) context.Context {
				return idempotency.WithOperation(t.Context(), "")
			},
		},
		{
			name: "value present",
			ctxFn: func(t *testing.T) context.Context {
				return idempotency.WithOperation(t.Context(), testOpID)
			},
			wantID: testOpID,
			wantOK: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, ok := idempotency.OperationFromContext(tc.ctxFn(t))
			require.Equal(t, tc.wantOK, ok)
			require.Equal(t, tc.wantID, got)
		})
	}
}

func TestUnaryClientInterceptor(t *testing.T) {
	t.Parallel()

	// header named in cases below; "" means [idempotency.DefaultIdempotencyKeyHeader].
	type wantHeader struct {
		name   string // header to inspect; "" → DefaultIdempotencyKeyHeader.
		values []string
	}

	tests := []struct {
		name     string
		opts     []idempotency.ClientOption
		ctxFn    func(t *testing.T) context.Context
		method   string
		want     wantHeader
		wantAuth []string // "authorization" header expectation; nil → don't check.
	}{
		{
			name:   "seed present stamps deterministic key",
			ctxFn:  func(t *testing.T) context.Context { return idempotency.WithOperation(t.Context(), testOpID) },
			method: testMethod,
			want:   wantHeader{values: []string{idempotency.DeriveKey(testOpID, testMethod)}},
		},
		{
			name:   "no seed forwards without stamping",
			ctxFn:  func(t *testing.T) context.Context { return t.Context() },
			method: testMethod,
			want:   wantHeader{},
		},
		{
			name: "explicit key wins over seed",
			ctxFn: func(t *testing.T) context.Context {
				return idempotency.WithKey(idempotency.WithOperation(t.Context(), testOpID), testExplicitKey)
			},
			method: testMethod,
			want:   wantHeader{values: []string{testExplicitKey}},
		},
		{
			name: "existing metadata survives stamping",
			ctxFn: func(t *testing.T) context.Context {
				ctx := metadata.AppendToOutgoingContext(t.Context(), "authorization", "Bearer t")
				return idempotency.WithOperation(ctx, testOpID)
			},
			method:   testMethod,
			want:     wantHeader{values: []string{idempotency.DeriveKey(testOpID, testMethod)}},
			wantAuth: []string{"Bearer t"},
		},
		{
			name:   "custom header replaces default",
			opts:   []idempotency.ClientOption{idempotency.WithClientIdempotencyKeyHeader(testCustomHdr)},
			ctxFn:  func(t *testing.T) context.Context { return idempotency.WithOperation(t.Context(), testOpID) },
			method: testMethod,
			want:   wantHeader{name: testCustomHdr, values: []string{idempotency.DeriveKey(testOpID, testMethod)}},
		},
		{
			name: "custom seed extractor replaces default source",
			opts: []idempotency.ClientOption{
				idempotency.WithClientSeedExtractor(func(ctx context.Context) (string, bool) {
					v, ok := ctx.Value(externalSeedKey{}).(string)
					return v, ok && v != ""
				}),
			},
			ctxFn: func(t *testing.T) context.Context {
				return context.WithValue(t.Context(), externalSeedKey{}, "external-op-99")
			},
			method: testMethod,
			want:   wantHeader{values: []string{idempotency.DeriveKey("external-op-99", testMethod)}},
		},
		{
			name: "method filter skips matching method",
			opts: []idempotency.ClientOption{
				idempotency.WithClientMethodFilter(func(method string) bool {
					return method != "/grpc.health.v1.Health/Check"
				}),
			},
			ctxFn:  func(t *testing.T) context.Context { return idempotency.WithOperation(t.Context(), testOpID) },
			method: "/grpc.health.v1.Health/Check",
			want:   wantHeader{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var spy invokerSpy
			interceptor := idempotency.UnaryClientInterceptor(tc.opts...)

			err := interceptor(tc.ctxFn(t), tc.method, nil, nil, nil, spy.invoke())
			require.NoError(t, err)
			require.True(t, spy.called, "invoker must be reached")

			header := tc.want.name
			if header == "" {
				header = idempotency.DefaultIdempotencyKeyHeader
			}
			require.Equal(t, tc.want.values, spy.md.Get(header))

			if header != idempotency.DefaultIdempotencyKeyHeader {
				require.Empty(t, spy.md.Get(idempotency.DefaultIdempotencyKeyHeader),
					"default header must remain untouched when a custom one is configured")
			}
			if tc.wantAuth != nil {
				require.Equal(t, tc.wantAuth, spy.md.Get("authorization"))
			}
		})
	}
}

// externalSeedKey is a context.Value key used by the custom-extractor test
// case to simulate a project that carries operation IDs under its own key.
type externalSeedKey struct{}

func TestUnaryClientInterceptor_PropagatesInvokerError(t *testing.T) {
	t.Parallel()

	want := errors.New("downstream boom")
	interceptor := idempotency.UnaryClientInterceptor()
	ctx := idempotency.WithOperation(t.Context(), testOpID)

	err := interceptor(ctx, testMethod, nil, nil, nil,
		func(context.Context, string, any, any, *grpc.ClientConn, ...grpc.CallOption) error {
			return want
		},
	)
	require.ErrorIs(t, err, want, "invoker error must propagate unchanged")
}
