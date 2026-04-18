// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package proxydial holds the wire-protocol details for routing client
// connections through a forward proxy: HTTP CONNECT (with optional TLS
// to the proxy itself), SOCKS5, and the *tls.Config merge rules used
// for the TLS handshake to an https:// proxy.
//
// It exists so [transport/http/client] and [transport/grpc/client]
// share the same dialer logic — both clients let callers configure a
// proxy resolver plus an optional proxy-only *tls.Config, and the
// implementation of "open TCP, optionally wrap with TLS using
// ServerName from the URL hostname, then send CONNECT" is identical
// regardless of the host transport.
//
// Each consumer wraps the helpers in this package with its own
// signature:
//
//   - [grpc.WithContextDialer] expects (ctx, addr) — gRPC client wraps
//     [HTTPConnect] / [SOCKS5Dialer] in a 2-arg closure.
//   - [http.Transport.DialContext] expects (ctx, network, addr) — HTTP
//     client wraps the same helpers in a 3-arg closure that resolves
//     the proxy URL via [http.Transport.Proxy] semantics first.
//
// # Out of scope
//
//   - End-to-end testing against a real HTTPS proxy with custom CA —
//     consumers cover that at their own integration layer.
//   - Proxy auto-detection (PAC, WPAD) — callers supply a static URL or
//     a [http.ProxyFromEnvironment]-style resolver.
package proxydial
