// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package clientip

import (
	"net/netip"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestExtractor_Extract_CacheHit(t *testing.T) {
	t.Parallel()
	ext, _ := NewExtractor(WithCacheSize(100), WithTrustPrivatePeers())
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

func TestExtractor_XFFCountPreservesPositions(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		values []string
		count  uint
		want   string
	}{
		{"empty hop", []string{"1.1.1.1, 8.8.8.8, "}, 1, "8.8.8.8"},
		{"overlong hop", []string{"1.1.1.1, 8.8.8.8, " + strings.Repeat("x", MaxIPLength+1)}, 1, "8.8.8.8"},
		{"invalid hop", []string{"1.1.1.1, 8.8.8.8, invalid"}, 1, "8.8.8.8"},
		{"multiline empty hop", []string{"1.1.1.1, 8.8.8.8", ""}, 1, "8.8.8.8"},
		{"invalid client", []string{"1.1.1.1, invalid, 9.9.9.9"}, 1, "10.0.0.1"},
		{"private client", []string{"1.1.1.1, 10.2.3.4, 9.9.9.9"}, 1, "10.0.0.1"},
		{"empty client", []string{"1.1.1.1, , 9.9.9.9"}, 1, "10.0.0.1"},
		{"count nine", []string{"8.8.8.8" + strings.Repeat(", 9.9.9.9", 9)}, 9, "8.8.8.8"},
		{"count ten", []string{"8.8.8.8" + strings.Repeat(", 9.9.9.9", 10)}, 10, "10.0.0.1"},
		{"count eleven", []string{"8.8.8.8" + strings.Repeat(", 9.9.9.9", 11)}, 11, "10.0.0.1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ext, err := NewExtractor(WithTrustedPeers(netip.MustParsePrefix("10.0.0.0/8")), WithTrustedProxiesCount(tc.count))
			require.NoError(t, err)
			got := ext.Extract(t.Context(), netip.MustParseAddr("10.0.0.1"), &mockHeaders{data: map[string][]string{
				HeaderXForwardedFor: tc.values,
				HeaderXRealIP:       {"6.6.6.6"},
			}})
			require.Equal(t, tc.want, got.String())
		})
	}
}

func TestExtractor_MappedTrustPrefixes(t *testing.T) {
	t.Parallel()
	for _, prefix := range []string{"10.0.0.0/24", "::ffff:10.0.0.0/120", "::ffff:10.0.0.1/128"} {
		t.Run(prefix, func(t *testing.T) {
			t.Parallel()
			ext, err := NewExtractor(WithTrustedPeers(netip.MustParsePrefix(prefix)),
				WithTrustedProxies(netip.MustParsePrefix("::ffff:9.9.9.0/120")))
			require.NoError(t, err)
			for range 2 { // Exercise both a cold and a warm parse cache.
				got := ext.Extract(t.Context(), netip.MustParseAddr("::ffff:10.0.0.1"),
					&mockHeaders{data: map[string][]string{HeaderXForwardedFor: {"::ffff:8.8.8.8, ::ffff:9.9.9.9"}}})
				require.Equal(t, "8.8.8.8", got.String())
			}
		})
	}
}

func TestExtractor_Extract_TrustedProxies(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
	ext, err := NewExtractor(WithTrustedPeers(netip.MustParsePrefix("10.0.0.0/8")))
	require.NoError(t, err)
	peerIP := netip.MustParseAddr("10.0.0.1")
	headers := &mockHeaders{data: map[string][]string{
		HeaderXRealIP: {"not-an-ip"},
	}}

	result := ext.Extract(t.Context(), peerIP, headers)
	require.Equal(t, peerIP, result)
}

func TestExtractor_Extract_WithCustomHeaders(t *testing.T) {
	t.Parallel()
	ext, _ := NewExtractor(WithHeaders("X-Custom-IP"), WithTrustPrivatePeers())
	peerIP := netip.MustParseAddr("10.0.0.1")
	headers := &mockHeaders{data: map[string][]string{
		"X-Custom-IP": {"5.6.7.8"},
	}}

	result := ext.Extract(t.Context(), peerIP, headers)
	require.Equal(t, "5.6.7.8", result.String())
}

func TestExtractor_Extract_TrustedProxiesCount(t *testing.T) {
	t.Parallel()
	ext, err := NewExtractor(WithTrustedPeers(netip.MustParsePrefix("10.0.0.0/8")), WithTrustedProxiesCount(1))
	require.NoError(t, err)
	peerIP := netip.MustParseAddr("10.0.0.1")
	headers := &mockHeaders{data: map[string][]string{
		HeaderXForwardedFor: {"1.1.1.1, 10.0.0.2"},
	}}

	result := ext.Extract(t.Context(), peerIP, headers)
	require.Equal(t, "1.1.1.1", result.String())
}

