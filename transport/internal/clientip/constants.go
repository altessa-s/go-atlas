// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package clientip

import (
	"net/netip"

	coreslices "github.com/altessa-s/go-atlas/core/collections/slices"
	corestrings "github.com/altessa-s/go-atlas/core/text/strings"
)

// Constants for IP processing limits.
const (
	// MaxIPsPerHeader is the maximum IPs to process from a single header.
	MaxIPsPerHeader = 10

	// MaxIPLength is the maximum IP string length (IPv6 max).
	MaxIPLength = 45

	// MaxTotalIPs is the maximum total IPs to process across all headers.
	MaxTotalIPs = 50
)

// Header constants for common proxy headers.
const (
	// HeaderXForwardedFor is the X-Forwarded-For header.
	HeaderXForwardedFor = "X-Forwarded-For"

	// HeaderXRealIP is the X-Real-IP header.
	HeaderXRealIP = "X-Real-IP"

	// HeaderXClientIP is the X-Client-IP header.
	HeaderXClientIP = "X-Client-IP"

	// HeaderCFConnectingIP is the Cloudflare CF-Connecting-IP header.
	HeaderCFConnectingIP = "CF-Connecting-IP"

	// HeaderFastlyClientIP is the Fastly-Client-Ip header.
	HeaderFastlyClientIP = "Fastly-Client-Ip"

	// HeaderTrueClientIP is the True-Client-Ip header.
	HeaderTrueClientIP = "True-Client-Ip"
)

// Interned header names for memory efficiency.
// These deduplicated strings avoid per-request allocations when the same
// header name is used across many concurrent extractions.
var (
	InternedHeaderXForwardedFor  = corestrings.InternString(HeaderXForwardedFor)
	InternedHeaderXRealIP        = corestrings.InternString(HeaderXRealIP)
	InternedHeaderXClientIP      = corestrings.InternString(HeaderXClientIP)
	InternedHeaderCFConnectingIP = corestrings.InternString(HeaderCFConnectingIP)
	InternedHeaderFastlyClientIP = corestrings.InternString(HeaderFastlyClientIP)
	InternedHeaderTrueClientIP   = corestrings.InternString(HeaderTrueClientIP)
)

// DefaultHeaders returns the default ordered set of proxy headers consulted
// during client IP extraction. The order matters: X-Forwarded-For is checked
// first because it is the most widely used proxy header.
func DefaultHeaders() []string {
	return []string{
		InternedHeaderXForwardedFor, // Most common, process first
		InternedHeaderXRealIP,
		InternedHeaderXClientIP,
		InternedHeaderCFConnectingIP,
		InternedHeaderFastlyClientIP,
		InternedHeaderTrueClientIP,
	}
}

// privateAndLocalRanges contains loopback, private, link local, and default unicast prefixes.
var privateAndLocalRanges = []netip.Prefix{
	netip.MustParsePrefix("10.0.0.0/8"),         // RFC1918
	netip.MustParsePrefix("172.16.0.0/12"),      // private
	netip.MustParsePrefix("192.168.0.0/16"),     // private
	netip.MustParsePrefix("127.0.0.0/8"),        // RFC5735
	netip.MustParsePrefix("0.0.0.0/8"),          // RFC1122 Section 3.2.1.3
	netip.MustParsePrefix("169.254.0.0/16"),     // RFC3927
	netip.MustParsePrefix("192.0.0.0/24"),       // RFC 5736
	netip.MustParsePrefix("192.0.2.0/24"),       // RFC 5737
	netip.MustParsePrefix("198.51.100.0/24"),    // Assigned as TEST-NET-2
	netip.MustParsePrefix("203.0.113.0/24"),     // Assigned as TEST-NET-3
	netip.MustParsePrefix("192.88.99.0/24"),     // RFC 3068
	netip.MustParsePrefix("192.18.0.0/15"),      // RFC 2544
	netip.MustParsePrefix("224.0.0.0/4"),        // RFC 3171
	netip.MustParsePrefix("240.0.0.0/4"),        // RFC 1112
	netip.MustParsePrefix("255.255.255.255/32"), // RFC 919 Section 7
	netip.MustParsePrefix("100.64.0.0/10"),      // RFC 6598
	netip.MustParsePrefix("::/128"),             // RFC 4291: Unspecified Address
	netip.MustParsePrefix("::1/128"),            // RFC 4291: Loopback Address
	netip.MustParsePrefix("100::/64"),           // RFC 6666: Discard Address Block
	netip.MustParsePrefix("2001::/23"),          // RFC 2928: IETF Protocol Assignments
	netip.MustParsePrefix("2001:2::/48"),        // RFC 5180: Benchmarking
	netip.MustParsePrefix("2001:db8::/32"),      // RFC 3849: Documentation
	netip.MustParsePrefix("2001::/32"),          // RFC 4380: TEREDO
	netip.MustParsePrefix("fc00::/7"),           // RFC 4193: Unique-Local
	netip.MustParsePrefix("fe80::/10"),          // RFC 4291: Section 2.5.6 Link-Scoped Unicast
	netip.MustParsePrefix("ff00::/8"),           // RFC 4291: Section 2.7
	netip.MustParsePrefix("2002::/16"),          // RFC 7526: 6to4 anycast prefix deprecated
}

// IsPrivate reports whether ip falls within any RFC-defined private, loopback,
// link-local, or reserved address range. The full list covers IPv4 and IPv6
// ranges from RFC 1918, RFC 4193, RFC 5735, and others.
func IsPrivate(ip netip.Addr) bool {
	return InList(ip, privateAndLocalRanges)
}

// IsTrusted reports whether ip belongs to any of the given trusted network
// prefixes. An empty trustedPrefixes slice always returns false.
func IsTrusted(ip netip.Addr, trustedPrefixes []netip.Prefix) bool {
	return InList(ip, trustedPrefixes)
}

// InList reports whether ip is contained in any of the given CIDR prefixes.
func InList(ip netip.Addr, prefixes []netip.Prefix) bool {
	return coreslices.Any(prefixes, func(prefix netip.Prefix) bool {
		return prefix.Contains(ip)
	})
}
