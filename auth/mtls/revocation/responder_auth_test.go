// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package revocation_test

import (
	"crypto"
	"crypto/tls"
	"crypto/x509"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/auth/mtls/revocation"
	"github.com/altessa-s/go-atlas/internal/testhelpers"

	"golang.org/x/crypto/ocsp"

	coremtls "github.com/altessa-s/go-atlas/auth/mtls"
)

// delegatedResponder serves a response with the given status signed by signer
// (a certificate issued by ca) and embeds signer's certificate, as a delegated
// responder — or anyone substituting the HTTP response — would.
func delegatedResponder(t *testing.T, ca *testhelpers.CA, signer tls.Certificate, status int, hits *atomic.Int32) *httptest.Server {
	t.Helper()
	key, ok := signer.PrivateKey.(crypto.Signer)
	require.True(t, ok)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		req, err := ocsp.ParseRequest(body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		der, err := ocsp.CreateResponse(ca.Cert, signer.Leaf, ocsp.Response{
			Status:       status,
			SerialNumber: req.SerialNumber,
			ThisUpdate:   time.Now().Add(-time.Minute),
			NextUpdate:   time.Now().Add(time.Hour),
			RevokedAt:    time.Now().Add(-time.Minute),
			IssuerHash:   crypto.SHA256,
			Certificate:  signer.Leaf,
		}, key)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		_, _ = w.Write(der)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// An ordinary certificate from the same issuer must not be able to sign a
// "good" status: only a delegated responder certificate may.
func TestCheckRejectsResponseSignedByOrdinaryCertificate(t *testing.T) {
	t.Parallel()
	ca := makeCA(t)
	ordinary := ca.SignLeaf(t, testhelpers.WithCommonName("some-client"),
		testhelpers.WithExtKeyUsage(x509.ExtKeyUsageClientAuth))

	for _, status := range []int{ocsp.Good, ocsp.Revoked} {
		var hits atomic.Int32
		srv := delegatedResponder(t, ca, ordinary, status, &hits)
		leaf := makeLeaf(t, ca, 42, srv.URL)

		c := newChecker([]*x509.Certificate{ca.Cert}, revocation.WithMaxAttempts(3))
		require.ErrorIs(t, c.Check(leaf), revocation.ErrUnauthorizedResponder)
		require.ErrorIs(t, c.Check(leaf), revocation.ErrUnauthorizedResponder)
		require.Equal(t, int32(2), hits.Load(), "an unauthorized response is neither retried nor cached")
	}
}

// ExtKeyUsageAny is not a delegation: RFC 6960 names id-kp-OCSPSigning.
func TestCheckRejectsResponseSignedByAnyEKUCertificate(t *testing.T) {
	t.Parallel()
	ca := makeCA(t)
	anyEKU := ca.SignLeaf(t, testhelpers.WithExtKeyUsage(x509.ExtKeyUsageAny))
	var hits atomic.Int32
	srv := delegatedResponder(t, ca, anyEKU, ocsp.Good, &hits)
	leaf := makeLeaf(t, ca, 42, srv.URL)

	err := newChecker([]*x509.Certificate{ca.Cert}).Check(leaf)
	require.ErrorIs(t, err, revocation.ErrUnauthorizedResponder)
}

func TestCheckAcceptsDelegatedResponder(t *testing.T) {
	t.Parallel()
	ca := makeCA(t)
	delegate := ca.SignLeaf(t, testhelpers.WithCommonName("ocsp-responder"),
		testhelpers.WithExtKeyUsage(x509.ExtKeyUsageOCSPSigning))

	var goodHits, revokedHits atomic.Int32
	good := delegatedResponder(t, ca, delegate, ocsp.Good, &goodHits)
	revoked := delegatedResponder(t, ca, delegate, ocsp.Revoked, &revokedHits)

	c := newChecker([]*x509.Certificate{ca.Cert})
	require.NoError(t, c.Check(makeLeaf(t, ca, 42, good.URL)))
	require.ErrorIs(t, c.Check(makeLeaf(t, ca, 43, revoked.URL)), coremtls.ErrRevoked)
}

func TestCheckRejectsDelegatedResponderOutsideValidity(t *testing.T) {
	t.Parallel()
	ca := makeCA(t)

	tests := []struct {
		name     string
		validity testhelpers.CertOption
	}{
		{"expired", func(c *x509.Certificate) { c.NotAfter = time.Now().Add(-30 * time.Minute) }},
		{"not yet valid", func(c *x509.Certificate) { c.NotBefore = time.Now().Add(30 * time.Minute) }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			delegate := ca.SignLeaf(t, testhelpers.WithExtKeyUsage(x509.ExtKeyUsageOCSPSigning), tc.validity)
			var hits atomic.Int32
			srv := delegatedResponder(t, ca, delegate, ocsp.Good, &hits)
			leaf := makeLeaf(t, ca, 42, srv.URL)

			err := newChecker([]*x509.Certificate{ca.Cert}).Check(leaf)
			require.ErrorIs(t, err, revocation.ErrUnauthorizedResponder)

			open := newChecker([]*x509.Certificate{ca.Cert}, revocation.WithFailMode(revocation.FailOpen))
			require.NoError(t, open.Check(leaf), "fail-open keeps accepting an indeterminate status")
		})
	}
}