func TestExtractor_Extract_MultipleXFF(t *testing.T) {
	t.Parallel()
	ext, err := NewExtractor(WithTrustedPeers(netip.MustParsePrefix("10.0.0.0/8")))
	require.NoError(t, err)
	peerIP := netip.MustParseAddr("10.0.0.1")
	headers := &mockHeaders{data: map[string][]string{
		HeaderXForwardedFor: {"203.0.114.1, 10.0.0.2, 10.0.0.3"},
	}}

	result := ext.Extract(t.Context(), peerIP, headers)
	require.Equal(t, "203.0.114.1", result.String())
}

func TestExtractor_WithLogger(t *testing.T) {
	t.Parallel()
	ext, err := NewExtractor(WithLogger(nil))
	require.NoError(t, err)
	require.NotNil(t, ext)
}

// TestExtractor_Extract_AllPrivateXFFFallsBackToPeer is a regression for a
// spoofing gap a fuzz target found.
//
// When every X-Forwarded-For entry was private or trusted, the extractor
// returned the leftmost one — an address the client chose. The peer has to be
// trusted for the header to be read at all, but its contents are not, so a
// request carrying "X-Forwarded-For: 10.0.0.1" was reported as coming from
// 10.0.0.1, which an ipacl allowlist for 10.0.0.0/8 admits. Extract's own
// contract already said what should happen: no public IP in any header means
// the peer address.
func TestExtractor_Extract_AllPrivateXFFFallsBackToPeer(t *testing.T) {
	t.Parallel()
	ext, err := NewExtractor(WithCacheDisabled(), WithTrustPrivatePeers())
	require.NoError(t, err)

	peerIP := netip.MustParseAddr("10.0.0.9")

	for _, forwarded := range []string{
		"10.0.0.1",
		"192.168.1.1, 172.16.0.1",
		"127.0.0.1",
		"255.255.255.255",
	} {
		headers := &mockHeaders{data: map[string][]string{HeaderXForwardedFor: {forwarded}}}
		require.Equal(t, peerIP, ext.Extract(t.Context(), peerIP, headers),
			"a header of only private addresses spoke for the peer: %q", forwarded)
	}

	// A public entry among them is still the client, which is the case the
	// rightmost-non-private walk exists for.
	headers := &mockHeaders{data: map[string][]string{
		HeaderXForwardedFor: {"10.0.0.1, 203.0.114.7, 10.0.0.2"},
	}}
	require.Equal(t, netip.MustParseAddr("203.0.114.7"), ext.Extract(t.Context(), peerIP, headers))
}

// TestExtractor_Extract_TrustedPublicProxyXFFFallsBackToPeer is the regression
// guard for X-Forwarded-For being reprocessed by the generic header parser:
// after the XFF walk rejected a configured public trusted proxy, the generic
// parser accepted it again because it only filters private addresses, so
// Extract reported the proxy instead of the peer.
func TestExtractor_Extract_TrustedPublicProxyXFFFallsBackToPeer(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		cache Option
	}{
		{name: "cache disabled", cache: WithCacheDisabled()},
		{name: "cache enabled", cache: WithCacheSize(16)},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ext, err := NewExtractor(tc.cache, WithTrustedProxies(netip.MustParsePrefix("8.8.8.8/32")))
			require.NoError(t, err)

			peerIP := netip.MustParseAddr("10.0.0.1")
			headers := &mockHeaders{data: map[string][]string{HeaderXForwardedFor: {"8.8.8.8"}}}

			// The second call is served from the LRU when caching is enabled.
			for range 2 {
				require.Equal(t, peerIP, ext.Extract(t.Context(), peerIP, headers))
			}
		})
	}
}

