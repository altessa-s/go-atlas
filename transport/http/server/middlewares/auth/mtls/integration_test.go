// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mtls_test

import (
	"crypto/tls"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/auth/spiffe"
	"github.com/altessa-s/go-atlas/transport/http/server/middlewares/auth/mtls"
	"github.com/altessa-s/go-atlas/transport/internal/mtlstest"

	coremtls "github.com/altessa-s/go-atlas/auth/mtls"
	authmw "github.com/altessa-s/go-atlas/transport/http/server/middlewares/auth"
)

// TestMiddlewareRealHandshake drives the middleware over a genuine mutual-TLS
// handshake: a real server requires and verifies the client certificate, so the
// VerifiedChains the middleware reads are populated by the TLS stack itself
// (not synthesized). It asserts the SPIFFE identity carried in the client
// certificate's URI SAN reaches the handler as the installed principal.
func TestMiddlewareRealHandshake(t *testing.T) {
	t.Parallel()
	ca := mtlstest.NewCA(t)
	serverCert := ca.ServerCert(t, net.IPv4(127, 0, 0, 1))
	spiffeID := "spiffe://example.org/sa/billing"
	clientCert := ca.ClientCert(t, spiffeID)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	tlsLn := tls.NewListener(ln, &tls.Config{
		Certificates: []tls.Certificate{serverCert},
		ClientCAs:    ca.Pool,
		ClientAuth:   tls.RequireAndVerifyClientCert,
		MinVersion:   tls.VersionTLS13,
	})

	handler := mtls.Middleware(coremtls.WithValidator(coremtls.TrustDomainValidator("example.org")))(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			id, _ := authmw.FromContext(r.Context()).(spiffe.ID)
			_, _ = io.WriteString(w, id.String())
		}))
	srv := &http.Server{Handler: handler, ReadHeaderTimeout: time.Second}
	go func() { _ = srv.Serve(tlsLn) }()
	t.Cleanup(func() { _ = srv.Close() })

	client := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{
		Certificates: []tls.Certificate{clientCert},
		RootCAs:      ca.Pool,
		MinVersion:   tls.VersionTLS13,
	}}}
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "https://"+ln.Addr().String()+"/", nil)
	require.NoError(t, err)
	resp, err := client.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, http.StatusOK, resp.StatusCode)
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, spiffeID, string(body)) // peer cert → spiffe.ID principal, end to end
}
