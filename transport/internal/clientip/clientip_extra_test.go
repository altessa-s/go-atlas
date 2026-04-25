// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package clientip

import (
	"net/netip"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestExtractor_Extract_CacheHit(t *testing.T) {
	ext, _ := NewExtractor(WithCacheSize(100))
	peerIP := netip.MustParseAddr("10.0.0.1")
	headers := &mockHeaders{data: map[string][]string{
		HeaderXRealIP: {"203.0.114.5"},
	}}

	// First call populates cache
	result1 := ext.Extract(t.Context(), peerIP, headers)
	// Second call should hit cache
	result2 := ext.Extract(t.Context(), peerIP, headers)

	require.Equal(t, result2, result1)
}

func TestExtractor_Extract_TrustedProxies(t *testing.T) {
	trustedProxy := netip.MustParsePrefix("10.0.0.0/8")
	ext, _ := NewExtractor(WithTrustedProxies(trustedProxy))
	peerIP := netip.MustParseAddr("10.0.0.1")
	headers := &mockHeaders{data: map[string][]string{
		HeaderXForwardedFor: {"203.0.114.1"},
	}}

	result := ext.Extract(t.Context(), peerIP, headers)
	require.Equal(t, "203.0.114.1", result.String())
}

func TestExtractor_Extract_TrustedPeers(t *testing.T) {
	trustedPeer := netip.MustParsePrefix("10.0.0.0/8")
	ext, _ := NewExtractor(WithTrustedPeers(trustedPeer))
	peerIP := netip.MustParseAddr("10.0.0.1")
	headers := &mockHeaders{data: map[string][]string{
		HeaderXRealIP: {"1.2.3.4"},
	}}

	result := ext.Extract(t.Context(), peerIP, headers)
	require.Equal(t, "1.2.3.4", result.String())
}

func TestExtractor_Extract_InvalidHeader(t *testing.T) {
	ext, _ := NewExtractor()
	peerIP := netip.MustParseAddr("10.0.0.1")
	headers := &mockHeaders{data: map[string][]string{
		HeaderXRealIP: {"not-an-ip"},
	}}

	result := ext.Extract(t.Context(), peerIP, headers)
	// Should fall back to peer IP or invalid
	_ = result // just ensure no panic
}

func TestExtractor_Extract_WithCustomHeaders(t *testing.T) {
	ext, _ := NewExtractor(WithHeaders("X-Custom-IP"))
	peerIP := netip.MustParseAddr("10.0.0.1")
	headers := &mockHeaders{data: map[string][]string{
		"X-Custom-IP": {"5.6.7.8"},
	}}

	result := ext.Extract(t.Context(), peerIP, headers)
	require.Equal(t, "5.6.7.8", result.String())
}

func TestExtractor_Extract_TrustedProxiesCount(t *testing.T) {
	ext, _ := NewExtractor(WithTrustedProxiesCount(1))
	peerIP := netip.MustParseAddr("10.0.0.1")
	headers := &mockHeaders{data: map[string][]string{
		HeaderXForwardedFor: {"1.1.1.1, 10.0.0.2"},
	}}

	result := ext.Extract(t.Context(), peerIP, headers)
	_ = result // verify no panic with proxy count
}

func TestExtractor_Extract_MultipleXFF(t *testing.T) {
	ext, _ := NewExtractor()
	peerIP := netip.MustParseAddr("10.0.0.1")
	headers := &mockHeaders{data: map[string][]string{
		HeaderXForwardedFor: {"203.0.114.1, 10.0.0.2, 10.0.0.3"},
	}}

	result := ext.Extract(t.Context(), peerIP, headers)
	require.True(t, result.IsValid(), "Extract() returned invalid addr")
}

func TestExtractor_WithLogger(t *testing.T) {
	ext, err := NewExtractor(WithLogger(nil))
	require.NoError(t, err)
	require.NotNil(t, ext)
}
