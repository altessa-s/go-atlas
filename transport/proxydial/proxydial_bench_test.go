// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package proxydial

import (
	"bufio"
	"crypto/tls"
	"crypto/x509"
	"io"
	"net"
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"
)

func BenchmarkTLSConfig_NilUser(b *testing.B) {
	u, _ := url.Parse("https://proxy.example.com:8443")
	for b.Loop() {
		_ = TLSConfig(u, nil)
	}
}

func BenchmarkTLSConfig_WithUserConfig(b *testing.B) {
	u, _ := url.Parse("https://proxy.example.com:8443")
	user := &tls.Config{RootCAs: x509.NewCertPool()}
	for b.Loop() {
		_ = TLSConfig(u, user)
	}
}

func BenchmarkBasicAuthHeader(b *testing.B) {
	auth := url.UserPassword("svc", "secret")
	for b.Loop() {
		_ = basicAuthHeader(auth)
	}
}

func BenchmarkFromURL_HTTPS(b *testing.B) {
	u, _ := url.Parse("https://proxy.example.com:8443")
	for b.Loop() {
		_, _ = FromURL(u)
	}
}

func BenchmarkFromURL_SOCKS5(b *testing.B) {
	u, _ := url.Parse("socks5://socks.example.com:1080")
	for b.Loop() {
		_, _ = FromURL(u)
	}
}

func BenchmarkFromURL_Nil(b *testing.B) {
	for b.Loop() {
		_, _ = FromURL(nil)
	}
}

// startLoopProxy starts a fake CONNECT proxy that answers every connection
// with a 200 followed by greeting in the same write.
func startLoopProxy(b *testing.B, greeting string) *url.URL {
	b.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(b, err)
	b.Cleanup(func() { _ = ln.Close() })

	go func() {
		for {
			conn, acceptErr := ln.Accept()
			if acceptErr != nil {
				return
			}
			go func() {
				defer func() { _ = conn.Close() }()
				br := bufio.NewReader(conn)
				if _, readErr := http.ReadRequest(br); readErr != nil {
					return
				}
				if _, writeErr := io.WriteString(conn, "HTTP/1.1 200 Connection established\r\n\r\n"+greeting); writeErr != nil {
					return
				}
				_, _ = io.Copy(io.Discard, br)
			}()
		}
	}()
	return &url.URL{Scheme: "http", Host: ln.Addr().String()}
}

func BenchmarkHTTPConnect(b *testing.B) {
	for _, bc := range []struct {
		name     string
		greeting string
	}{
		{"client_first", ""},
		{"server_first", "220 smtp.example ESMTP ready\r\n"},
	} {
		b.Run(bc.name, func(b *testing.B) {
			proxyURL := startLoopProxy(b, bc.greeting)
			dialer := &net.Dialer{}
			buf := make([]byte, len(bc.greeting))
			for b.Loop() {
				conn, err := HTTPConnect(b.Context(), dialer, proxyURL, nil, "smtp.example:25")
				if err != nil {
					b.Fatal(err)
				}
				if _, err = io.ReadFull(conn, buf); err != nil {
					b.Fatal(err)
				}
				_ = conn.Close()
			}
		})
	}
}
