// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package proxydial

import (
	"bufio"
	"context"
	"crypto/tls"
	"crypto/x509"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestTLSConfig_NilUserAppliesSafeDefaults(t *testing.T) {
	t.Parallel()

	u, err := url.Parse("https://proxy.example.com:8443")
	require.NoError(t, err)

	got := TLSConfig(u, nil)
	require.NotNil(t, got)
	require.Equal(t, "proxy.example.com", got.ServerName)
	require.Equal(t, uint16(tls.VersionTLS12), got.MinVersion)
}

func TestTLSConfig_FillsMissingDefaults(t *testing.T) {
	t.Parallel()

	u, err := url.Parse("https://proxy.example.com:8443")
	require.NoError(t, err)

	pool := x509.NewCertPool()
	user := &tls.Config{RootCAs: pool} // ServerName="" and MinVersion=0 → defaults filled

	got := TLSConfig(u, user)
	require.NotSame(t, user, got, "user config must be cloned, not mutated")
	require.Equal(t, "proxy.example.com", got.ServerName)
	require.Equal(t, uint16(tls.VersionTLS12), got.MinVersion)
	require.Same(t, pool, got.RootCAs, "non-default fields must be preserved verbatim")

	// Original user value must remain untouched.
	require.Empty(t, user.ServerName)
	require.Zero(t, user.MinVersion)
}

func TestTLSConfig_PreservesUserOverrides(t *testing.T) {
	t.Parallel()

	u, err := url.Parse("https://proxy.example.com:8443")
	require.NoError(t, err)

	user := &tls.Config{ServerName: "alt.example", MinVersion: tls.VersionTLS13}
	got := TLSConfig(u, user)
	require.Equal(t, "alt.example", got.ServerName)
	require.Equal(t, uint16(tls.VersionTLS13), got.MinVersion)
}

func TestBasicAuthHeader(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		auth *url.Userinfo
		want string
	}{
		{"user_password", url.UserPassword("svc", "secret"), "Basic c3ZjOnNlY3JldA=="},
		{"username_only", url.User("svc"), "Basic c3ZjOg=="},
		{"empty", url.UserPassword("", ""), "Basic Og=="},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, basicAuthHeader(tc.auth))
		})
	}
}

// TestHTTPConnect_CancelInterruptsStalledHandshake is the regression guard for
// a CONNECT handshake that only honored the context deadline: with a
// deadline-free context, a proxy that reads the request and never answers left
// HTTPConnect blocked in ReadResponse even after the context was canceled.
func TestHTTPConnect_CancelInterruptsStalledHandshake(t *testing.T) {
	t.Parallel()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = ln.Close() })

	arrived := make(chan struct{})
	proxyClosed := make(chan error, 1)
	go func() {
		conn, acceptErr := ln.Accept()
		if acceptErr != nil {
			proxyClosed <- acceptErr
			return
		}
		defer func() { _ = conn.Close() }()

		br := bufio.NewReader(conn)
		if _, readErr := http.ReadRequest(br); readErr != nil {
			proxyClosed <- readErr
			return
		}
		close(arrived)

		// Never answer; wait for the client to give up and close.
		_, readErr := br.ReadByte()
		proxyClosed <- readErr
	}()

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	result := make(chan error, 1)
	go func() {
		conn, connectErr := HTTPConnect(ctx, &net.Dialer{}, &url.URL{Scheme: "http", Host: ln.Addr().String()}, nil, "target.example:443")
		if conn != nil {
			_ = conn.Close()
		}
		result <- connectErr
	}()

	select {
	case <-arrived:
	case err := <-proxyClosed:
		t.Fatalf("proxy failed before CONNECT arrived: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("CONNECT request never reached the proxy")
	}
	cancel()

	select {
	case err := <-result:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(5 * time.Second):
		t.Fatal("HTTPConnect did not return after the context was canceled")
	}

	select {
	case err := <-proxyClosed:
		require.ErrorIs(t, err, io.EOF, "the client must close the stalled proxy connection")
	case <-time.After(5 * time.Second):
		t.Fatal("proxy connection was not closed")
	}
}

