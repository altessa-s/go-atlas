// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"net"
	"net/url"
	"strconv"

	"github.com/altessa-s/go-atlas/config"
	"github.com/altessa-s/go-atlas/transport/proxydial"

	corefactory "github.com/altessa-s/go-atlas/core/factory"
)

// DialerBuilder assembles a [proxydial.DialContextFunc] from a
// [config.HTTPProxy] using a fluent API. Create instances with [New].
// Errors from fluent methods are accumulated and reported at
// [DialerBuilder.Build] time. The builder is not safe for concurrent
// use.
type DialerBuilder struct {
	corefactory.Base
	cfg  *config.HTTPProxy
	errs []error

	// Dependencies
	dialer         *net.Dialer
	proxyTLSConfig *tls.Config
}

// New creates a [DialerBuilder] for the given proxy configuration.
//
// A nil cfg is allowed and yields (nil, nil) at [DialerBuilder.Build]
// time — this matches the "proxy is opt-in" contract used elsewhere
// in the SDK and lets callers wire the builder unconditionally
// without first checking whether the YAML block was present.
func New(cfg *config.HTTPProxy) *DialerBuilder {
	return &DialerBuilder{
		Base: corefactory.NewBase(slog.New(slog.DiscardHandler)),
		cfg:  cfg,
	}
}

// Build materializes the proxy configuration into a
// [proxydial.DialContextFunc].
//
// Returns (nil, nil) when proxying is disabled (nil receiver, nil
// cfg, empty Mode, or [config.HTTPProxyModeNone]). Caller treats
// that as "use a direct dial" and skips wiring a custom dialer.
//
// The ctx is reserved for future use (DNS lookup of the proxy host,
// dependency probes); current implementation does not consult it.
func (b *DialerBuilder) Build(_ context.Context) (proxydial.DialContextFunc, error) {
	if b == nil {
		return nil, nil //nolint:nilnil
	}
	if err := corefactory.JoinErrors(b.errs); err != nil {
		return nil, err
	}
	if b.cfg == nil {
		return nil, nil //nolint:nilnil
	}

	u, err := b.proxyURL()
	if err != nil || u == nil {
		return nil, err
	}

	return proxydial.FromURL(u, b.dialOptions()...)
}

// proxyURL folds Mode/URL/Host/Port/Auth into a canonical *url.URL.
// Returns (nil, nil) for the passthrough modes.
func (b *DialerBuilder) proxyURL() (*url.URL, error) {
	switch b.cfg.Mode {
	case "", config.HTTPProxyModeNone:
		return nil, nil //nolint:nilnil

	case config.HTTPProxyModeURL:
		u, err := url.Parse(b.cfg.URL)
		if err != nil {
			return nil, fmt.Errorf("HTTPProxy: parse url: %w", err)
		}
		return u, nil

	case config.HTTPProxyModeHost:
		return &url.URL{
			Scheme: "http",
			Host:   net.JoinHostPort(b.cfg.Host, strconv.Itoa(b.cfg.Port)),
			User:   userinfo(b.cfg.Auth),
		}, nil

	default:
		return nil, fmt.Errorf("HTTPProxy: unknown mode %q", b.cfg.Mode)
	}
}

// dialOptions assembles the [proxydial.Option] slice from the
// dependencies stored on the builder. Only non-nil fields produce
// options, so default proxydial behavior is preserved when callers
// skip the corresponding Use* method.
func (b *DialerBuilder) dialOptions() []proxydial.Option {
	var opts []proxydial.Option
	if b.dialer != nil {
		opts = append(opts, proxydial.WithDialer(b.dialer))
	}
	if b.proxyTLSConfig != nil {
		opts = append(opts, proxydial.WithProxyTLSConfig(b.proxyTLSConfig))
	}
	return opts
}

// userinfo builds a *url.Userinfo from an Auth block, returning nil
// for anonymous proxies. The plain-text password has to be exposed
// here — the underlying URL needs it on the wire.
func userinfo(a *config.HTTPProxyAuth) *url.Userinfo {
	if a == nil || a.Username == "" {
		return nil
	}
	if a.Password.IsEmpty() {
		return url.User(a.Username)
	}
	return url.UserPassword(a.Username, a.Password.Expose())
}
