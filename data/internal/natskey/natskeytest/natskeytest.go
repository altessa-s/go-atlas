// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package natskeytest

// LegacyKey is already a valid NATS KeyValue key without '=', so it must be
// stored under itself.
const LegacyKey = "plain-key_1/v2.x"

// Keys returns distinct logical keys a NATS KeyValue bucket rejects verbatim.
// "a:b" and "a=3Ab" are both included: they must not reach the same entry.
func Keys() []string {
	return []string{
		// Transport key wrapped by the Keeper in the empty namespace.
		"idk:v2:9:namespace0:38:idk:v2:9:transport12:POST:/orders3:123",
		"idk:v2:9:namespace4:acme9:invoice-1", // Keeper namespace
		"idk:POST:/orders:123",                // ':'-joined composite key
		"2001:db8::1",                         // IPv6 limiter key
		"κλειδί.ü",                            // Unicode
		"a..b",                                // empty subject token
		".lead",                               // leading dot
		"trail.",                              // trailing dot
		"a*b>c",                               // subject wildcards
		"with space",                          // whitespace
		"a:b",                                 // escaped by the encoding...
		"a=3Ab",                               // ...and its escaped look-alike
	}
}
