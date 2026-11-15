// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package health_test

import (
	"errors"
	"regexp"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/health"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestIgnoredMethodsBypassHealthCheck(t *testing.T) {
	t.Parallel()

	unhealthy := fakeHealth{err: errors.New("db down")}

	tests := []struct {
		name    string
		opts    []health.Option
		method  string
		bypass  bool
		wantErr codes.Code
	}{
		{name: "exact ignored method", opts: []health.Option{health.WithIgnoreMethods("/svc.A/Ping")}, method: "/svc.A/Ping", bypass: true},
		{name: "pattern match", opts: []health.Option{health.WithIgnorePatterns(regexp.MustCompile(`^/svc\.a/`))}, method: "/svc.A/Get", bypass: true},
		{name: "not ignored", opts: []health.Option{health.WithIgnoreMethods("/svc.A/Ping")}, method: "/svc.A/Get", wantErr: codes.Unavailable},
		{name: "empty options keep checking", opts: []health.Option{health.WithIgnoreMethods(), health.WithIgnorePatterns()}, method: "/svc.A/Get", wantErr: codes.Unavailable},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			unary := health.ServerUnaryInterceptor(unhealthy, tc.opts...)
			resp, err := unary(t.Context(), "req", &grpc.UnaryServerInfo{FullMethod: tc.method}, okHandler)

			stream := health.ServerStreamInterceptor(unhealthy, tc.opts...)
			serr := stream(nil, &fakeServerStream{ctx: t.Context()}, &grpc.StreamServerInfo{FullMethod: tc.method}, okStreamHandler)

			if tc.bypass {
				require.NoError(t, err)
				require.Equal(t, "ok", resp)
				require.NoError(t, serr)
				return
			}
			require.Equal(t, tc.wantErr, status.Code(err))
			require.Equal(t, tc.wantErr, status.Code(serr))
			require.ErrorIs(t, err, health.ErrServiceUnavailable)
		})
	}
}
