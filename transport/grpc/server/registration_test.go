// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package grpc

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/observability/health"

	"google.golang.org/grpc"
	"google.golang.org/grpc/channelz/service"

	healthhandler "github.com/altessa-s/go-atlas/transport/grpc/handlers/health"
	channelzpb "google.golang.org/grpc/channelz/grpc_channelz_v1"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	reflectionv1 "google.golang.org/grpc/reflection/grpc_reflection_v1"
	reflectionv1alpha "google.golang.org/grpc/reflection/grpc_reflection_v1alpha"
)

type registerFunc func(grpc.ServiceRegistrar)

func (f registerFunc) Register(r grpc.ServiceRegistrar, _ <-chan struct{}) { f(r) }

func TestServerRejectsInvalidServiceRegistration(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name       string
		reflection bool
		setup      func(*testing.T, *Server)
	}{
		{
			name: "duplicate in one handler",
			setup: func(_ *testing.T, srv *Server) {
				h := testService("test.Service", nil)
				srv.RegisterHandlers(registerFunc(func(r grpc.ServiceRegistrar) {
					h(r)
					h(r)
				}))
			},
		},
		{
			name: "duplicate across handler calls",
			setup: func(_ *testing.T, srv *Server) {
				srv.RegisterHandlers(testService("test.Service", nil))
				srv.RegisterHandlers(testService("test.Service", nil))
			},
		},
		{
			name: "duplicate built-in health",
			setup: func(t *testing.T, srv *Server) {
				coord := health.New()
				t.Cleanup(coord.Close)
				srv.RegisterHandlers(healthhandler.New(coord), testService(healthpb.Health_ServiceDesc.ServiceName, nil))
			},
		},
		{
			name:       "duplicate built-in reflection v1",
			reflection: true,
			setup: func(_ *testing.T, srv *Server) {
				srv.RegisterHandlers(testService(reflectionv1.ServerReflection_ServiceDesc.ServiceName, nil))
			},
		},
		{
			name:       "duplicate built-in reflection v1alpha",
			reflection: true,
			setup: func(_ *testing.T, srv *Server) {
				srv.RegisterHandlers(testService(reflectionv1alpha.ServerReflection_ServiceDesc.ServiceName, nil))
			},
		},
		{
			name: "implementation mismatch",
			setup: func(_ *testing.T, srv *Server) {
				srv.RegisterHandlers(testService("test.Service", func(desc *grpc.ServiceDesc) {
					desc.HandlerType = (*healthpb.HealthServer)(nil)
				}))
			},
		},
		{
			name: "malformed HandlerType",
			setup: func(_ *testing.T, srv *Server) {
				srv.RegisterHandlers(testService("test.Service", func(desc *grpc.ServiceDesc) {
					desc.HandlerType = (*struct{})(nil)
				}))
			},
		},
		{
			name: "nil descriptor",
			setup: func(_ *testing.T, srv *Server) {
				srv.RegisterHandlers(registerFunc(func(r grpc.ServiceRegistrar) { r.RegisterService(nil, nil) }))
			},
		},
		{
			name: "nil method handler",
			setup: func(_ *testing.T, srv *Server) {
				srv.RegisterHandlers(testService("test.Service", func(desc *grpc.ServiceDesc) {
					desc.Methods[0].Handler = nil
				}))
			},
		},
		{
			name: "nil stream handler",
			setup: func(_ *testing.T, srv *Server) {
				srv.RegisterHandlers(testService("test.Service", func(desc *grpc.ServiceDesc) {
					desc.Streams[0].Handler = nil
				}))
			},
		},
		{
			name: "method and stream name collision",
			setup: func(_ *testing.T, srv *Server) {
				srv.RegisterHandlers(testService("test.Service", func(desc *grpc.ServiceDesc) {
					desc.Streams[0].StreamName = desc.Methods[0].MethodName
				}))
			},
		},
		{
			name: "stream without direction",
			setup: func(_ *testing.T, srv *Server) {
				srv.RegisterHandlers(testService("test.Service", func(desc *grpc.ServiceDesc) {
					desc.Streams[0].ServerStreams = false
				}))
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			opts := []Option{WithStartupVerification(0)}
			if tc.reflection {
				opts = append(opts, WithReflection())
			}
			srv, err := New(opts...)
			require.NoError(t, err)
			tc.setup(t, srv)

			require.ErrorIs(t, srv.Start(), ErrInvalidServiceRegistration)
			require.False(t, srv.IsStarted())
			require.Nil(t, srv.Listener())
		})
	}
}

func testService(name string, edit func(*grpc.ServiceDesc)) registerFunc {
	return func(r grpc.ServiceRegistrar) {
		desc := &grpc.ServiceDesc{
			ServiceName: name,
			HandlerType: (*any)(nil),
			Methods: []grpc.MethodDesc{{
				MethodName: "Call",
				Handler:    func(any, context.Context, func(any) error, grpc.UnaryServerInterceptor) (any, error) { return nil, nil },
			}},
			Streams: []grpc.StreamDesc{{
				StreamName:    "Watch",
				ServerStreams: true,
				Handler:       func(any, grpc.ServerStream) error { return nil },
			}},
		}
		if edit != nil {
			edit(desc)
		}
		r.RegisterService(desc, struct{}{})
	}
}

func TestServerInvalidRegistrationLeavesChannelzUnchanged(t *testing.T) {
	// Channelz tracks servers across the process, so keep this test serial.
	before := channelzServers(t)
	srv, err := New()
	require.NoError(t, err)
	srv.RegisterHandlers(testService("test.Service", func(desc *grpc.ServiceDesc) {
		desc.Methods[0].Handler = nil
	}))

	require.ErrorIs(t, srv.Start(), ErrInvalidServiceRegistration)
	require.Equal(t, before, channelzServers(t))
}

func channelzServers(t testing.TB) []int64 {
	t.Helper()
	var channelz channelzpb.ChannelzServer
	service.RegisterChannelzServiceToServer(testRegistrar(func(_ *grpc.ServiceDesc, impl any) {
		channelz = impl.(channelzpb.ChannelzServer)
	}))
	resp, err := channelz.GetServers(t.Context(), &channelzpb.GetServersRequest{})
	require.NoError(t, err)
	require.True(t, resp.GetEnd())
	ids := make([]int64, 0, len(resp.GetServer()))
	for _, srv := range resp.GetServer() {
		ids = append(ids, srv.GetRef().GetServerId())
	}
	return ids
}

type testRegistrar func(*grpc.ServiceDesc, any)

func (f testRegistrar) RegisterService(desc *grpc.ServiceDesc, impl any) { f(desc, impl) }
