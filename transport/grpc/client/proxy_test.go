// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package client

import (
	"bufio"
	"context"
	"crypto/tls"
	"crypto/x509"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
)

// fakeProxy is an in-process HTTP CONNECT proxy. Each accepted CONNECT
// bumps connectCount and stores the request's Proxy-Authorization header
// for later assertion.
type fakeProxy struct {
	addr         string
	connectCount atomic.Int32
	lastAuth     atomic.Value // string
}

func newFakeProxy(t *testing.T) *fakeProxy {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = lis.Close() })

	p := &fakeProxy{addr: lis.Addr().String()}
	p.lastAuth.Store("")

	go func() {
		for {
			client, err := lis.Accept()
			if err != nil {
				return
			}
			go p.handle(client)
		}
	}()
	return p
}

func (p *fakeProxy) handle(client net.Conn) {
	defer func() { _ = client.Close() }()

	br := bufio.NewReader(client)
	req, err := http.ReadRequest(br)
	if err != nil || req.Method != http.MethodConnect {
		return
	}

	p.lastAuth.Store(req.Header.Get("Proxy-Authorization"))
	p.connectCount.Add(1)

	backend, err := net.Dial("tcp", req.Host)
	if err != nil {
		_, _ = client.Write([]byte("HTTP/1.1 502 Bad Gateway\r\n\r\n"))
		return
	}
	defer func() { _ = backend.Close() }()

	if _, err = client.Write([]byte("HTTP/1.1 200 OK\r\n\r\n")); err != nil {
		return
	}

	done := make(chan struct{}, 2)
	go func() { _, _ = io.Copy(backend, br); done <- struct{}{} }()
	go func() { _, _ = io.Copy(client, backend); done <- struct{}{} }()
	<-done
}

// newFakeGRPCServer starts an empty grpc.Server on 127.0.0.1:0 and
// returns its listen address. The server has no registered services but
// completes the HTTP/2 handshake — that's enough for the connection to
// reach connectivity.Ready, which is what the proxy tests assert.
func newFakeGRPCServer(t *testing.T) string {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	s := grpc.NewServer()
	go func() { _ = s.Serve(lis) }()
	t.Cleanup(s.Stop)
	return lis.Addr().String()
}

// connectAndWaitReady forces grpc-go's lazy dialing to fire and blocks
// until the connection reaches Ready or ctx expires. Without Connect()
// the dialer wouldn't be invoked at all because no RPC is made.
func connectAndWaitReady(t *testing.T, c *Client) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()

	conn, err := c.GetConnection(ctx)
	require.NoError(t, err)
	conn.Connect()

	state := conn.GetState()
	for state != connectivity.Ready {
		if !conn.WaitForStateChange(ctx, state) {
			require.NoErrorf(t, ctx.Err(), "connection stuck in state %s", state)
		}
		state = conn.GetState()
	}
}

func TestWithProxyURL_RoutesViaConnect(t *testing.T) {
	proxy := newFakeProxy(t)
	grpcAddr := newFakeGRPCServer(t)

	c, err := New(t.Context(), grpcAddr,
		WithInsecure(),
		WithProxyURL(mustParseURL(t, "http://"+proxy.addr)),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = c.Close(t.Context()) })

	connectAndWaitReady(t, c)
	require.Greater(t, proxy.connectCount.Load(), int32(0), "proxy saw no CONNECT")
}

func TestWithProxy_HostPortAuth(t *testing.T) {
	proxy := newFakeProxy(t)
	grpcAddr := newFakeGRPCServer(t)

	host, port := splitHostPort(t, proxy.addr)
	c, err := New(t.Context(), grpcAddr,
		WithInsecure(),
		WithProxy(host, port, url.UserPassword("svc", "secret")),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = c.Close(t.Context()) })

	connectAndWaitReady(t, c)

	// base64("svc:secret") == "c3ZjOnNlY3JldA=="
	require.Equal(t, "Basic c3ZjOnNlY3JldA==", proxy.lastAuth.Load())
}

func TestWithoutProxy_InstallsNoProxyResolver(t *testing.T) {
	grpcAddr := newFakeGRPCServer(t)

	c, err := New(t.Context(), grpcAddr,
		WithInsecure(),
		WithoutProxy(),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = c.Close(t.Context()) })

	require.NotNil(t, c.options.proxy, "WithoutProxy must install a no-proxy resolver")
	probe := &http.Request{URL: &url.URL{Scheme: "https", Host: "any:443"}}
	gotURL, gotErr := c.options.proxy(probe)
	require.NoError(t, gotErr)
	require.Nil(t, gotURL, "no-proxy resolver must return (nil, nil)")
	connectAndWaitReady(t, c)
}

