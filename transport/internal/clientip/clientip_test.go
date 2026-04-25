// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package clientip

import (
	"net/netip"
	"testing"

	"github.com/stretchr/testify/require"
)

type mockHeaders struct {
	data map[string][]string
}

func (m *mockHeaders) GetHeader(name string) []string {
	return m.data[name]
}

func TestIsPrivate(t *testing.T) {
	tests := []struct {
		ip   string
		want bool
	}{
		{"10.0.0.1", true},
		{"172.16.0.1", true},
		{"192.168.1.1", true},
		{"127.0.0.1", true},
		{"8.8.8.8", false},
		{"1.1.1.1", false},
		{"::1", true},
		{"2001:db8::1", true},
		{"2607:f8b0:4004:800::200e", false},
	}

	for _, tt := range tests {
		t.Run(tt.ip, func(t *testing.T) {
			addr := netip.MustParseAddr(tt.ip)
			got := IsPrivate(addr)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestIsTrusted(t *testing.T) {
	trusted := []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8")}

	tests := []struct {
		ip   string
		want bool
	}{
		{"10.0.0.1", true},
		{"192.168.1.1", false},
		{"8.8.8.8", false},
	}

	for _, tt := range tests {
		t.Run(tt.ip, func(t *testing.T) {
			addr := netip.MustParseAddr(tt.ip)
			got := IsTrusted(addr, trusted)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestInList(t *testing.T) {
	require.False(t, InList(netip.MustParseAddr("1.2.3.4"), nil), "InList with nil prefixes should return false")
}

func TestNewExtractor_Default(t *testing.T) {
	ext, err := NewExtractor()
	require.NoError(t, err)
	require.NotNil(t, ext)
}

func TestNewExtractor_CacheDisabled(t *testing.T) {
	ext, err := NewExtractor(WithCacheDisabled())
	require.NoError(t, err)
	require.Nil(t, ext.cache)
}

func TestExtractor_Extract_UntrustedPeer(t *testing.T) {
	ext, _ := NewExtractor()
	peerIP := netip.MustParseAddr("8.8.8.8")
	headers := &mockHeaders{}

	result := ext.Extract(t.Context(), peerIP, headers)
	require.Equal(t, peerIP, result)
}

func TestExtractor_Extract_FromXForwardedFor(t *testing.T) {
	ext, _ := NewExtractor()
	peerIP := netip.MustParseAddr("10.0.0.1") // private/trusted

	headers := &mockHeaders{data: map[string][]string{
		HeaderXForwardedFor: {"203.0.114.1, 10.0.0.2"},
	}}

	result := ext.Extract(t.Context(), peerIP, headers)
	require.True(t, result.IsValid(), "Extract() returned invalid addr")
	require.Equal(t, "203.0.114.1", result.String())
}

func TestExtractor_Extract_FromXRealIP(t *testing.T) {
	ext, _ := NewExtractor()
	peerIP := netip.MustParseAddr("10.0.0.1")

	headers := &mockHeaders{data: map[string][]string{
		HeaderXRealIP: {"203.0.114.5"},
	}}

	result := ext.Extract(t.Context(), peerIP, headers)
	require.Equal(t, "203.0.114.5", result.String())
}

func TestExtractor_Headers(t *testing.T) {
	ext, _ := NewExtractor()
	count := 0
	for range ext.Headers() {
		count++
	}
	require.Equal(t, len(DefaultHeaders()), count)
}

func TestDefaultHeaders(t *testing.T) {
	headers := DefaultHeaders()
	require.NotEmpty(t, headers)
	require.Equal(t, InternedHeaderXForwardedFor, headers[0])
}

func TestNewContext_FromContext(t *testing.T) {
	ip := netip.MustParseAddr("1.2.3.4")
	ctx := NewContext(t.Context(), ip)
	got := FromContext(ctx)
	require.Equal(t, ip, got)
}

func TestFromContext_Empty(t *testing.T) {
	got := FromContext(t.Context())
	require.False(t, got.IsValid(), "FromContext(empty) = %v, want invalid", got)
}
