// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package proxydial

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"time"

	xproxy "golang.org/x/net/proxy"
)

// DialContextFunc is the standard net.Dialer signature.
type DialContextFunc = func(ctx context.Context, network, addr string) (net.Conn, error)

// Default TCP-level options for the dialer used to reach the proxy.
// These mirror the values the HTTP client uses for its pooled
// transport, so consumers wiring [HTTPConnect] / [SOCKS5Dialer] get
// the same timeout/keep-alive defaults whether they go through the
// HTTP or gRPC client.
const (
	DefaultDialTimeout   = 30 * time.Second
	DefaultDialKeepAlive = 30 * time.Second
)

// DefaultDialer returns a *net.Dialer pre-configured with
// [DefaultDialTimeout] and [DefaultDialKeepAlive]. Use it as the
// dialer argument to [HTTPConnect] and [SOCKS5Dialer] unless callers
// have a specific reason to override the timeouts.
func DefaultDialer() *net.Dialer {
	return &net.Dialer{
		Timeout:   DefaultDialTimeout,
		KeepAlive: DefaultDialKeepAlive,
	}
}

// HTTPConnect opens TCP+TLS to proxyURL and sends an HTTP CONNECT
// request for addr (RFC 7231 §4.3.6), returning the tunneled
// net.Conn. tlsCfg is consulted only when proxyURL.Scheme is "https"
// — for plain "http" it is ignored. The supplied dialer controls
// TCP-level options (timeout, keep-alive); pass &net.Dialer{} for
// stdlib defaults.
//
// Caller's tlsCfg is cloned via [TLSConfig] before use; user input
// is never mutated.
func HTTPConnect(ctx context.Context, dialer *net.Dialer, proxyURL *url.URL, tlsCfg *tls.Config, addr string) (net.Conn, error) {
	conn, err := dialer.DialContext(ctx, "tcp", proxyURL.Host)
	if err != nil {
		return nil, fmt.Errorf("dial proxy %s: %w", proxyURL.Host, err)
	}
	if proxyURL.Scheme == "https" {
		tlsConn := tls.Client(conn, TLSConfig(proxyURL, tlsCfg))
		if err = tlsConn.HandshakeContext(ctx); err != nil {
			_ = conn.Close()
			return nil, fmt.Errorf("tls handshake to proxy %s: %w", proxyURL.Host, err)
		}
		conn = tlsConn
	}
	if err = sendConnect(ctx, conn, addr, proxyURL.User); err != nil {
		_ = conn.Close()
		return nil, err
	}
	return conn, nil
}

// SOCKS5Dialer constructs a SOCKS5 dialer for proxyURL and returns a
// per-connection dial function. dialer controls TCP options for the
// connection to the SOCKS proxy itself. The returned func always
// dials over TCP — SOCKS5 supports only TCP for the inner conn.
//
// SOCKS does not TLS-wrap the proxy connection; callers passing a
// proxy *tls.Config alongside SOCKS should be aware that it has no
// effect here.
//
// Note: golang.org/x/net/proxy.SOCKS5 always sends ATYP=DomainName
// for hostname addresses, performing DNS on the proxy side. The
// "socks5" and "socks5h" URL schemes are therefore treated as
// synonyms by callers of this dialer — there is no client-side DNS
// resolution mode. Callers needing client-side DNS must resolve to
// an IP literal before passing addr.
func SOCKS5Dialer(proxyURL *url.URL, dialer *net.Dialer) (DialContextFunc, error) {
	var auth *xproxy.Auth
	if u := proxyURL.User; u != nil {
		pwd, _ := u.Password()
		auth = &xproxy.Auth{User: u.Username(), Password: pwd}
	}
	d, err := xproxy.SOCKS5("tcp", proxyURL.Host, auth, dialer)
	if err != nil {
		return nil, fmt.Errorf("build socks5 dialer for %s: %w", proxyURL.Host, err)
	}
	contextDialer, ok := d.(xproxy.ContextDialer)
	if !ok {
		return nil, fmt.Errorf("socks5 dialer for %s does not implement ContextDialer", proxyURL.Host)
	}
	return func(ctx context.Context, _, addr string) (net.Conn, error) {
		return contextDialer.DialContext(ctx, "tcp", addr)
	}, nil
}

// TLSConfig returns the *tls.Config to use for the TLS handshake to
// an https:// proxy. A nil user produces the safe default
// (ServerName=proxyURL.Hostname(), MinVersion=TLS12). A non-nil user
// is cloned, then ServerName and MinVersion are filled in only if
// the caller left them at their zero values — every other field
// (RootCAs, Certificates, InsecureSkipVerify, …) is preserved
// verbatim. The caller's *tls.Config is never mutated.
func TLSConfig(proxyURL *url.URL, user *tls.Config) *tls.Config {
	if user == nil {
		return &tls.Config{
			ServerName: proxyURL.Hostname(),
			MinVersion: tls.VersionTLS12,
		}
	}
	cfg := user.Clone()
	if cfg.ServerName == "" {
		cfg.ServerName = proxyURL.Hostname()
	}
	if cfg.MinVersion == 0 {
		cfg.MinVersion = tls.VersionTLS12
	}
	return cfg
}

// sendConnect writes a CONNECT request to conn and validates the
// response, returning a non-2xx status as an error. Mirrors stdlib
// net/http.Transport's CONNECT handling and refuses to return the
// raw conn when bufio buffered extra bytes after the response —
// those bytes would be silently lost when the caller starts using
// the tunnel.
func sendConnect(ctx context.Context, conn net.Conn, addr string, auth *url.Userinfo) error {
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
		defer func() { _ = conn.SetDeadline(time.Time{}) }()
	}

	req := &http.Request{
		Method: http.MethodConnect,
		URL:    &url.URL{Opaque: addr},
		Host:   addr,
		Header: make(http.Header),
	}
	if auth != nil {
		req.Header.Set("Proxy-Authorization", basicAuthHeader(auth))
	}

	if err := req.Write(conn); err != nil {
		return fmt.Errorf("write CONNECT to %s: %w", addr, err)
	}

	br := bufio.NewReader(conn)
	resp, err := http.ReadResponse(br, req)
	if err != nil {
		return fmt.Errorf("read CONNECT response from %s: %w", addr, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("proxy CONNECT to %s: %s", addr, resp.Status)
	}
	if br.Buffered() > 0 {
		return fmt.Errorf("proxy CONNECT to %s: %d unexpected bytes after response", addr, br.Buffered())
	}
	return nil
}

// basicAuthHeader formats *url.Userinfo as a "Basic <base64>" value
// suitable for the Proxy-Authorization header.
func basicAuthHeader(u *url.Userinfo) string {
	user := u.Username()
	pwd, _ := u.Password()
	creds := base64.StdEncoding.EncodeToString([]byte(user + ":" + pwd))
	return "Basic " + creds
}
