// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"context"
	"net/netip"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas-net/clientip"
	"github.com/altessa-s/go-atlas/transport/policy/geoacl"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	grpcconfig "github.com/altessa-s/go-atlas/config/grpc"
	middlewareconfig "github.com/altessa-s/go-atlas/config/middleware"
)

// unplacedGeo answers like a GeoIP database asked about a private address: no
// location, no error.
type unplacedGeo struct{}

func (unplacedGeo) Resolve(context.Context, netip.Addr) (geoacl.GeoInfo, error) {
	return geoacl.GeoInfo{}, nil
}

// geoACL.allowPrivateAddresses reaches the interceptor: without it an allow
// list turns an in-cluster caller away.
func TestGeoAclInterceptor_AllowPrivateAddressesFromConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		allow bool
		ip    string
		want  codes.Code
	}{
		{"off, private client", false, "10.4.2.9", codes.PermissionDenied},
		{"on, private client", true, "10.4.2.9", codes.OK},
		{"on, loopback client", true, "127.0.0.1", codes.OK},
		{"on, public client", true, "8.8.8.8", codes.PermissionDenied},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			cfg := grpcconfig.DefaultGeoACLInterceptor()
			cfg.Enabled = true
			cfg.DefaultRule = &middlewareconfig.GeoACLRule{AllowCountries: []string{"DE"}}
			cfg.AllowPrivateAddresses = tc.allow

			b := New(&grpcconfig.Config{Interceptors: &grpcconfig.Interceptors{GeoAcl: &cfg}}).
				UseGeoResolver(unplacedGeo{}).
				WithGeoAclInterceptor()
			require.Empty(t, b.errs)
			require.Len(t, b.interceptors, 1)

			ctx := clientip.NewContext(t.Context(), netip.MustParseAddr(tc.ip))
			_, err := b.interceptors[0].ServerUnaryInterceptor()(ctx, nil,
				&grpc.UnaryServerInfo{FullMethod: "/orders.OrderService/List"},
				func(context.Context, any) (any, error) { return "ok", nil })
			require.Equal(t, tc.want, status.Code(err))
		})
	}
}
