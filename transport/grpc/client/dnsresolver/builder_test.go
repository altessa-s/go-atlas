// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package dnsresolver

import (
	"bytes"
	"encoding/binary"
	"net"
	"net/url"
	"slices"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/transport/grpc/client/internal/dnshook"

	"google.golang.org/grpc/resolver"
)

func TestBuilderScheme(t *testing.T) {
	t.Parallel()
	require.Equal(t, "dns", NewBuilder().Scheme())
}

// IP literals resolve to themselves once, as with grpc-go's resolver, and
// start no polling.
func TestBuildIPLiteralIsStatic(t *testing.T) {
	t.Parallel()

	tests := []struct{ endpoint, want string }{
		{"127.0.0.1", "127.0.0.1:443"},
		{"127.0.0.1:12345", "127.0.0.1:12345"},
		{"::1", "[::1]:443"},
		{"[::1]", "[::1]:443"},
		{"[::1]:12345", "[::1]:12345"},
		{"2001:db8:85a3::8a2e:370:7334", "[2001:db8:85a3::8a2e:370:7334]:443"},
		{"[2001:db8:85a3::8a2e:370:7334]:12345", "[2001:db8:85a3::8a2e:370:7334]:12345"},
		{"[2001:db8::1]:http", "[2001:db8::1]:http"},
		{"[fe80::1%eth0]:1234", "[fe80::1%eth0]:1234"},
		{"fe80::1%eth0", "[fe80::1%eth0]:443"},
	}
	for _, tt := range tests {
		t.Run(tt.endpoint, func(t *testing.T) {
			t.Parallel()
			synctest.Test(t, func(t *testing.T) {
				dns := &fakeDNS{addrs: []string{"10.0.0.1"}}
				cc := &fakeConn{}
				r, err := NewBuilder(withLookup(dns.lookup)).Build(dnsTarget("", tt.endpoint), cc, resolver.BuildOptions{})
				require.NoError(t, err)
				defer r.Close()
				r.ResolveNow(resolver.ResolveNowOptions{})
				time.Sleep(time.Hour)
				synctest.Wait()
				require.Equal(t, [][]string{{tt.want}}, cc.addresses())
				require.Empty(t, dns.lookups())
			})
		})
	}
}

func TestBuildDefaultPort(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		dns := &fakeDNS{addrs: []string{"10.0.0.1"}}
		cc := &fakeConn{}
		r, err := NewBuilder(withLookup(dns.lookup)).Build(dnsTarget("", "orders.test"), cc, resolver.BuildOptions{})
		require.NoError(t, err)
		defer r.Close()
		synctest.Wait()
		require.Equal(t, [][]string{{"10.0.0.1:443"}}, cc.addresses())
	})
}

func TestBuildRejectsInvalidTargets(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		target resolver.Target
		want   string
	}{
		{"empty", dnsTarget("", ""), "missing address"},
		{"only colon", dnsTarget("", ":"), "missing port"},
		{"missing port", dnsTarget("", "orders.test:"), "missing port"},
		{"missing IPv6 port", dnsTarget("", "[2001:db8::1]:"), "missing port"},
		{"unclosed bracket", dnsTarget("", "[2001:db8::1"), "invalid address"},
		{"too many colons", dnsTarget("", "a:b:c"), "invalid address"},
		{"DNS server missing port", dnsTarget("dns.example.com:", "orders.test:9000"), "DNS server"},
		{"opaque", resolver.Target{URL: url.URL{Scheme: Scheme, Opaque: "orders.test:"}}, "missing port"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cc := &fakeConn{}
			r, err := NewBuilder().Build(tt.target, cc, resolver.BuildOptions{})
			require.Nil(t, r)
			require.ErrorContains(t, err, "dnsresolver: ")
			require.ErrorContains(t, err, tt.want)
			require.Empty(t, cc.addresses())
		})
	}
}