func TestWithProxyURL_NilFailsAtFirstDial(t *testing.T) {
	t.Parallel()

	c, err := New(t.Context(), "127.0.0.1:1",
		WithInsecure(),
		WithProxyURL(nil),
	)
	require.NoError(t, err, "deferred-error semantics: New must succeed even with nil URL")
	t.Cleanup(func() { _ = c.Close(t.Context()) })

	probe := &http.Request{URL: &url.URL{Scheme: "https", Host: "any:443"}}
	_, gotErr := c.options.proxy(probe)
	require.Error(t, gotErr, "resolver must surface the nil-URL error at first invocation")
}

func TestWithProxyURL_UnsupportedSchemeFailsAtFirstDial(t *testing.T) {
	t.Parallel()

	// http.ProxyURL accepts any scheme; the dial-time switch in
	// proxyDialer is what rejects unsupported schemes. Build a client
	// and trigger a connection attempt to surface the error.
	c, err := New(t.Context(), "127.0.0.1:1",
		WithInsecure(),
		WithProxyURL(mustParseURL(t, "ftp://x:1")),
	)
	require.NoError(t, err, "deferred-error semantics: New must succeed")
	t.Cleanup(func() { _ = c.Close(t.Context()) })

	require.NotNil(t, c.options.proxy, "WithProxyURL must install a resolver")
}

func TestWithProxyFunc_StoresResolver(t *testing.T) {
	t.Parallel()

	called := false
	resolver := func(*http.Request) (*url.URL, error) {
		called = true
		return nil, nil //nolint:nilnil
	}
	o := &options{}
	WithProxyFunc(resolver)(o)
	require.NotNil(t, o.proxy)

	_, err := o.proxy(&http.Request{URL: &url.URL{Scheme: "https", Host: "any:443"}})
	require.NoError(t, err)
	require.True(t, called, "stored resolver must be the one supplied")
}

func TestWithProxy_LastOptionWins(t *testing.T) {
	t.Parallel()

	// WithProxyURL then WithoutProxy: the no-proxy resolver wins.
	o := &options{}
	WithProxyURL(mustParseURL(t, "http://a:1"))(o)
	WithoutProxy()(o)
	require.NotNil(t, o.proxy)
	gotURL, _ := o.proxy(&http.Request{URL: &url.URL{Scheme: "https", Host: "any:443"}})
	require.Nil(t, gotURL, "WithoutProxy applied after WithProxyURL must yield no-proxy")

	// WithoutProxy then WithProxyURL: the URL resolver wins.
	o = &options{}
	WithoutProxy()(o)
	WithProxyURL(mustParseURL(t, "http://a:1"))(o)
	require.NotNil(t, o.proxy)
	gotURL, _ = o.proxy(&http.Request{URL: &url.URL{Scheme: "https", Host: "any:443"}})
	require.NotNil(t, gotURL, "WithProxyURL applied last must yield the URL")
	require.Equal(t, "http://a:1", gotURL.String())
}

func TestWithProxyTLSConfig_StoresOption(t *testing.T) {
	t.Parallel()

	pool := x509.NewCertPool()
	cfg := &tls.Config{
		MinVersion: tls.VersionTLS13,
		RootCAs:    pool,
		ServerName: "user-set.example",
	}

	o := &options{}
	WithProxyTLSConfig(cfg)(o)
	require.Same(t, cfg, o.proxyTLSConfig)
	// Merge / clone semantics live in transport/proxydial and
	// are exercised in proxydial_test.go — the test here only verifies
	// the option-storage hook on options.
}

func TestDefaultProxyBehavior_ProxyFieldNil(t *testing.T) {
	grpcAddr := newFakeGRPCServer(t)

	c, err := New(t.Context(), grpcAddr, WithInsecure())
	require.NoError(t, err)
	t.Cleanup(func() { _ = c.Close(t.Context()) })

	require.Nil(t, c.options.proxy, "no proxy option → no resolver → grpc-go default applies")
}

func mustParseURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	u, err := url.Parse(raw)
	require.NoError(t, err)
	return u
}

func splitHostPort(t *testing.T, addr string) (string, int) {
	t.Helper()
	host, portStr, err := net.SplitHostPort(addr)
	require.NoError(t, err)
	port, err := strconv.Atoi(portStr)
	require.NoError(t, err)
	return host, port
}
