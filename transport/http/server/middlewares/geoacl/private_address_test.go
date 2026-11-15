// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package geoacl

import (
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/transport/policy/geoacl"
)

// unknownLocationResolver answers like a GeoIP database asked about a
// loopback or private address: no location, no error.
func unknownLocationResolver() *mockResolver {
	return &mockResolver{}
}

func serve(t *testing.T, mw func(http.Handler) http.Handler, path string, ip netip.Addr) int {
	t.Helper()
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, requestWithIP(http.MethodGet, path, ip))
	return rec.Code
}

// A sanctions deny list under PolicyAllow used to turn away every caller the
// resolver could not place: kubelet probes from 127.0.0.1, in-cluster callers
// from 10.0.0.0/8.
func TestMiddleware_DenyListLetsUnplacedInternalCallersThrough(t *testing.T) {
	t.Parallel()

	reg := geoacl.NewRegistry(geoacl.PolicyAllow)
	reg.SetDefault(&geoacl.AccessRule{DenyCountries: []string{"KP", "IR"}})

	for _, ip := range []string{"127.0.0.1", "10.4.2.9", "::1", "169.254.10.1"} {
		t.Run(ip, func(t *testing.T) {
			t.Parallel()
			mw := Middleware(unknownLocationResolver(), reg)
			require.Equal(t, http.StatusOK, serve(t, mw, "/healthz", netip.MustParseAddr(ip)))
		})
	}
}

// An allow list cannot be satisfied by an unplaced address. Internal callers
// get through only when the operator says so, and only from an address that
// is loopback, private, link-local or otherwise reserved.
func TestMiddleware_AllowPrivateAddresses(t *testing.T) {
	t.Parallel()

	reg := geoacl.NewRegistry(geoacl.PolicyDeny)
	reg.SetDefault(&geoacl.AccessRule{AllowCountries: []string{"DE"}})

	tests := []struct {
		name string
		ip   string
		opts []Option
		want int
	}{
		{"loopback, option off", "127.0.0.1", nil, http.StatusForbidden},
		{"private, option off", "10.4.2.9", nil, http.StatusForbidden},
		{"loopback, option on", "127.0.0.1", []Option{WithAllowPrivateAddresses()}, http.StatusOK},
		{"private, option on", "10.4.2.9", []Option{WithAllowPrivateAddresses()}, http.StatusOK},
		{"IPv4-mapped private, option on", "::ffff:192.168.1.5", []Option{WithAllowPrivateAddresses()}, http.StatusOK},
		{"link-local, option on", "fe80::1", []Option{WithAllowPrivateAddresses()}, http.StatusOK},
		{"public address, option on", "8.8.8.8", []Option{WithAllowPrivateAddresses()}, http.StatusForbidden},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			mw := Middleware(unknownLocationResolver(), reg, tc.opts...)
			require.Equal(t, tc.want, serve(t, mw, "/api/orders", netip.MustParseAddr(tc.ip)))
		})
	}
}
