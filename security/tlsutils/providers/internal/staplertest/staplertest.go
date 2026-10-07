// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package staplertest

import (
	"context"
	"crypto/tls"
	"net"
	"sync/atomic"
	"testing"

	"github.com/altessa-s/go-atlas/security/tlsutils"
)

// Staple is the fixed staple served by [CountingStapler].
var Staple = []byte("test-ocsp-staple")

// CountingStapler is a [tlsutils.OCSPStapler] that serves [Staple] and counts
// GetOCSPStaple calls.
type CountingStapler struct {
	calls atomic.Int32
}

var _ tlsutils.OCSPStapler = (*CountingStapler)(nil)

// GetOCSPStaple records the call and returns [Staple].
func (s *CountingStapler) GetOCSPStaple(context.Context, *tls.Certificate) ([]byte, error) {
	s.calls.Add(1)
	return Staple, nil
}

// RunRefreshAll is a no-op.
func (s *CountingStapler) RunRefreshAll(context.Context) error { return nil }

// Calls reports how many times GetOCSPStaple was called.
func (s *CountingStapler) Calls() int32 { return s.calls.Load() }

// Handshake runs one TLS handshake over an in-memory pipe with cfg as the
// server config and returns the OCSP response the client received.
func Handshake(tb testing.TB, cfg *tls.Config) []byte {
	tb.Helper()
	serverConn, clientConn := net.Pipe()
	tb.Cleanup(func() {
		_ = serverConn.Close()
		_ = clientConn.Close()
	})

	serverErr := make(chan error, 1)
	go func() {
		serverErr <- tls.Server(serverConn, cfg).HandshakeContext(tb.Context())
	}()

	client := tls.Client(clientConn, &tls.Config{
		// SNI makes crypto/tls consult GetCertificate even when
		// Config.Certificates is populated.
		ServerName:         "localhost",
		InsecureSkipVerify: true, //nolint:gosec // test peer uses a self-signed certificate
		MinVersion:         tls.VersionTLS12,
	})
	if err := client.HandshakeContext(tb.Context()); err != nil {
		tb.Fatalf("client handshake: %v", err)
	}
	if err := <-serverErr; err != nil {
		tb.Fatalf("server handshake: %v", err)
	}
	return client.ConnectionState().OCSPResponse
}
