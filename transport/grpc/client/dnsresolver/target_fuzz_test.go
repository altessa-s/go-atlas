// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package dnsresolver

import (
	"net"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// FuzzParseTarget states the properties of target parsing. An accepted target
// yields a non-empty host and a non-empty port without a colon. A target
// ending in the port separator is rejected unless it is an IPv6 literal such
// as "::". A host name (not an IP literal) parses back to itself and the
// port once joined as host:port, so the addresses the resolver builds from
// lookups name the target's port. IP literals are exempt from the round trip
// because, as in grpc-go, a zone is taken verbatim: "::%]0" is accepted, and
// the address it yields fails when dialed.
func FuzzParseTarget(f *testing.F) {
	for _, seed := range []string{
		"orders.default.svc:9000", "inventory", "127.0.0.1", "127.0.0.1:443", "::", "::1", "[::1]", "[::1]:80",
		"[fe80::1%eth0]:1234", "fe80::1%eth0", ":80", "", ":", "orders:", "[::1]:", "[2001:db8::1", "a:b:c", "[::1]x:80",
		"host:http", "[]:80", " :80", "orders.test.:9000",
	} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, target string) {
		host, port, err := parseTarget(target, defaultPort)
		if err != nil {
			return
		}
		require.NotEmpty(t, host)
		require.NotEmpty(t, port)
		require.NotContains(t, port, ":")
		if strings.HasSuffix(target, ":") {
			require.True(t, isIP(target), "accepted %q, which ends with the port separator", target)
		}
		if isIP(host) {
			return
		}

		again, againPort, err := parseTarget(net.JoinHostPort(host, port), defaultPort)
		require.NoError(t, err, "host %q port %q from %q", host, port, target)
		require.Equal(t, host, again)
		require.Equal(t, port, againPort)
	})
}
