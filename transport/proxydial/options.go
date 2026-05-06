// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package proxydial

//go:generate go run github.com/altessa-s/go-atlas/tools/codegen/optgen generate --type=options

import (
	"crypto/tls"
	"net"
)

// options holds the internal configuration state consumed by [FromURL].
// Configure through the generated [Option] values; the struct is never
// exposed to callers.
type options struct {
	// dialer overrides the underlying *net.Dialer used to reach the
	// proxy. Defaults to [DefaultDialer]. Generated WithDialer skips
	// nil arguments so callers can pass through configuration values
	// unconditionally.
	dialer *net.Dialer `optgen:"default=DefaultDialer()"`

	// proxyTLSConfig is the *tls.Config used for the TLS handshake to
	// an https:// proxy. The caller's config is cloned via [TLSConfig]
	// before use; it is never mutated. ServerName and MinVersion are
	// filled in from the proxy URL only when left at the zero value.
	// No effect for http:// or socks5:// proxies.
	proxyTLSConfig *tls.Config
}
