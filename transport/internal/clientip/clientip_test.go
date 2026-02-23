// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package clientip

import (
	"net/netip"
	"testing"
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
			if got := IsPrivate(addr); got != tt.want {
				t.Fatalf("IsPrivate(%s) = %v, want %v", tt.ip, got, tt.want)
			}
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
			if got := IsTrusted(addr, trusted); got != tt.want {
				t.Fatalf("IsTrusted(%s) = %v, want %v", tt.ip, got, tt.want)
			}
		})
	}
}

func TestInList(t *testing.T) {
	if InList(netip.MustParseAddr("1.2.3.4"), nil) {
		t.Fatal("InList with nil prefixes should return false")
	}
}

func TestNewExtractor_Default(t *testing.T) {
	ext, err := NewExtractor()
	if err != nil {
		t.Fatalf("NewExtractor() error = %v", err)
	}
	if ext == nil {
		t.Fatal("NewExtractor() returned nil")
	}
}

func TestNewExtractor_CacheDisabled(t *testing.T) {
	ext, err := NewExtractor(WithCacheDisabled())
	if err != nil {
		t.Fatalf("NewExtractor() error = %v", err)
	}
	if ext.cache != nil {
		t.Fatal("expected cache to be nil when disabled")
	}
}

func TestExtractor_Extract_UntrustedPeer(t *testing.T) {
	ext, _ := NewExtractor()
	peerIP := netip.MustParseAddr("8.8.8.8")
	headers := &mockHeaders{}

	result := ext.Extract(t.Context(), peerIP, headers)
	if result != peerIP {
		t.Fatalf("Extract() = %v, want %v (untrusted peer)", result, peerIP)
	}
}

func TestExtractor_Extract_FromXForwardedFor(t *testing.T) {
	ext, _ := NewExtractor()
	peerIP := netip.MustParseAddr("10.0.0.1") // private/trusted

	headers := &mockHeaders{data: map[string][]string{
		HeaderXForwardedFor: {"203.0.114.1, 10.0.0.2"},
	}}

	result := ext.Extract(t.Context(), peerIP, headers)
	if !result.IsValid() {
		t.Fatal("Extract() returned invalid addr")
	}
	if result.String() != "203.0.114.1" {
		t.Fatalf("Extract() = %v, want 203.0.114.1", result)
	}
}

func TestExtractor_Extract_FromXRealIP(t *testing.T) {
	ext, _ := NewExtractor()
	peerIP := netip.MustParseAddr("10.0.0.1")

	headers := &mockHeaders{data: map[string][]string{
		HeaderXRealIP: {"203.0.114.5"},
	}}

	result := ext.Extract(t.Context(), peerIP, headers)
	if result.String() != "203.0.114.5" {
		t.Fatalf("Extract() = %v, want 203.0.114.5", result)
	}
}

func TestExtractor_Headers(t *testing.T) {
	ext, _ := NewExtractor()
	count := 0
	for range ext.Headers() {
		count++
	}
	if count != len(DefaultHeaders()) {
		t.Fatalf("Headers() yielded %d, want %d", count, len(DefaultHeaders()))
	}
}

func TestDefaultHeaders(t *testing.T) {
	headers := DefaultHeaders()
	if len(headers) == 0 {
		t.Fatal("DefaultHeaders() returned empty")
	}
	if headers[0] != InternedHeaderXForwardedFor {
		t.Fatalf("first header = %q, want X-Forwarded-For", headers[0])
	}
}

func TestNewContext_FromContext(t *testing.T) {
	ip := netip.MustParseAddr("1.2.3.4")
	ctx := NewContext(t.Context(), ip)
	got := FromContext(ctx)
	if got != ip {
		t.Fatalf("FromContext() = %v, want %v", got, ip)
	}
}

func TestFromContext_Empty(t *testing.T) {
	got := FromContext(t.Context())
	if got.IsValid() {
		t.Fatalf("FromContext(empty) = %v, want invalid", got)
	}
}

func TestConstants(t *testing.T) {
	if MaxIPsPerHeader != 10 {
		t.Fatalf("MaxIPsPerHeader = %d", MaxIPsPerHeader)
	}
	if MaxIPLength != 45 {
		t.Fatalf("MaxIPLength = %d", MaxIPLength)
	}
	if MaxTotalIPs != 50 {
		t.Fatalf("MaxTotalIPs = %d", MaxTotalIPs)
	}
}
