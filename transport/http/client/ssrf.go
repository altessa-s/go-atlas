// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package client

import (
	"net"
	"net/http"
	"net/netip"
	"syscall"
	"time"

	"github.com/altessa-s/go-atlas/transport/internal/clientip"
)

const (
	ssrfDialTimeout   = 30 * time.Second
	ssrfDialKeepAlive = 30 * time.Second
)

// newSSRFSafeTransport clones the base transport and sets a DialContext that
// rejects connections to private/local IP addresses. If allowedPrefixes is
// non-empty, those ranges are exempted from blocking.
func newSSRFSafeTransport(base *http.Transport, allowedPrefixes []netip.Prefix) *http.Transport {
	t := base.Clone()

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
	return func(_, address string, _ syscall.RawConn) error {
		host, _, err := net.SplitHostPort(address)
		if err != nil {
			return err
		}

		ip, err := netip.ParseAddr(host)
		if err != nil {
			return err
		}

		// Loopback (127.0.0.0/8, ::1) is exempt by default. Blocking it would
		// break local development and httptest-based callers, and loopback SSRF
		// is a far rarer vector than access to internal networks or cloud
		// metadata — which remain blocked (RFC1918, link-local, ULA, etc.).
		if clientip.IsPrivate(ip) && !ip.IsLoopback() && !clientip.InList(ip, allowedPrefixes) {
			return &SSRFError{
				Host: host,
				IP:   ip.String(),
			}
		}

		return nil
	}
}
