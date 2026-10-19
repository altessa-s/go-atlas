// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package idempotency

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/idempotency"
	"github.com/altessa-s/go-atlas/data/idempotency/storages/memory"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors"

	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/wrapperspb"

	sharedmetadata "github.com/altessa-s/go-atlas/transport/grpc/interceptors/metadata"
	grpcmetadata "google.golang.org/grpc/metadata"
)

const testKey = "0b8a4f7e-6c1d-4e2a-9b3c-5d6e7f8a9b0c"

type ownerCtxKey struct{}

// check runs one request through a fresh per-request driver of ic and, on a
// held lock, completes it as a successful call would.
func check(t *testing.T, ic *interceptor, ctx context.Context, req any) error {
	t.Helper()
	ctx = grpcmetadata.NewIncomingContext(ctx, grpcmetadata.Pairs(DefaultIdempotencyKeyHeader, testKey))
	ri := &requestInterceptor{interceptor: ic, meta: &sharedmetadata.CallMetadata{FullyMethodName: "/svc.v1.Orders/Create"}}
	if err := ri.checkIdempotency(ctx, req); err != nil {
		return err
	}
	return ri.PostCall(ctx, nil, nil)
}

func newTestInterceptor(k idempotency.Idempotency, opt ...Option) *interceptor {
	o := newOptions(opt...)
	o.statusCreator = defaultStatusCreator
	return &interceptor{
		BaseInterceptor: interceptors.NewBaseInterceptorWithFilter(interceptorName, nil, nil, nil),
		i:               k,
		opts:            o,
	}
}

func requireScenario(t *testing.T, err error, code codes.Code, reason string, msgAndArgs ...any) {
	t.Helper()
	st, ok := status.FromError(err)
	require.True(t, ok, "%v", err)
	require.Equal(t, code, st.Code(), msgAndArgs...)
	for _, d := range st.Details() {
		if ei, ok := d.(*errdetails.ErrorInfo); ok {
			require.Equal(t, reason, ei.GetReason(), msgAndArgs...)
			return
		}
	}
	t.Fatalf("no ErrorInfo in %v", st)
}

func TestRequestFingerprint(t *testing.T) {
	t.Parallel()

	ic := newTestInterceptor(idempotency.New(memory.New(), idempotency.WithMaxLockDuration(time.Minute)))
	require.NoError(t, check(t, ic, t.Context(), wrapperspb.String("pay 10")))

	requireScenario(t, check(t, ic, t.Context(), wrapperspb.String("pay 10")), codes.FailedPrecondition, ReasonIDKAlreadyUsed)
	requireScenario(t, check(t, ic, t.Context(), wrapperspb.String("pay 99")), codes.InvalidArgument, ReasonIDKMismatch)
}

func TestRequestFingerprintDisabled(t *testing.T) {
	t.Parallel()

	ic := newTestInterceptor(idempotency.New(memory.New(), idempotency.WithMaxLockDuration(time.Minute)), WithRequestFingerprint(false))
	require.NoError(t, check(t, ic, t.Context(), wrapperspb.String("pay 10")))
	requireScenario(t, check(t, ic, t.Context(), wrapperspb.String("pay 99")), codes.FailedPrecondition, ReasonIDKAlreadyUsed)
}

func TestOwnerForbidden(t *testing.T) {
	t.Parallel()

	k := idempotency.New(memory.New(),
		idempotency.WithMaxLockDuration(time.Minute),
		idempotency.WithOwner(func(ctx context.Context) string { s, _ := ctx.Value(ownerCtxKey{}).(string); return s }),
	)
	ic := newTestInterceptor(k)
	alice := context.WithValue(t.Context(), ownerCtxKey{}, "alice")
	bob := context.WithValue(t.Context(), ownerCtxKey{}, "bob")
	require.NoError(t, check(t, ic, alice, wrapperspb.String("pay 10")))
	requireScenario(t, check(t, ic, bob, wrapperspb.String("pay 10")), codes.PermissionDenied, ReasonIDKForbidden)
}

func TestRequestFingerprintValue(t *testing.T) {
	t.Parallel()

	ri := &requestInterceptor{interceptor: newTestInterceptor(nil), meta: &sharedmetadata.CallMetadata{FullyMethodName: "/a/B"}}
	fp := ri.requestFingerprint(wrapperspb.String("x"))
	require.Len(t, fp, 32)
	require.Equal(t, fp, ri.requestFingerprint(wrapperspb.String("x")))
	require.Nil(t, ri.requestFingerprint(struct{}{}), "not a protobuf message")

	other := &requestInterceptor{interceptor: ri.interceptor, meta: &sharedmetadata.CallMetadata{FullyMethodName: "/a/C"}}
	require.NotEqual(t, fp, other.requestFingerprint(wrapperspb.String("x")), "the method is part of the fingerprint")
}

func TestReplayAuthorizerStatusPassesThrough(t *testing.T) {
	t.Parallel()

	k := idempotency.New(memory.New(),
		idempotency.WithMaxLockDuration(time.Minute),
		idempotency.WithOwner(func(ctx context.Context) string { s, _ := ctx.Value(ownerCtxKey{}).(string); return s }),
		idempotency.WithReplayAuthorizer(func(ctx context.Context, owner string) error {
			if ctx.Value(ownerCtxKey{}) == owner {
				return nil
			}
			return status.Error(codes.NotFound, "payment not found")
		}),
	)
	ic := newTestInterceptor(k)
	require.NoError(t, check(t, ic, context.WithValue(t.Context(), ownerCtxKey{}, "alice"), wrapperspb.String("pay 10")))

	err := check(t, ic, context.WithValue(t.Context(), ownerCtxKey{}, "bob"), wrapperspb.String("pay 10"))
	st, ok := status.FromError(err)
	require.True(t, ok)
	require.Equal(t, codes.NotFound, st.Code())
	require.Equal(t, "payment not found", st.Message(), "the service's own status, nothing of the record")
}
