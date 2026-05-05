// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package proxydial provides forward-proxy dialers for client
// connections that do not go through net/http: SMTP, SOAP, raw TCP,
// gRPC, and any other protocol that needs to tunnel through a
// corporate or egress proxy.
//
// # Layered API
//
//   - [transport/proxydial/factory] — high-level: a fluent builder
//     that turns a [config.HTTPProxy] into a [DialContextFunc].
//     The intended entry point for service code that already loads
//     HTTPProxy from YAML/env.
//
//   - [FromURL] — middle-level: build a [DialContextFunc] from a
//     parsed *url.URL. Use when the proxy comes from somewhere other
//     than HTTPProxy (env, runtime override, custom resolver).
//
//   - [HTTPConnect] / [SOCKS5Dialer] — low-level escape hatches: open
//     a single tunneled connection. Use when you control the
//     lifecycle yourself (one-shot persistent connection, custom
//     retry policy, custom TLS handshake).
//
// # Wire flow by scheme
//
//   - http://         — plain TCP + HTTP CONNECT
//   - https://        — TCP + TLS-to-proxy + HTTP CONNECT
//   - socks5/socks5h  — golang.org/x/net/proxy.SOCKS5 (TCP, no proxy TLS)
//
// # Why this exists
//
// [transport/http/client] and [transport/grpc/client] each accept a
// proxy configuration via their option pattern (`WithProxyURL`,
// `WithProxy`, `WithProxyTLSConfig`). Both internally delegate the
// "open TCP, optionally wrap with TLS, send CONNECT" wire flow to
// this package. Public consumers that need raw-TCP-through-proxy
// (SMTP, SOAP, etc.) use [transport/proxydial/factory] /
// [FromURL] directly.
//
// # Out of scope
//
//   - End-to-end testing against a real HTTPS proxy with custom CA —
//     consumers cover that at their own integration layer.
//   - Proxy auto-detection (PAC, WPAD) — callers supply a static URL
//     or a [http.ProxyFromEnvironment]-style resolver.
package proxydial
