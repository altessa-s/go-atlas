// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package clientip

import (
	"net/netip"
	"testing"

	"github.com/stretchr/testify/require"
)

// fuzzHeaders is a HeaderGetter over a plain map.
type fuzzHeaders struct {
	data map[string][]string
}

func (h *fuzzHeaders) GetHeader(key string) []string { return h.data[key] }

// spoofSeeds are header values worth starting from: ordinary proxy chains, and
// the shapes that try to talk the extractor into trusting them.
var spoofSeeds = []string{
	"1.2.3.4",
	"203.0.113.9, 10.0.0.1",
	"10.0.0.1, 203.0.113.9",
	"127.0.0.1",
	"::1",
	"192.168.1.1",
	"not-an-ip",
	"",
	"1.2.3.4, 5.6.7.8, 9.10.11.12",
	"::ffff:10.0.0.1",
	"0.0.0.0",
	"255.255.255.255",
}

// FuzzExtractIgnoresHeadersFromAnUntrustedPeer is the spoofing oracle, and the
// reason this package is worth fuzzing at all.
//
// The address Extract returns is what `ipacl` and `geoacl` then match against
// their prefixes and what the rate limiters key on. A caller that can move that
// address with a header it controls does not bypass one check — it bypasses
// every check downstream of this one, from a request that looks entirely
// ordinary.
//
// The rule is the first line of Extract's own contract: a peer that is neither
// trusted nor private is returned immediately, headers unread. So for such a
// peer the result must equal the peer address no matter what the headers say.
func FuzzExtractIgnoresHeadersFromAnUntrustedPeer(f *testing.F) {
	for _, seed := range spoofSeeds {
		f.Add("8.8.8.8", seed, seed)
		f.Add("203.0.113.1", seed, seed)
	}

	extractor, err := NewExtractor(
		WithCacheDisabled(),
		WithHeadersEnabled(),
		WithTrustedProxies(netip.MustParsePrefix("10.0.0.0/8")),
	)
	require.NoError(f, err)

	f.Fuzz(func(t *testing.T, peerStr, forwardedFor, realIP string) {
		peer, err := netip.ParseAddr(peerStr)
		if err != nil {
			t.Skip("not an address; the caller supplies a parsed peer")
		}
		if IsTrusted(peer, extractor.opts.trustedPeers) || IsPrivate(peer) {
			t.Skip("a trusted or private peer is allowed to be spoken for by its headers")
		}

		got := extractor.Extract(t.Context(), peer, &fuzzHeaders{data: map[string][]string{
			HeaderXForwardedFor: {forwardedFor},
			HeaderXRealIP:       {realIP},
		}})

		require.Equal(t, peer, got,
			"an untrusted peer was spoken for by its own headers: xff=%q real-ip=%q", forwardedFor, realIP)
	})
}

// FuzzExtractNeverReturnsAPrivateHeaderAddress pins the other half: a header may
// only ever contribute a public address.
//
// A private address arriving from a header is the more interesting direction of
// the same attack. Internal ranges are exactly what an `ipacl` allowlist is
// written in terms of — "10.0.0.0/8 may reach /admin" — so a client that can
// make Extract report 10.0.0.1 gets in through the front door. The peer address
// is the one legitimate exception: it is the connection's own, not a claim.
func FuzzExtractNeverReturnsAPrivateHeaderAddress(f *testing.F) {
	for _, seed := range spoofSeeds {
		f.Add("10.0.0.1", seed, seed)
		f.Add("127.0.0.1", seed, seed)
	}

	extractor, err := NewExtractor(
		WithCacheDisabled(),
		WithHeadersEnabled(),
		WithTrustedProxies(netip.MustParsePrefix("10.0.0.0/8")),
	)
	require.NoError(f, err)

	f.Fuzz(func(t *testing.T, peerStr, forwardedFor, realIP string) {
		peer, err := netip.ParseAddr(peerStr)
		if err != nil {
			t.Skip("not an address; the caller supplies a parsed peer")
		}

		got := extractor.Extract(t.Context(), peer, &fuzzHeaders{data: map[string][]string{
			HeaderXForwardedFor: {forwardedFor},
			HeaderXRealIP:       {realIP},
		}})

		if got == peer {
			return // The fallback: the connection's own address, not a claim.
		}

		require.True(t, got.IsValid(),
			"a header produced an invalid address: xff=%q real-ip=%q", forwardedFor, realIP)
		require.False(t, IsPrivate(got),
			"a private address arrived from a header and was believed: %s (xff=%q real-ip=%q)",
			got, forwardedFor, realIP)
	})
}

// FuzzExtractIsDeterministic pins that the answer does not depend on how the
// extractor was warmed.
//
// The parsed-address LRU sits in front of the private-range check, so a cache
// hit and a cache miss travel different code paths to the same decision. If
// they can disagree, the address a request is judged by depends on what other
// requests happened to run before it — which is both a correctness bug and, for
// an ACL, an intermittently exploitable one.
func FuzzExtractIsDeterministic(f *testing.F) {
	for _, seed := range spoofSeeds {
		f.Add("10.0.0.1", seed)
		f.Add("8.8.8.8", seed)
	}

	cached, err := NewExtractor(
		WithCacheSize(64),
		WithHeadersEnabled(),
		WithTrustedProxies(netip.MustParsePrefix("10.0.0.0/8")),
	)
	require.NoError(f, err)

	uncached, err := NewExtractor(
		WithCacheDisabled(),
		WithHeadersEnabled(),
		WithTrustedProxies(netip.MustParsePrefix("10.0.0.0/8")),
	)
	require.NoError(f, err)

	f.Fuzz(func(t *testing.T, peerStr, forwardedFor string) {
		peer, err := netip.ParseAddr(peerStr)
		if err != nil {
			t.Skip("not an address; the caller supplies a parsed peer")
		}

		headers := func() *fuzzHeaders {
			return &fuzzHeaders{data: map[string][]string{HeaderXForwardedFor: {forwardedFor}}}
		}

		// Twice through the cached extractor: the second call is a cache hit.
		first := cached.Extract(t.Context(), peer, headers())
		second := cached.Extract(t.Context(), peer, headers())
		require.Equal(t, first, second, "a warm cache changed the verdict for xff=%q", forwardedFor)

		require.Equal(t, uncached.Extract(t.Context(), peer, headers()), first,
			"the cached and uncached paths disagree for xff=%q", forwardedFor)
	})
}
