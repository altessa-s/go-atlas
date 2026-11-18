// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"crypto/tls"
	"log/slog"
	"net"

	"github.com/altessa-s/go-atlas-net/proxydial"

	coreslices "github.com/altessa-s/go-atlas-core/collections/slices"
	proxyconfig "github.com/altessa-s/go-atlas/config/proxy"
	corefactory "github.com/altessa-s/go-atlas/factory/core"
)

// DialerBuilder assembles a [proxydial.DialContextFunc] from a
// [proxyconfig.Config] using a fluent API. Create instances with [New].
// The builder is not safe for concurrent use.
type DialerBuilder struct {
	corefactory.Base
	cfg *proxyconfig.Config

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
func New(cfg *proxyconfig.Config) *DialerBuilder {
	return &DialerBuilder{
		Base: corefactory.NewBase(slog.New(slog.DiscardHandler)),
		cfg:  cfg,
	}
}

// Build materializes the proxy configuration into a
// [proxydial.DialContextFunc].
//
// Returns (nil, nil) when proxying is disabled (nil receiver, nil
// cfg, empty Mode, or explicit none mode). Caller treats
// that as "use a direct dial" and skips wiring a custom dialer.
//
// The configuration is validated first ([proxyconfig.Config.Validate]),
// so a malformed URL or one whose credentials spill into the host or path
// is rejected with an error that quotes no part of it, before any dial or
// DNS lookup. A validated URL's host cannot carry credentials, which is
// why proxydial dial errors may name the proxy host:port.
func (b *DialerBuilder) Build() (proxydial.DialContextFunc, error) {
	if b == nil || b.cfg == nil {
		return nil, nil //nolint:nilnil
	}
	u, err := b.cfg.URLValue()
	if err != nil || u == nil {
		return nil, err
	}

	return proxydial.FromURL(u, b.dialOptions()...)
}

// dialOptions assembles the [proxydial.Option] slice from the
// dependencies stored on the builder. Only non-nil fields produce
// options, so default proxydial behavior is preserved when callers
// skip the corresponding Use* method.
func (b *DialerBuilder) dialOptions() []proxydial.Option {
	var opts []proxydial.Option
	opts = coreslices.AppendIf(opts, b.dialer != nil, proxydial.WithDialer(b.dialer))
	opts = coreslices.AppendIf(opts, b.proxyTLSConfig != nil, proxydial.WithProxyTLSConfig(b.proxyTLSConfig))
	return opts
}
