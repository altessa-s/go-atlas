// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package geoacl

import (
	"net/netip"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas-net/clientip"
	"github.com/altessa-s/go-atlas/transport/policy/geoacl"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const privateAddressTestMethod = "/orders.OrderService/List"

// call runs the unary interceptor for a client the resolver cannot place, the
// way a GeoIP database answers a loopback or private address: no location, no
// error.
func call(t *testing.T, reg *geoacl.Registry, ip string, opts ...Option) codes.Code {
	t.Helper()
	inter := ServerInterceptor(&mockResolver{}, reg, opts...)
	ctx := clientip.NewContext(t.Context(), netip.MustParseAddr(ip))
	_, err := inter.ServerUnaryInterceptor()(ctx, nil, &grpc.UnaryServerInfo{FullMethod: privateAddressTestMethod}, noopHandler)
	return status.Code(err)
}

// A sanctions deny list under PolicyAllow used to turn away every caller the
// resolver could not place.
func TestInterceptor_DenyListLetsUnplacedInternalCallersThrough(t *testing.T) {
	t.Parallel()

	reg := geoacl.NewRegistry(geoacl.PolicyAllow)
	reg.SetDefault(&geoacl.AccessRule{DenyCountries: []string{"KP", "IR"}})

	for _, ip := range []string{"127.0.0.1", "10.4.2.9", "::1"} {
		t.Run(ip, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, codes.OK, call(t, reg, ip))
		})
	}
}

// An allow list cannot be satisfied by an unplaced address. Internal callers
// get through only when the operator says so.
func TestInterceptor_AllowPrivateAddresses(t *testing.T) {
	t.Parallel()

	reg := geoacl.NewRegistry(geoacl.PolicyDeny)
	reg.SetDefault(&geoacl.AccessRule{AllowCountries: []string{"DE"}})

	tests := []struct {
		name string
		ip   string
		opts []Option
		want codes.Code
	}{
		{"loopback, option off", "127.0.0.1", nil, codes.PermissionDenied},
		{"private, option off", "10.4.2.9", nil, codes.PermissionDenied},
		{"loopback, option on", "127.0.0.1", []Option{WithAllowPrivateAddresses()}, codes.OK},
		{"private, option on", "10.4.2.9", []Option{WithAllowPrivateAddresses()}, codes.OK},
		{"IPv4-mapped private, option on", "::ffff:192.168.1.5", []Option{WithAllowPrivateAddresses()}, codes.OK},
		{"public address, option on", "8.8.8.8", []Option{WithAllowPrivateAddresses()}, codes.PermissionDenied},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, call(t, reg, tc.ip, tc.opts...))
		})
	}
}
