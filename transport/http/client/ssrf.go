// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package client

import (
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"reflect"
	"slices"
	"syscall"
	"time"

	"github.com/altessa-s/go-atlas/transport/clientip"
)

// validateSSRFTransport rejects configurations whose dialing cannot be guarded.
// It never invokes caller proxy callbacks while deciding whether they are safe.
func validateSSRFTransport(opts *options) error {
	if !opts.ssrfProtection {
		return nil
	}
	transport := http.DefaultTransport
	if opts.client != nil && opts.client.Transport != nil {
		transport = opts.client.Transport
	}
	if opts.transport != nil {
		transport = opts.transport
	}
	t, ok := transport.(*http.Transport)
	if !ok || t == nil {
		return fmt.Errorf("%w: requires a non-nil *http.Transport", ErrSSRFUnsafeTransport)
	}
	if t.DialTLS != nil || t.DialTLSContext != nil { //nolint:staticcheck // deprecated dialers can still bypass the guard
		return fmt.Errorf("%w: custom TLS dialer", ErrSSRFUnsafeTransport)
	}
	if opts.proxyTLSConfig != nil {
		return fmt.Errorf("%w: proxy TLS configuration", ErrSSRFUnsafeTransport)
	}
	proxy := t.Proxy
	if opts.proxy != nil {
		proxy = opts.proxy
	}
	if proxy != nil {
		// Only these named, non-closure resolvers are recognized; this does
		// not attempt to compare arbitrary callback behavior.
		p := reflect.ValueOf(proxy).Pointer()
		if p != reflect.ValueOf(http.ProxyFromEnvironment).Pointer() && p != reflect.ValueOf(noProxyResolver).Pointer() {
			return fmt.Errorf("%w: proxy resolver", ErrSSRFUnsafeTransport)
		}
	}
	return nil
}

const (
	ssrfDialTimeout   = 30 * time.Second
	ssrfDialKeepAlive = 30 * time.Second
)

// newSSRFSafeTransport clones the base transport and sets a DialContext that
// rejects connections to private/local IP addresses. If allowedPrefixes is
// non-empty, those ranges are exempted from blocking.
func newSSRFSafeTransport(base *http.Transport, allowedPrefixes []netip.Prefix) *http.Transport {
	t := base.Clone()
	// A proxy resolves/dials the target outside our guarded dialer. The only
	// accepted resolvers are environment/no-proxy, both disabled here.
	t.Proxy = nil

	dialer := &net.Dialer{
		Timeout:   ssrfDialTimeout,
		KeepAlive: ssrfDialKeepAlive,
		Control:   ssrfControl(allowedPrefixes),
	}

	t.DialContext = dialer.DialContext
	return t
}

// ssrfControl returns a syscall.RawConn control function that inspects the
// resolved address and rejects connections to private/local IPs.
func ssrfControl(allowedPrefixes []netip.Prefix) func(network, address string, c syscall.RawConn) error {
	const mappedIPv4PrefixBits = 96
	allowedPrefixes = slices.Clone(allowedPrefixes)
	for i, prefix := range allowedPrefixes {
		if prefix.Addr().Is4In6() && prefix.Bits() >= mappedIPv4PrefixBits {
			allowedPrefixes[i] = netip.PrefixFrom(prefix.Addr().Unmap(), prefix.Bits()-mappedIPv4PrefixBits)
		}
	}
	return func(_, address string, _ syscall.RawConn) error {
		host, _, err := net.SplitHostPort(address)
		if err != nil {
			return err
		}

		ip, err := netip.ParseAddr(host)
		if err != nil {
			return err
		}
		// An IPv4-mapped IPv6 address (::ffff:a.b.c.d) reaches the IPv4 host,
		// so match it against the IPv4 ranges.
		// Zones select an interface, not a different destination network.
		originalIP := ip.WithZone("")
		ip = originalIP.Unmap()

		// Loopback (127.0.0.0/8, ::1) is blocked like every other private or
		// local range: local admin and debug endpoints are a classic SSRF
		// target. Development and httptest-based callers exempt it explicitly
		// with WithSSRFAllowedCIDRs.
		if clientip.IsPrivate(ip) && !clientip.InList(ip, allowedPrefixes) && !clientip.InList(originalIP, allowedPrefixes) {
			return &SSRFError{
				Host: host,
				IP:   ip.String(),
			}
		}

		return nil
	}
}
