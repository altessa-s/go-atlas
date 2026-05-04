// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package proxydial

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/url"
)

// Option configures [FromURL]. Pass zero or more to override the
// default dialer or supply a TLS config for https:// proxies.
// Options compose left-to-right; a later option overrides an earlier
// one.
type Option func(*options)

type options struct {
	dialer         *net.Dialer
	proxyTLSConfig *tls.Config
}

// WithDialer overrides the underlying *net.Dialer used to reach the
// proxy. Pass when callers need custom TCP timeouts, keep-alive
// intervals, or a control function. Defaults to [DefaultDialer].
func WithDialer(d *net.Dialer) Option {
	return func(o *options) {
		if d != nil {
			o.dialer = d
		}
	}
}

// WithProxyTLSConfig sets the *tls.Config used for the TLS handshake
// to an https:// proxy. Cloned via [TLSConfig] before use; the
// caller's config is never mutated. ServerName and MinVersion are
// filled in from the proxy URL only when left at the zero value.
//
// No effect for http:// or socks5:// proxies.
func WithProxyTLSConfig(cfg *tls.Config) Option {
	return func(o *options) {
		if cfg != nil {
			o.proxyTLSConfig = cfg
		}
	}
}

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
// For HTTPProxy-driven configuration prefer
// [config.HTTPProxy.DialContext] which folds Mode/Host/Port/Auth
// into the right URL before delegating here.
func FromURL(proxyURL *url.URL, opts ...Option) (DialContextFunc, error) {
	if proxyURL == nil {
		return nil, nil //nolint:nilnil
	}

	o := &options{dialer: DefaultDialer()}
	for _, opt := range opts {
		opt(o)
	}

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
