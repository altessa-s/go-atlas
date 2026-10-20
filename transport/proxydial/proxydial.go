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
	"sync"
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
//
// Bytes the target sent right behind the 200 response (the greeting of
// a server-first protocol such as SMTP, FTP, POP3 or SSH) are kept and
// returned by the first reads on the tunnel.
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
	tunnel, err := sendConnect(ctx, conn, addr, proxyURL.User)
	if err != nil {
		_ = conn.Close()
		return nil, err
	}
	return tunnel, nil
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
// response, returning a non-2xx status as an error. A 200 response has
// no content (RFC 9110 §9.3.6): its Content-Length and
// Transfer-Encoding are ignored and every byte after the header block
// belongs to the tunnel. When bufio already holds such bytes, the
// returned conn serves them before reading from conn; otherwise conn
// itself is returned. Canceling ctx interrupts the handshake I/O, and
// the returned error then wraps ctx.Err().
func sendConnect(ctx context.Context, conn net.Conn, addr string, auth *url.Userinfo) (_ net.Conn, err error) {
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	}
	// A deadline in the past unblocks the pending Write/Read on cancellation.
	interrupted := make(chan struct{})
	stop := context.AfterFunc(ctx, func() {
		_ = conn.SetDeadline(time.Unix(1, 0))
		close(interrupted)
	})
	defer func() {
		if !stop() {
			// Wait for the callback so it cannot poison the deadline after the
			// reset below; the conn is unusable either way, so report ctx.
			<-interrupted
			err = fmt.Errorf("CONNECT to %s: %w", addr, ctx.Err())
		}
		_ = conn.SetDeadline(time.Time{})
	}()

	req := &http.Request{
		Method: http.MethodConnect,
		URL:    &url.URL{Opaque: addr},
		Host:   addr,
		Header: make(http.Header),
	}
	if auth != nil {
		req.Header.Set("Proxy-Authorization", basicAuthHeader(auth))
	}

	if err = req.Write(conn); err != nil {
		return nil, fmt.Errorf("write CONNECT to %s: %w", addr, err)
	}

	br := bufio.NewReader(conn)
	resp, err := http.ReadResponse(br, req)
	if err != nil {
		return nil, fmt.Errorf("read CONNECT response from %s: %w", addr, err)
	}
	if resp.StatusCode != http.StatusOK {
		_ = resp.Body.Close()
		return nil, fmt.Errorf("proxy CONNECT to %s: %s", addr, resp.Status)
	}
	// The body of a 200 is never read or closed: closing it would consume
	// tunnel bytes as Content-Length or chunked content.
	return newBufferedConn(conn, br), nil
}

// newBufferedConn returns conn unchanged when br holds no bytes, and
// otherwise a wrapper that serves the buffered bytes first. The wrapper
// keeps CloseWrite (and CloseRead) when conn has them, so half-close
// works on the tunnel as on the bare connection.
func newBufferedConn(conn net.Conn, br *bufio.Reader) net.Conn {
	n := br.Buffered()
	if n == 0 {
		return conn
	}
	peeked, _ := br.Peek(n) // n bytes are buffered: Peek cannot fail
	bc := &bufferedConn{Conn: conn, pending: append([]byte(nil), peeked...)}
	cw, ok := conn.(closeWriter)
	if !ok {
		return bc
	}
	if cr, ok := conn.(closeReader); ok {
		return &bufferedHalfCloseConn{bufferedConn: bc, cw: cw, cr: cr}
	}
	return &bufferedCloseWriteConn{bufferedConn: bc, cw: cw}
}

type closeWriter interface{ CloseWrite() error }

type closeReader interface{ CloseRead() error }

// bufferedConn is a net.Conn whose first reads drain bytes buffered
// while parsing the CONNECT response; every other method delegates.
type bufferedConn struct {
	net.Conn

	mu      sync.Mutex
	pending []byte
}

// Read returns buffered bytes first, then reads from the connection.
func (c *bufferedConn) Read(p []byte) (int, error) {
	c.mu.Lock()
	if len(c.pending) > 0 {
		n := copy(p, c.pending)
		c.pending = c.pending[n:]
		if len(c.pending) == 0 {
			c.pending = nil
		}
		c.mu.Unlock()
		return n, nil
	}
	c.mu.Unlock()
	return c.Conn.Read(p)
}

// bufferedCloseWriteConn is a bufferedConn over a connection that
// supports CloseWrite (e.g. *tls.Conn).
type bufferedCloseWriteConn struct {
	*bufferedConn

	cw closeWriter
}

// CloseWrite shuts down the writing side of the connection.
func (c *bufferedCloseWriteConn) CloseWrite() error { return c.cw.CloseWrite() }

// bufferedHalfCloseConn is a bufferedConn over a connection that
// supports both CloseWrite and CloseRead (e.g. *net.TCPConn).
type bufferedHalfCloseConn struct {
	*bufferedConn

	cw closeWriter
	cr closeReader
}

// CloseWrite shuts down the writing side of the connection.
func (c *bufferedHalfCloseConn) CloseWrite() error { return c.cw.CloseWrite() }

// CloseRead shuts down the reading side of the connection.
func (c *bufferedHalfCloseConn) CloseRead() error { return c.cr.CloseRead() }

// basicAuthHeader formats *url.Userinfo as a "Basic <base64>" value
// suitable for the Proxy-Authorization header.
func basicAuthHeader(u *url.Userinfo) string {
	user := u.Username()
	pwd, _ := u.Password()
	creds := base64.StdEncoding.EncodeToString([]byte(user + ":" + pwd))
	return "Basic " + creds
}
