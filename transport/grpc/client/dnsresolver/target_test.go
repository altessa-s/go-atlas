// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package dnsresolver

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// The cases mirror the target parsing tests of grpc-go's dns resolver
// (internal/resolver/dns), so both resolvers accept the same targets.
func TestParseTarget(t *testing.T) {
	t.Parallel()

	tests := []struct {
		target, host, port string
		wantErr            error
	}{
		{target: "www.example.com", host: "www.example.com", port: "443"},
		{target: "orders.default.svc:9000", host: "orders.default.svc", port: "9000"},
		{target: "inventory:http", host: "inventory", port: "http"},
		{target: "127.0.0.1", host: "127.0.0.1", port: "443"},
		{target: "127.0.0.1:12345", host: "127.0.0.1", port: "12345"},
		{target: "::", host: "::", port: "443"},
		{target: "::1", host: "::1", port: "443"},
		{target: "[::1]", host: "::1", port: "443"},
		{target: "[::1]:80", host: "::1", port: "80"},
		{target: "2001:db8:85a3::8a2e:370:7334", host: "2001:db8:85a3::8a2e:370:7334", port: "443"},
		{target: "[2001:db8:a0b:12f0::1]:21", host: "2001:db8:a0b:12f0::1", port: "21"},
		{target: "[2001:db8::1]:http", host: "2001:db8::1", port: "http"},
		{target: "fe80::1%eth0", host: "fe80::1%eth0", port: "443"},
		{target: "[fe80::1%lo0]:80", host: "fe80::1%lo0", port: "80"},
		{target: ":80", host: "localhost", port: "80"},
		{target: "", wantErr: errMissingAddress},
		{target: "[]", wantErr: errMissingAddress},
		{target: ":", wantErr: errMissingPort},
		{target: "orders:", wantErr: errMissingPort},
		{target: "[2001:db8::1]:", wantErr: errMissingPort},
		{target: "[2001:db8:a0b:12f0::1"},
		{target: "a:b:c"},
		{target: "[::1]x:80"},
	}
	for _, tt := range tests {
		t.Run(tt.target, func(t *testing.T) {
			t.Parallel()

			host, port, err := parseTarget(tt.target, defaultPort)
			if tt.host == "" {
				require.Error(t, err)
				if tt.wantErr != nil {
					require.ErrorIs(t, err, tt.wantErr)
				}
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.host, host)
			require.Equal(t, tt.port, port)
		})
	}
}

// The cases mirror grpc-go's custom authority tests: dns://server/host:port
// sends lookups to server, port 53 unless it names one.
func TestDNSServer(t *testing.T) {
	t.Parallel()

	tests := []struct {
		authority, want string
		wantErr         bool
	}{
		{authority: "", want: ""},
		{authority: "4.3.2.1:53", want: "4.3.2.1:53"},
		{authority: "4.3.2.1:123", want: "4.3.2.1:123"},
		{authority: "4.3.2.1", want: "4.3.2.1:53"},
		{authority: "[::1]", want: "[::1]:53"},
		{authority: "[::1]:123", want: "[::1]:123"},
		{authority: "dns.example.com", want: "dns.example.com:53"},
		{authority: ":123", want: "localhost:123"},
		{authority: ":", wantErr: true},
		{authority: "[::1]:", wantErr: true},
		{authority: "dns.example.com:", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.authority, func(t *testing.T) {
			t.Parallel()

			got, err := dnsServer(tt.authority)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}