// TestExtractor_Extract_XFFPaddingKeepsProxyAppendedEntry is a regression for
// SA-002: X-Forwarded-For entries used to be collected left to right and
// truncated at MaxIPsPerHeader, so a client that prepended that many
// addresses pushed the entry its proxy appended — the real client address —
// out of the list, and the right-to-left walk then returned a spoofed one.
// An overlong value was also skipped whole, handing the decision to the next
// (client-controlled) header.
func TestExtractor_Extract_XFFPaddingKeepsProxyAppendedEntry(t *testing.T) {
	t.Parallel()

	spoofed := make([]string, 0, 2*MaxIPsPerHeader)
	for i := range 2 * MaxIPsPerHeader {
		spoofed = append(spoofed, netip.AddrFrom4([4]byte{1, 1, 1, byte(i + 1)}).String())
	}
	padded := strings.Join(spoofed, ", ")
	long := padded + strings.Repeat(", 1.2.3.4", MaxIPLength*MaxIPsPerHeader)

	tests := []struct {
		name    string
		opts    []Option
		headers map[string][]string
		want    string
	}{
		{
			name:    "exactly ten spoofed entries",
			headers: map[string][]string{HeaderXForwardedFor: {strings.Join(spoofed[:MaxIPsPerHeader], ", ") + ", 8.8.8.8"}},
			want:    "8.8.8.8",
		},
		{
			name:    "padded single line",
			headers: map[string][]string{HeaderXForwardedFor: {padded + ", 8.8.8.8"}},
			want:    "8.8.8.8",
		},
		{
			name:    "padded separate lines",
			headers: map[string][]string{HeaderXForwardedFor: {padded, "8.8.8.8"}},
			want:    "8.8.8.8",
		},
		{
			name: "padded with trusted proxy hop",
			opts: []Option{WithTrustedProxies(netip.MustParsePrefix("9.9.9.0/24"))},
			headers: map[string][]string{
				HeaderXForwardedFor: {padded + ", 8.8.8.8, 9.9.9.9"},
			},
			want: "8.8.8.8",
		},
		{
			name:    "padded with trusted proxies count",
			opts:    []Option{WithTrustedProxiesCount(1)},
			headers: map[string][]string{HeaderXForwardedFor: {padded + ", 8.8.8.8, 9.9.9.9"}},
			want:    "8.8.8.8",
		},
		{
			name: "overlong value does not defer to X-Real-IP",
			headers: map[string][]string{
				HeaderXForwardedFor: {long + ", 8.8.8.8"},
				HeaderXRealIP:       {"6.6.6.6"},
			},
			want: "8.8.8.8",
		},
		{
			name:    "lowercase configured header name",
			opts:    []Option{WithHeaders("x-forwarded-for")},
			headers: map[string][]string{"x-forwarded-for": {"6.6.6.6, 8.8.8.8"}},
			want:    "8.8.8.8",
		},
		{
			name:    "IPv4-mapped private entry is private",
			headers: map[string][]string{HeaderXForwardedFor: {"8.8.8.8, ::ffff:10.0.0.5"}},
			want:    "8.8.8.8",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			opts := append([]Option{WithTrustedPeers(netip.MustParsePrefix("10.0.0.0/8"))}, tc.opts...)
			ext, err := NewExtractor(opts...)
			require.NoError(t, err)

			got := ext.Extract(t.Context(), netip.MustParseAddr("10.0.0.1"), &mockHeaders{data: tc.headers})
			require.Equal(t, tc.want, got.String())
		})
	}
}

// TestExtractor_Extract_PrivatePeerNotTrustedByDefault is a regression for
// SA-003: any peer in a private range used to be allowed to supply forwarding
// headers, so a workload sharing the network could claim any client address.
// Headers are honored only for an explicitly trusted peer, or for private
// peers once [WithTrustPrivatePeers] opts back in.
func TestExtractor_Extract_PrivatePeerNotTrustedByDefault(t *testing.T) {
	t.Parallel()

	peerIP := netip.MustParseAddr("10.0.0.1")
	headers := &mockHeaders{data: map[string][]string{
		HeaderXForwardedFor: {"8.8.8.8"},
		HeaderXRealIP:       {"8.8.4.4"},
	}}

	tests := []struct {
		name string
		opts []Option
		peer netip.Addr
		want netip.Addr
	}{
		{name: "private peer", peer: peerIP, want: peerIP},
		{name: "loopback peer", peer: netip.MustParseAddr("127.0.0.1"), want: netip.MustParseAddr("127.0.0.1")},
		{name: "IPv6 ULA peer", peer: netip.MustParseAddr("fd00::1"), want: netip.MustParseAddr("fd00::1")},
		{
			name: "private peer outside trusted peers",
			opts: []Option{WithTrustedPeers(netip.MustParsePrefix("10.1.0.0/16"))},
			peer: peerIP,
			want: peerIP,
		},
		{
			name: "trusted peer",
			opts: []Option{WithTrustedPeers(netip.MustParsePrefix("10.0.0.0/8"))},
			peer: peerIP,
			want: netip.MustParseAddr("8.8.8.8"),
		},
		{
			name: "peer is a trusted proxy",
			opts: []Option{WithTrustedProxies(netip.MustParsePrefix("10.0.0.0/8"))},
			peer: peerIP,
			want: netip.MustParseAddr("8.8.8.8"),
		},
		{
			name: "IPv4-mapped trusted peer",
			opts: []Option{WithTrustedPeers(netip.MustParsePrefix("10.0.0.0/8"))},
			peer: netip.MustParseAddr("::ffff:10.0.0.1"),
			want: netip.MustParseAddr("8.8.8.8"),
		},
		{
			name: "private peers opted in",
			opts: []Option{WithTrustPrivatePeers()},
			peer: peerIP,
			want: netip.MustParseAddr("8.8.8.8"),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ext, err := NewExtractor(tc.opts...)
			require.NoError(t, err)
			require.Equal(t, tc.want, ext.Extract(t.Context(), tc.peer, headers))
		})
	}
}
