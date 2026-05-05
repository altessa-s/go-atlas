// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"crypto/tls"
	"log/slog"
	"net"
)

// UseLogger sets the logger for the builder. Currently informational
// (the dialer itself does not log), but kept for symmetry with the
// rest of go-atlas factories — wiring once at construction time
// avoids touchups when the dialer eventually grows logging.
func (b *DialerBuilder) UseLogger(v *slog.Logger) *DialerBuilder {
	b.SetLogger(v)
	return b
}

// UseDefaultLogger sets the logger to [slog.Default].
func (b *DialerBuilder) UseDefaultLogger() *DialerBuilder {
	return b.UseLogger(slog.Default())
}

// UseDialer overrides the underlying *net.Dialer used to reach the
// proxy. Pass when callers need custom TCP timeouts, keep-alive
// intervals, or a control function. Defaults to
// [proxydial.DefaultDialer].
func (b *DialerBuilder) UseDialer(v *net.Dialer) *DialerBuilder {
	b.dialer = v
	return b
}

// UseProxyTLSConfig sets the *tls.Config used for the TLS handshake
// to an https:// proxy. The caller's config is cloned before use; it
// is never mutated. ServerName and MinVersion are filled in from the
// proxy URL only when left at the zero value.
//
// No effect for http:// or socks5:// proxies.
func (b *DialerBuilder) UseProxyTLSConfig(v *tls.Config) *DialerBuilder {
	b.proxyTLSConfig = v
	return b
}