// startConnectProxy starts a fake HTTP CONNECT proxy that accepts one
// connection, reads the CONNECT request, and hands the connection (with the
// reader that still holds any bytes the client sent after the request) to
// handle. The listener is closed and the handler awaited on cleanup.
func startConnectProxy(tb testing.TB, handle func(conn net.Conn, br *bufio.Reader)) *url.URL {
	tb.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(tb, err)

	done := make(chan struct{})
	go func() {
		defer close(done)
		conn, acceptErr := ln.Accept()
		if acceptErr != nil {
			return
		}
		defer func() { _ = conn.Close() }()

		br := bufio.NewReader(conn)
		if _, readErr := http.ReadRequest(br); readErr != nil {
			return
		}
		handle(conn, br)
	}()
	tb.Cleanup(func() {
		_ = ln.Close()
		<-done
	})
	return &url.URL{Scheme: "http", Host: ln.Addr().String()}
}

// serverFirstHandler answers CONNECT with head, sends greeting (in the same
// write unless split is set), then answers the client's "QUIT" line.
func serverFirstHandler(head, greeting string, split bool) func(net.Conn, *bufio.Reader) {
	return func(conn net.Conn, br *bufio.Reader) {
		if split {
			if _, err := io.WriteString(conn, head); err != nil {
				return
			}
			time.Sleep(20 * time.Millisecond)
			if _, err := io.WriteString(conn, greeting); err != nil {
				return
			}
		} else if _, err := io.WriteString(conn, head+greeting); err != nil {
			return
		}
		line, err := br.ReadString('\n')
		if err != nil || line != "QUIT\r\n" {
			return
		}
		_, _ = io.WriteString(conn, "221 bye\r\n")
	}
}

// requireRead reads exactly len(want) bytes from conn and compares them.
func requireRead(tb testing.TB, conn net.Conn, want string) {
	tb.Helper()

	got := make([]byte, len(want))
	_, err := io.ReadFull(conn, got)
	require.NoError(tb, err)
	require.Equal(tb, want, string(got))
}

// dialThrough runs HTTPConnect against proxyURL with a safety deadline on the
// returned tunnel.
func dialThrough(tb testing.TB, proxyURL *url.URL) net.Conn {
	tb.Helper()

	conn, err := HTTPConnect(tb.Context(), &net.Dialer{}, proxyURL, nil, "smtp.example:25")
	require.NoError(tb, err)
	trackTunnel(tb, conn)
	return conn
}

// trackTunnel closes conn on cleanup and arms a safety deadline on it.
func trackTunnel(tb testing.TB, conn net.Conn) {
	tb.Helper()

	tb.Cleanup(func() { _ = conn.Close() })
	require.NoError(tb, conn.SetDeadline(time.Now().Add(5*time.Second)))
}

// TestHTTPConnect_ServerFirstGreeting is the regression guard for protocols
// where the target speaks first (SMTP, FTP, POP3, SSH): the greeting may share
// a TCP segment with the 200, and a 2xx CONNECT response carries no content
// regardless of Content-Length or Transfer-Encoding (RFC 9110 §9.3.6).
func TestHTTPConnect_ServerFirstGreeting(t *testing.T) {
	t.Parallel()

	const greeting = "220 smtp.example ESMTP ready\r\n"
	cases := []struct {
		name     string
		head     string
		greeting string
		split    bool
	}{
		{"same_write", "HTTP/1.1 200 Connection established\r\n\r\n", greeting, false},
		{"content_length", "HTTP/1.1 200 OK\r\nContent-Length: 5\r\n\r\n", greeting, false},
		{"chunked", "HTTP/1.1 200 OK\r\nTransfer-Encoding: chunked\r\n\r\n", greeting, false},
		{"separate_write", "HTTP/1.1 200 Connection established\r\n\r\n", greeting, true},
		{"larger_than_buffer", "HTTP/1.1 200 Connection established\r\n\r\n", strings.Repeat("x", 64<<10) + "\r\n", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			proxyURL := startConnectProxy(t, serverFirstHandler(tc.head, tc.greeting, tc.split))
			conn := dialThrough(t, proxyURL)

			requireRead(t, conn, tc.greeting)
			_, err := io.WriteString(conn, "QUIT\r\n")
			require.NoError(t, err)
			requireRead(t, conn, "221 bye\r\n")
		})
	}
}

