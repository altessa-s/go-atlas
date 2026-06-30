// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mtls_test

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/auth/audit"
	"github.com/altessa-s/go-atlas/auth/spiffe"
	"github.com/altessa-s/go-atlas/transport/http/server/middlewares/auth/mtls"

	coremtls "github.com/altessa-s/go-atlas/auth/mtls"
	authmw "github.com/altessa-s/go-atlas/transport/http/server/middlewares/auth"
)

func requestWithCert(t *testing.T, cert *x509.Certificate) *http.Request {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	if cert != nil {
		req.TLS = &tls.ConnectionState{VerifiedChains: [][]*x509.Certificate{{cert}}}
	}
	return req
}

func spiffeCert(t *testing.T, id string) *x509.Certificate {
	t.Helper()
	u, err := url.Parse(id)
	require.NoError(t, err)
	return &x509.Certificate{URIs: []*url.URL{u}}
}

// capture is a handler that records the principal installed by the middleware.
func capture(dst *any) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*dst = authmw.FromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	})
}

func TestMiddlewareSuccess(t *testing.T) {
	t.Parallel()
	var got any
	rec := httptest.NewRecorder()
	mtls.Middleware()(capture(&got)).ServeHTTP(rec, requestWithCert(t, spiffeCert(t, "spiffe://example.org/sa/billing")))

	require.Equal(t, http.StatusOK, rec.Code)
	id, ok := got.(spiffe.ID)
	require.True(t, ok)
	require.Equal(t, "spiffe://example.org/sa/billing", id.String())
}

func TestMiddlewareWithIdentity(t *testing.T) {
	t.Parallel()
	var got any
	mw := mtls.Middleware(coremtls.WithIdentity(func(c *x509.Certificate) (any, error) {
		id, err := spiffe.IDFromCertificate(c)
		return id.Path, err
	}))
	rec := httptest.NewRecorder()
	mw(capture(&got)).ServeHTTP(rec, requestWithCert(t, spiffeCert(t, "spiffe://example.org/sa/billing")))
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "/sa/billing", got)
}

func TestMiddlewareUnauthorized(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		req  func(t *testing.T) *http.Request
	}{
		{"no tls", func(t *testing.T) *http.Request { return requestWithCert(t, nil) }},
		{"no spiffe san", func(t *testing.T) *http.Request { return requestWithCert(t, &x509.Certificate{}) }},
		{"untrusted domain", func(t *testing.T) *http.Request { return requestWithCert(t, spiffeCert(t, "spiffe://evil.example/x")) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			mw := mtls.Middleware(coremtls.WithValidator(coremtls.TrustDomainValidator("example.org")))
			rec := httptest.NewRecorder()
			called := false
			mw(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true })).ServeHTTP(rec, tc.req(t))
			require.Equal(t, http.StatusUnauthorized, rec.Code)
			require.False(t, called) // fail-closed: handler not reached
		})
	}
}

func TestMiddlewareRequiredAuditMapsTo500(t *testing.T) {
	t.Parallel()
	rec := audit.NewRecorder(
		audit.SinkFunc(func(context.Context, audit.Decision) error { return errors.New("sink down") }),
		audit.WithPolicyMode(audit.PolicyAll), audit.WithFailureMode(audit.FailureRequired),
	)
	mw := mtls.Middleware(coremtls.WithAudit(rec, nil))
	w := httptest.NewRecorder()
	mw(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})).ServeHTTP(w, requestWithCert(t, spiffeCert(t, "spiffe://example.org/x")))
	require.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestPeerCertificate(t *testing.T) {
	t.Parallel()
	cert := spiffeCert(t, "spiffe://example.org/x")
	got, err := mtls.PeerCertificate(requestWithCert(t, cert))
	require.NoError(t, err)
	require.Same(t, cert, got)

	_, err = mtls.PeerCertificate(requestWithCert(t, nil))
	require.ErrorIs(t, err, mtls.ErrNoTLS)
}
