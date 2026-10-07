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
