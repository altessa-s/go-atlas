// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package clientip

import (
	"net/netip"
)

// ParsePrefixes parses string IP/CIDR prefixes to netip.Prefix.
// Each string can be either a CIDR notation (e.g. "10.0.0.0/8") or
// a single IP address (e.g. "192.168.1.1"), which is converted to a
// host prefix (/32 for IPv4, /128 for IPv6).
func ParsePrefixes(strs []string) ([]netip.Prefix, error) {
	result := make([]netip.Prefix, 0, len(strs))
	for _, s := range strs {
		prefix, err := netip.ParsePrefix(s)
		if err != nil {
			// Try as single IP address
			addr, err := netip.ParseAddr(s)
			if err != nil {
				return nil, err
			}
			prefix = netip.PrefixFrom(addr, addr.BitLen())
		}
		result = append(result, prefix)
	}
	return result, nil
}
