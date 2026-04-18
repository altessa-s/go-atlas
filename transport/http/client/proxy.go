// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package client

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"net/url"

	"github.com/altessa-s/go-atlas/transport/internal/proxydial"
)

// proxyAwareDialer returns a *http.Transport-compatible DialContext
// that resolves the proxy via proxyFn for each new connection and
// tunnels through it. The proxy scheme determines the wire flow:
//
//   - http://         — plain TCP + HTTP CONNECT
//   - https://        — TCP + TLS-with-proxyTLSCfg + HTTP CONNECT
//   - socks5/socks5h  — golang.org/x/net/proxy.SOCKS5
//   - nil URL         — direct dial (proxyFn opted out for this request)
//
// proxyTLSCfg is consulted only for the https scheme. SOCKS dialers
// silently ignore it because the SOCKS handshake is not TLS; callers
// using SOCKS should leave proxyTLSCfg unset and let stdlib handle
// the proxy via [http.Transport.Proxy].
func proxyAwareDialer(proxyFn ProxyFunc, proxyTLSCfg *tls.Config) func(context.Context, string, string) (net.Conn, error) {
	base := proxydial.DefaultDialer()
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		// Synthesize a destination URL for the resolver. http.ProxyURL
		// and http.ProxyFromEnvironment both decide off the host, so
		// the placeholder scheme is harmless for the typical
		// gated-https case; it only narrows the path when callers
		// wired a custom ProxyFunc that branches on req.URL.Scheme.
		req := &http.Request{URL: &url.URL{Scheme: "https", Host: addr}}
		proxyURL, err := proxyFn(req)
		if err != nil {
			return nil, fmt.Errorf("resolve proxy for %s: %w", addr, err)
		}
		if proxyURL == nil {
			return base.DialContext(ctx, network, addr)
		}
		switch proxyURL.Scheme {
		case "http", "https":
			return proxydial.HTTPConnect(ctx, base, proxyURL, proxyTLSCfg, addr)
		case "socks5", "socks5h":
			contextDial, err := proxydial.SOCKS5Dialer(proxyURL, base)
			if err != nil {
				return nil, err
			}
			return contextDial(ctx, network, addr)
		default:
			return nil, fmt.Errorf("unsupported proxy scheme %q (allowed: http, https, socks5, socks5h)", proxyURL.Scheme)
		}
	}
}
