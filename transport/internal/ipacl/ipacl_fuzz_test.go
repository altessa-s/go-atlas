// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package ipacl_test

import (
	"net/netip"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/transport/internal/ipacl"
)

// FuzzEvaluateDenyBeatsAllow pins the precedence an operator relies on: an
// address matched by the denylist is refused no matter what else matches it.
//
// The two lists are written by different people at different times — a broad
// allow for an office range, a narrow deny for a compromised host inside it —
// and the whole value of the narrow entry is that it wins. Overlap is exactly
// where an evaluation order gets it wrong, and overlapping prefixes are what a
// fuzzer generates without being asked.
func FuzzEvaluateDenyBeatsAllow(f *testing.F) {
	f.Add(uint32(0x0A000001), uint8(8), uint8(32), true)  // 10.0.0.1 in a /8 allow and a /32 deny.
	f.Add(uint32(0xC0A80001), uint8(16), uint8(24), true) // 192.168.0.1
	f.Add(uint32(0x7F000001), uint8(0), uint8(0), false)  // 127.0.0.1 with /0 on both.
	f.Add(uint32(0), uint8(32), uint8(0), true)

	f.Fuzz(func(t *testing.T, raw uint32, allowBits, denyBits uint8, defaultAllow bool) {
		addr := addrOf(raw)

		allow, err := addr.Prefix(int(allowBits) % 33)
		if err != nil {
			t.Skip("unrepresentable prefix length")
		}
		deny, err := addr.Prefix(int(denyBits) % 33)
		if err != nil {
			t.Skip("unrepresentable prefix length")
		}

		policy := ipacl.PolicyDeny
		if defaultAllow {
			policy = ipacl.PolicyAllow
		}

		registry := ipacl.NewRegistry(policy)
		registry.Register("/api", &ipacl.AccessRule{
			Allowlist: []netip.Prefix{allow},
			Denylist:  []netip.Prefix{deny},
		})

		// Both prefixes were derived from addr, so both contain it: the address
		// is allowed and denied at once, and deny has to win.
		require.False(t, registry.Evaluate(addr, "/api"),
			"an address on the denylist was allowed: addr=%s allow=%s deny=%s", addr, allow, deny)
	})
}

// FuzzEvaluateUnknownEndpointFollowsThePolicy pins the fallback: an endpoint
// nobody registered is decided by the registry's default policy and nothing
// else.
//
// This is the case that decides what a new route does before anyone has thought
// about it. A registry built with PolicyDeny that quietly allows an unmatched
// path — because a pattern almost matched, or a lookup fell through — is a
// route shipped without an ACL and no sign of it.
func FuzzEvaluateUnknownEndpointFollowsThePolicy(f *testing.F) {
	f.Add(uint32(0x0A000001), "/unregistered", true)
	f.Add(uint32(0x0A000001), "", false)
	f.Add(uint32(0), "/api", false)
	f.Add(uint32(0xFFFFFFFF), "/api/v1/../admin", true)

	f.Fuzz(func(t *testing.T, raw uint32, endpoint string, defaultAllow bool) {
		if endpoint == "/registered" {
			t.Skip("the fixture registers this one")
		}

		addr := addrOf(raw)

		policy := ipacl.PolicyDeny
		if defaultAllow {
			policy = ipacl.PolicyAllow
		}

		registry := ipacl.NewRegistry(policy)
		// One unrelated rule, so the lookup has something to walk past rather
		// than short-circuiting on an empty registry.
		registry.Register("/registered", &ipacl.AccessRule{
			Allowlist: []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8")},
		})

		require.Equal(t, defaultAllow, registry.Evaluate(addr, endpoint),
			"an unregistered endpoint did not fall back to the default policy: addr=%s endpoint=%q", addr, endpoint)
	})
}

// FuzzEvaluateFollowsTheDocumentedPrecedence restates [ipacl.Registry.Evaluate]'s
// three-way decision independently and requires the implementation to agree for
// every combination of address, prefixes and default policy.
//
// Worth being explicit about what this does *not* claim: an allowlist here is
// not exclusive. An address in neither list falls back to the registry policy,
// so a non-empty allowlist under PolicyAllow admits addresses it never named.
// That is the documented contract, and pinning it is the point — an ACL whose
// precedence quietly changes is one nobody re-reads before trusting it.
func FuzzEvaluateFollowsTheDocumentedPrecedence(f *testing.F) {
	f.Add(uint32(0x0A000001), uint32(0x0A000000), uint8(8), uint32(0x0A000001), uint8(32), true)
	f.Add(uint32(0x08080808), uint32(0x7F000000), uint8(24), uint32(0), uint8(0), false)
	f.Add(uint32(0xFFFFFFFF), uint32(0), uint8(1), uint32(0), uint8(1), true)
	f.Add(uint32(0), uint32(0), uint8(32), uint32(0), uint8(32), false)

	f.Fuzz(func(t *testing.T, probeRaw, allowRaw uint32, allowBits uint8, denyRaw uint32, denyBits uint8, defaultAllow bool) {
		probe := addrOf(probeRaw)

		allow, err := addrOf(allowRaw).Prefix(int(allowBits) % 33)
		if err != nil {
			t.Skip("unrepresentable prefix length")
		}
		deny, err := addrOf(denyRaw).Prefix(int(denyBits) % 33)
		if err != nil {
			t.Skip("unrepresentable prefix length")
		}

		policy := ipacl.PolicyDeny
		if defaultAllow {
			policy = ipacl.PolicyAllow
		}

		registry := ipacl.NewRegistry(policy)
		registry.Register("/api", &ipacl.AccessRule{
			Allowlist: []netip.Prefix{allow},
			Denylist:  []netip.Prefix{deny},
		})

		// The documented rule, spelled out here rather than reused from the
		// implementation: an oracle that shares code with its subject agrees
		// with it even when both are wrong.
		want := defaultAllow
		switch {
		case deny.Contains(probe):
			want = false
		case allow.Contains(probe):
			want = true
		}

		require.Equal(t, want, registry.Evaluate(probe, "/api"),
			"probe=%s allow=%s deny=%s policy=%v", probe, allow, deny, policy)
	})
}

// addrOf renders a fuzzed word as an IPv4 address.
func addrOf(raw uint32) netip.Addr {
	return netip.AddrFrom4([4]byte{
		byte(raw >> 24), byte(raw >> 16), byte(raw >> 8), byte(raw),
	})
}