// dns://server/host:port sends the queries to server instead of the system
// resolver. The fake server only records the query; the lookup times out.
func TestBuildQueriesTheDNSServerOfTheAuthority(t *testing.T) {
	t.Parallel()
	server, err := net.ListenPacket("udp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = server.Close() })

	cc := &fakeConn{}
	r, err := NewBuilder(WithLookupTimeout(200*time.Millisecond)).
		Build(dnsTarget(server.LocalAddr().String(), "orders.test:9000"), cc, resolver.BuildOptions{})
	require.NoError(t, err)
	t.Cleanup(r.Close)

	buf := make([]byte, 512)
	require.NoError(t, server.SetReadDeadline(time.Now().Add(5*time.Second)))
	n, _, err := server.ReadFrom(buf)
	require.NoError(t, err)
	require.True(t, bytes.Contains(buf[:n], []byte("\x06orders\x04test")), "query %q does not ask for orders.test", buf[:n])
}

// startDNSServer serves DNS on a UDP socket of 127.0.0.1: an A query for
// orders.test gets 10.0.0.1, an AAAA query for it gets no answer with
// aaaaRCode, and any other name NXDOMAIN.
func startDNSServer(t *testing.T, aaaaRCode byte) string {
	t.Helper()
	conn, err := net.ListenPacket("udp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	go func() {
		buf := make([]byte, 1500)
		for {
			n, addr, err := conn.ReadFrom(buf)
			if err != nil {
				return
			}
			if resp := dnsResponse(buf[:n], aaaaRCode); resp != nil {
				_, _ = conn.WriteTo(resp, addr)
			}
		}
	}()
	return conn.LocalAddr().String()
}

// dnsResponse answers the single question of query, or returns nil for a
// malformed one. The response is authoritative and carries no EDNS record.
func dnsResponse(query []byte, aaaaRCode byte) []byte {
	const (
		headerLen = 12
		typeA     = 1
		rcodeNX   = 3
	)
	end := headerLen
	for end < len(query) && query[end] != 0 {
		end += int(query[end]) + 1
	}
	end += 5 // root label, type, class
	if end > len(query) {
		return nil
	}
	name, qtype := query[headerLen:end-4], binary.BigEndian.Uint16(query[end-4:])
	resp := slices.Clone(query[:end])
	resp[2] = 0x84 | query[2]&0x01 // response, authoritative, recursion desired as asked
	resp[3] = 0x80                 // recursion available, no error
	clear(resp[6:headerLen])       // no answer, authority or additional records yet
	switch {
	case !bytes.Equal(name, []byte("\x06orders\x04test\x00")):
		resp[3] |= rcodeNX
	case qtype == typeA:
		resp[7] = 1
		// Name pointer to the question, type A, class IN, TTL 30, 10.0.0.1.
		resp = append(resp, 0xc0, headerLen, 0, typeA, 0, 1, 0, 0, 0, 30, 0, 4, 10, 0, 0, 1)
	default:
		resp[3] |= aaaaRCode
	}
	return resp
}

// One failed query of a lookup fails the whole lookup: with an AAAA query
// failing with SERVFAIL, Go's resolver would otherwise return the A answer
// alone, and the connection would lose its IPv6 addresses until the next
// lookup.
func TestBuildFailsLookupsWithAFailedQuery(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name      string
		aaaaRCode byte
		ok        bool
	}{
		{"AAAA without records", 0, true},
		{"AAAA SERVFAIL", 2, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			server := startDNSServer(t, tc.aaaaRCode)
			cc := &fakeConn{}
			r, err := NewBuilder(WithLookupTimeout(5*time.Second)).Build(dnsTarget(server, testTarget), cc, resolver.BuildOptions{})
			require.NoError(t, err)
			t.Cleanup(r.Close)

			require.Eventually(t, func() bool { return len(cc.addresses())+len(cc.errors()) > 0 }, 10*time.Second, 5*time.Millisecond)
			if tc.ok {
				require.Equal(t, [][]string{{"10.0.0.1:9000"}}, cc.addresses())
				require.Empty(t, cc.errors())
				return
			}
			require.Empty(t, cc.addresses(), "a partial answer is not reported")
			require.NotErrorIs(t, cc.errors()[0], errNoAddresses)
			require.ErrorContains(t, cc.errors()[0], "lookup orders.test")
		})
	}
}

// Tests outside the package reach the lookup through dnshook.
func TestDNSHookInstallsLookup(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		dns := &fakeDNS{addrs: []string{"10.0.0.9"}}
		opt, ok := dnshook.WithLookup(dns.lookup).(Option)
		require.True(t, ok)
		cc := &fakeConn{}
		r, err := NewBuilder(opt).Build(dnsTarget("", testTarget), cc, resolver.BuildOptions{})
		require.NoError(t, err)
		defer r.Close()
		synctest.Wait()
		require.Equal(t, [][]string{{"10.0.0.9:9000"}}, cc.addresses())
	})
}