// TestHTTPConnect_ClientFirstReturnsBareConn pins the HTTPS-like path: with
// nothing buffered after the 200 the original connection is returned as is.
func TestHTTPConnect_ClientFirstReturnsBareConn(t *testing.T) {
	t.Parallel()

	proxyURL := startConnectProxy(t, func(conn net.Conn, br *bufio.Reader) {
		if _, err := io.WriteString(conn, "HTTP/1.1 200 Connection established\r\n\r\n"); err != nil {
			return
		}
		line, err := br.ReadString('\n')
		if err != nil {
			return
		}
		_, _ = io.WriteString(conn, "pong:"+line)
	})
	conn := dialThrough(t, proxyURL)
	require.IsType(t, &net.TCPConn{}, conn, "no buffered bytes: the tunnel must not be wrapped")

	_, err := io.WriteString(conn, "ping\r\n")
	require.NoError(t, err)
	requireRead(t, conn, "pong:ping\r\n")
}

// TestHTTPConnect_BufferedTunnelKeepsHalfClose verifies that the wrapper used
// when greeting bytes were buffered still exposes the TCP half-close methods.
func TestHTTPConnect_BufferedTunnelKeepsHalfClose(t *testing.T) {
	t.Parallel()

	const greeting = "SSH-2.0-OpenSSH_9.9\r\n"
	proxyURL := startConnectProxy(t, func(conn net.Conn, br *bufio.Reader) {
		if _, err := io.WriteString(conn, "HTTP/1.1 200 Connection established\r\n\r\n"+greeting); err != nil {
			return
		}
		if _, err := io.Copy(io.Discard, br); err != nil {
			return
		}
		_, _ = io.WriteString(conn, "bye")
	})
	conn := dialThrough(t, proxyURL)
	requireRead(t, conn, greeting)

	_, ok := conn.(interface{ CloseRead() error })
	require.True(t, ok, "CloseRead must survive the wrapper")
	cw, ok := conn.(interface{ CloseWrite() error })
	require.True(t, ok, "CloseWrite must survive the wrapper")
	require.NoError(t, cw.CloseWrite())

	rest, err := io.ReadAll(conn)
	require.NoError(t, err)
	require.Equal(t, "bye", string(rest), "the proxy must see EOF after CloseWrite and still answer")
}

// TestHTTPConnect_CancelAfterSuccessKeepsTunnel verifies that canceling the
// dial context after HTTPConnect returned does not poison the tunnel deadline.
func TestHTTPConnect_CancelAfterSuccessKeepsTunnel(t *testing.T) {
	t.Parallel()

	const greeting = "220 ftp.example ready\r\n"
	proxyURL := startConnectProxy(t, serverFirstHandler("HTTP/1.1 200 Connection established\r\n\r\n", greeting, false))

	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	conn, err := HTTPConnect(ctx, &net.Dialer{}, proxyURL, nil, "ftp.example:21")
	require.NoError(t, err)
	trackTunnel(t, conn)
	cancel()

	requireRead(t, conn, greeting)
	_, err = io.WriteString(conn, "QUIT\r\n")
	require.NoError(t, err)
	requireRead(t, conn, "221 bye\r\n")
}

// TestHTTPConnect_Non200Fails verifies that a refused CONNECT is an error and
// the proxy connection is closed.
func TestHTTPConnect_Non200Fails(t *testing.T) {
	t.Parallel()

	proxyClosed := make(chan error, 1)
	proxyURL := startConnectProxy(t, func(conn net.Conn, br *bufio.Reader) {
		if _, err := io.WriteString(conn, "HTTP/1.1 407 Proxy Authentication Required\r\nContent-Length: 4\r\n\r\ndeny"); err != nil {
			proxyClosed <- err
			return
		}
		_, err := br.ReadByte()
		proxyClosed <- err
	})

	conn, err := HTTPConnect(t.Context(), &net.Dialer{}, proxyURL, nil, "smtp.example:25")
	require.Error(t, err)
	require.Nil(t, conn)

	select {
	case err := <-proxyClosed:
		require.ErrorIs(t, err, io.EOF, "the client must close the refused proxy connection")
	case <-time.After(5 * time.Second):
		t.Fatal("proxy connection was not closed")
	}
}
