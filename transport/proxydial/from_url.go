// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package proxydial

import (
	"context"
	"fmt"
	"net"
	"net/url"
)

// FromURL builds a [DialContextFunc] that tunnels every dial through
// proxyURL. Dispatches on proxyURL.Scheme:
//
//   - http, https        — [HTTPConnect]
//   - socks5, socks5h    — [SOCKS5Dialer]
//
// Other schemes return an error so misconfiguration surfaces at
// dialer construction time rather than at the first request.
//
// A nil proxyURL returns (nil, nil) — caller should treat that as
// "use a direct dial" and skip wiring a custom dialer at all.
//
// Pass zero or more [Option] values (e.g. [WithDialer],
// [WithProxyTLSConfig]) to override defaults; options compose
// left-to-right and a later one overrides an earlier one.
//
// For config.Proxy-driven configuration prefer the
// [github.com/altessa-s/go-atlas/transport/proxydial/factory] builder,
// which folds Mode/Host/Port/Auth into the right URL before delegating
// here.
func FromURL(proxyURL *url.URL, opts ...Option) (DialContextFunc, error) {
	if proxyURL == nil {
		return nil, nil //nolint:nilnil
	}

	o := newOptions(opts...)

	switch proxyURL.Scheme {
	case "http", "https":
		dialer := o.dialer
		tlsCfg := o.proxyTLSConfig
		u := proxyURL
		return func(ctx context.Context, _, addr string) (net.Conn, error) {
			return HTTPConnect(ctx, dialer, u, tlsCfg, addr)
		}, nil

	case "socks5", "socks5h":
		return SOCKS5Dialer(proxyURL, o.dialer)

	default:
		return nil, fmt.Errorf("unsupported proxy scheme %q (allowed: http, https, socks5, socks5h)", proxyURL.Scheme)
	}
}
