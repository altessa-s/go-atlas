// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"context"
	"net"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/transport/clientip"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/auth"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"

	grpcconfig "github.com/altessa-s/go-atlas/config/grpc"
)

// White-box access exercises factory wiring without starting a network server.
func TestRealIPInterceptorTrust(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name           string
		legacy         bool
		peers, proxies []string
		want           string
	}{
		{name: "default", want: "10.0.0.1"},
		{name: "explicit peer", peers: []string{"10.0.0.0/8"}, want: "8.8.8.8"},
		{name: "explicit proxy", proxies: []string{"10.0.0.0/8"}, want: "8.8.8.8"},
		{name: "legacy opt out", legacy: true, want: "8.8.8.8"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cfg := grpcconfig.DefaultRealIPInterceptor()
			cfg.Enabled = true
			cfg.TrustPrivatePeers = tc.legacy
			cfg.TrustedPeers, cfg.TrustedProxies = tc.peers, tc.proxies
			b := New(&grpcconfig.Config{Interceptors: &grpcconfig.Interceptors{RealIp: &cfg}}).WithRealIPInterceptor()
			require.Empty(t, b.errs)
			require.Len(t, b.interceptors, 1)
			ctx := peer.NewContext(t.Context(), &peer.Peer{Addr: &net.TCPAddr{IP: net.ParseIP("10.0.0.1"), Port: 1234}})
			ctx = metadata.NewIncomingContext(ctx, metadata.Pairs("x-forwarded-for", "8.8.8.8"))
			got, err := b.interceptors[0].ServerUnaryInterceptor()(ctx, nil, &grpc.UnaryServerInfo{FullMethod: "/api.Service/Get"},
				func(ctx context.Context, _ any) (any, error) { return clientip.FromContext(ctx).String(), nil })
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestAuthInterceptorExemptions(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name              string
		methods, patterns []string
		want              codes.Code
	}{
		{name: "default", want: codes.Unauthenticated},
		{name: "explicit method", methods: []string{"/grpc.health.v1.Health/Check"}, want: codes.OK},
		{name: "explicit pattern", patterns: []string{`(?i)^/grpc[.]health[.]v1[.]Health/`}, want: codes.OK},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cfg := grpcconfig.DefaultAuthInterceptor()
			cfg.Enabled = true
			cfg.IgnoreMethods, cfg.IgnorePatterns = tc.methods, tc.patterns
			b := New(&grpcconfig.Config{Interceptors: &grpcconfig.Interceptors{Auth: &cfg}}).
				UseAuth(auth.AuthFunc(func(context.Context, auth.Request) (any, error) {
					return nil, status.Error(codes.Unauthenticated, "denied")
				}), nil).WithAuthInterceptor()
			require.Empty(t, b.errs)
			require.Len(t, b.interceptors, 1)
			called := false
			_, err := b.interceptors[0].ServerUnaryInterceptor()(t.Context(), nil,
				&grpc.UnaryServerInfo{FullMethod: "/grpc.health.v1.Health/Check"},
				func(context.Context, any) (any, error) { called = true; return nil, nil })
			require.Equal(t, tc.want, status.Code(err))
			require.Equal(t, tc.want == codes.OK, called)
		})
	}
}
