// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package client

import (
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"reflect"
	"slices"
	"syscall"
	"time"

	"github.com/altessa-s/go-atlas/transport/clientip"
)

// validateSSRFTransport rejects configurations whose dialing cannot be guarded.
// It never invokes caller proxy callbacks while deciding whether they are safe.
// An explicit proxy option (WithProxy, WithProxyURL, WithProxyFunc, WithoutProxy)
// is accepted: the dial to the proxy goes through the guarded dialer. A proxy
// resolver set on a caller-supplied transport is not, since it is not known to
// be intended; use WithProxyFunc instead.
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
		// The proxy TLS dialer connects to the proxy outside the guarded dialer.
		return fmt.Errorf("%w: proxy TLS configuration", ErrSSRFUnsafeTransport)
	}
	if opts.proxy == nil && t.Proxy != nil {
		// Only these named, non-closure resolvers are recognized; this does
		// not attempt to compare arbitrary callback behavior.
		p := reflect.ValueOf(t.Proxy).Pointer()
		if p != reflect.ValueOf(http.ProxyFromEnvironment).Pointer() && p != reflect.ValueOf(noProxyResolver).Pointer() {
			return fmt.Errorf("%w: proxy resolver on the transport", ErrSSRFUnsafeTransport)
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
//
// proxy is the explicit proxy resolver, or nil. Environment proxies are
// disabled either way. With a proxy, the guarded dialer connects to the proxy
// (its address is checked like any destination) and the proxy resolves and
// dials the target host; a target given as a literal IP address is still
// checked before the request is handed to the proxy.
func newSSRFSafeTransport(base *http.Transport, allowedPrefixes []netip.Prefix, proxy ProxyFunc) *http.Transport {
	t := base.Clone()
	t.Proxy = nil
	if proxy != nil {
		t.Proxy = guardedProxy(proxy, allowedPrefixes)
	}

	dialer := &net.Dialer{
		Timeout:   ssrfDialTimeout,
		KeepAlive: ssrfDialKeepAlive,
		Control:   SSRFDialControl(allowedPrefixes...),
	}

	t.DialContext = dialer.DialContext
	return t
}

// guardedProxy wraps proxy so a request whose target host is a literal IP
// address in a blocked range fails with an [SSRFError] instead of being sent
// through the proxy. Host names are resolved by the proxy and cannot be
// checked here.
func guardedProxy(proxy ProxyFunc, allowedPrefixes []netip.Prefix) ProxyFunc {
	check := SSRFDialControl(allowedPrefixes...)
	return func(req *http.Request) (*url.URL, error) {
		u, err := proxy(req)
		if err != nil || u == nil {
			return u, err
		}
		if ip, perr := netip.ParseAddr(req.URL.Hostname()); perr == nil {
			if cerr := check("tcp", net.JoinHostPort(ip.String(), "0"), nil); cerr != nil {
				return nil, cerr
			}
		}
		return u, nil
	}
}

// SSRFDialControl returns a [net.Dialer.Control] function that rejects a
// connection whose resolved address is private, loopback, link-local or
// otherwise reserved ([clientip.IsPrivate]) with an [*SSRFError] (matching
// [ErrSSRFBlocked]). It runs after DNS resolution and before the connect, so
// it also closes DNS rebinding. IPv4-mapped IPv6 addresses are checked as
// IPv4 and zones are ignored. allowed exempts ranges (an IPv4-mapped prefix
// matches the IPv4 range too). An address that is not "ip:port" is rejected
// with the parse error. It is the guard the client installs with SSRF
// protection on; services with their own transport install it in their
// dialer.
func SSRFDialControl(allowed ...netip.Prefix) func(network, address string, c syscall.RawConn) error {
	const mappedIPv4PrefixBits = 96
	allowedPrefixes := slices.Clone(allowed)
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
