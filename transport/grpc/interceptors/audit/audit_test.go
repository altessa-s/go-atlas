// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package audit_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/audit"
	"github.com/altessa-s/go-atlas/audit/audittest"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/requestid"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	auditgrpc "github.com/altessa-s/go-atlas/transport/grpc/interceptors/audit"
)

var _ auditgrpc.EventEmitter = (*audit.Auditor)(nil)

type fakeEmitter struct {
	event *audit.Event
	ok    bool
}

func (f *fakeEmitter) Emit(event *audit.Event) bool {
	f.event = event
	return f.ok
}

func TestEventEmitterSeam(t *testing.T) {
	t.Parallel()
	emitter := &fakeEmitter{}
	interceptor := auditgrpc.ServerInterceptor(emitter).ServerUnaryInterceptor()
	info := &grpc.UnaryServerInfo{FullMethod: "/example.v1.Service/Get"}
	resp, err := interceptor(t.Context(), nil, info, func(context.Context, any) (any, error) { return "ok", nil })
	require.NoError(t, err)
	require.Equal(t, "ok", resp)
	require.NotNil(t, emitter.event)
	require.Equal(t, audit.EventTypeAPIRequest, emitter.event.Type)
}

func TestServerInterceptor_AuditsCall(t *testing.T) {
	t.Parallel()
	a, store, shutdown := audittest.NewTestAuditor(t)

	i := auditgrpc.ServerInterceptor(a)
	interceptor := i.ServerUnaryInterceptor()

	info := &grpc.UnaryServerInfo{FullMethod: "/myservice.v1.MyService/GetItem"}
	handler := func(ctx context.Context, req any) (any, error) {
		return "ok", nil
	}

	resp, err := interceptor(t.Context(), nil, info, handler)
	require.NoError(t, err)
	assert.Equal(t, "ok", resp)

	require.NoError(t, shutdown(t.Context()))
	require.Equal(t, 1, store.Len())

	event := store.Events()[0]
	assert.Equal(t, audit.EventTypeAPIRequest, event.Type)
	assert.Equal(t, audit.ActionExecute, event.Action)
	assert.Equal(t, "myservice.v1.MyService", event.Resource.Type)
	assert.Equal(t, "/myservice.v1.MyService/GetItem", event.Resource.Path)
	assert.Equal(t, audit.ResultStatusSuccess, event.Result.Status)
}

func TestServerInterceptor_IgnoreMethods(t *testing.T) {
	t.Parallel()
	a, store, shutdown := audittest.NewTestAuditor(t)

	i := auditgrpc.ServerInterceptor(a,
		auditgrpc.WithIgnoreMethods("/grpc.health.v1.Health/Check"),
	)
	interceptor := i.ServerUnaryInterceptor()

	handler := func(ctx context.Context, req any) (any, error) {
		return "ok", nil
	}

	// Ignored method
	info := &grpc.UnaryServerInfo{FullMethod: "/grpc.health.v1.Health/Check"}
	_, err := interceptor(t.Context(), nil, info, handler)
	require.NoError(t, err)

	// Non-ignored method
	info = &grpc.UnaryServerInfo{FullMethod: "/myservice.v1.MyService/GetItem"}
	_, err = interceptor(t.Context(), nil, info, handler)
	require.NoError(t, err)

	require.NoError(t, shutdown(t.Context()))
	assert.Equal(t, 1, store.Len())
}

func TestServerInterceptor_NilAuditor(t *testing.T) {
	t.Parallel()
	i := auditgrpc.ServerInterceptor(nil)
	interceptor := i.ServerUnaryInterceptor()

	called := false
	handler := func(ctx context.Context, req any) (any, error) {
		called = true
		return "ok", nil
	}

	info := &grpc.UnaryServerInfo{FullMethod: "/test/Method"}
	resp, err := interceptor(t.Context(), nil, info, handler)
	require.NoError(t, err)
	assert.True(t, called)
	assert.Equal(t, "ok", resp)
}

func TestServerInterceptor_GRPCStatusMapping(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		err  error
		want audit.ResultStatus
		code int
	}{
		{"success", nil, audit.ResultStatusSuccess, 0},
		{"not_found", status.Error(codes.NotFound, "not found"), audit.ResultStatusError, int(codes.NotFound)},
		{"permission_denied", status.Error(codes.PermissionDenied, "denied"), audit.ResultStatusDenied, int(codes.PermissionDenied)},
		{"unauthenticated", status.Error(codes.Unauthenticated, "unauth"), audit.ResultStatusDenied, int(codes.Unauthenticated)},
		{"internal", status.Error(codes.Internal, "oops"), audit.ResultStatusError, int(codes.Internal)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			a, store, shutdown := audittest.NewTestAuditor(t)

			i := auditgrpc.ServerInterceptor(a)
			interceptor := i.ServerUnaryInterceptor()

			info := &grpc.UnaryServerInfo{FullMethod: "/test/Method"}
			handler := func(ctx context.Context, req any) (any, error) {
				return nil, tt.err
			}

			_, _ = interceptor(t.Context(), nil, info, handler)

			require.NoError(t, shutdown(t.Context()))
			require.Equal(t, 1, store.Len())

			event := store.Events()[0]
			assert.Equal(t, tt.want, event.Result.Status)
			assert.Equal(t, tt.code, event.Result.Code)
		})
	}
}

func TestServerInterceptor_ActorExtractor(t *testing.T) {
	t.Parallel()
	a, store, shutdown := audittest.NewTestAuditor(t)

	i := auditgrpc.ServerInterceptor(a,
		auditgrpc.WithActorExtractor(func(ctx context.Context) audit.Actor {
			return audit.Actor{Type: audit.ActorTypeUser, ID: "user-42"}
		}),
	)
	interceptor := i.ServerUnaryInterceptor()

	info := &grpc.UnaryServerInfo{FullMethod: "/test/Method"}
	handler := func(ctx context.Context, req any) (any, error) { return nil, nil }

	_, _ = interceptor(t.Context(), nil, info, handler)

	require.NoError(t, shutdown(t.Context()))
	require.Equal(t, 1, store.Len())
	assert.Equal(t, "user-42", store.Events()[0].Actor.ID)
	assert.Equal(t, audit.ActorTypeUser, store.Events()[0].Actor.Type)
}

func TestServerInterceptor_RequestIDFromContext(t *testing.T) {
	t.Parallel()
	a, store, shutdown := audittest.NewTestAuditor(t)

	i := auditgrpc.ServerInterceptor(a)
	interceptor := i.ServerUnaryInterceptor()

	ctx := requestid.NewContext(t.Context(), "req-abc-123")

	info := &grpc.UnaryServerInfo{FullMethod: "/test/Method"}
	handler := func(ctx context.Context, req any) (any, error) { return nil, nil }

	_, _ = interceptor(ctx, nil, info, handler)

	require.NoError(t, shutdown(t.Context()))
	require.Equal(t, 1, store.Len())
	assert.Equal(t, "req-abc-123", store.Events()[0].Context.RequestID)
}
