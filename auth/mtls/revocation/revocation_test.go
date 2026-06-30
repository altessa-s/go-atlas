// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package revocation_test

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/auth/mtls/revocation"

	"golang.org/x/crypto/ocsp"

	coremtls "github.com/altessa-s/go-atlas/auth/mtls"
)

func makeCA(t testing.TB) (*x509.Certificate, crypto.Signer) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "test-ca"},
		SubjectKeyId:          []byte{0x01, 0x02, 0x03},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, key.Public(), key)
	require.NoError(t, err)
	ca, err := x509.ParseCertificate(der)
	require.NoError(t, err)
	return ca, key
}

func makeLeaf(t testing.TB, ca *x509.Certificate, caKey crypto.Signer, serial int64, ocspURL string) *x509.Certificate {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	tmpl := &x509.Certificate{
		SerialNumber:   big.NewInt(serial),
		Subject:        pkix.Name{CommonName: "client"},
		AuthorityKeyId: ca.SubjectKeyId,
		NotBefore:      time.Now().Add(-time.Hour),
		NotAfter:       time.Now().Add(time.Hour),
		KeyUsage:       x509.KeyUsageDigitalSignature,
		ExtKeyUsage:    []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}
	if ocspURL != "" {
		tmpl.OCSPServer = []string{ocspURL}
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, ca, key.Public(), caKey)
	require.NoError(t, err)
	leaf, err := x509.ParseCertificate(der)
	require.NoError(t, err)
	return leaf
}

// responder serves a signed OCSP response for any request, counting hits.
func responder(t testing.TB, ca *x509.Certificate, caKey crypto.Signer, status int, hits *atomic.Int32) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		serial := big.NewInt(42)
		if body, rerr := io.ReadAll(r.Body); rerr == nil {
			if req, perr := ocsp.ParseRequest(body); perr == nil {
				serial = req.SerialNumber
			}
		}
		tmpl := ocsp.Response{
			Status:       status,
			SerialNumber: serial,
			ThisUpdate:   time.Now().Add(-time.Minute),
			NextUpdate:   time.Now().Add(time.Hour),
			IssuerHash:   crypto.SHA256,
		}
		der, err := ocsp.CreateResponse(ca, ca, tmpl, caKey)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/ocsp-response")
		_, _ = w.Write(der)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestCheckGoodIsCached(t *testing.T) {
	t.Parallel()
	ca, caKey := makeCA(t)
	var hits atomic.Int32
	srv := responder(t, ca, caKey, ocsp.Good, &hits)
	leaf := makeLeaf(t, ca, caKey, 42, srv.URL)

	c := revocation.New([]*x509.Certificate{ca})
	require.NoError(t, c.Check(leaf))
	require.NoError(t, c.Check(leaf)) // served from cache
	require.Equal(t, int32(1), hits.Load())
}

func TestCheckRevoked(t *testing.T) {
	t.Parallel()
	ca, caKey := makeCA(t)
	var hits atomic.Int32
	srv := responder(t, ca, caKey, ocsp.Revoked, &hits)
	leaf := makeLeaf(t, ca, caKey, 42, srv.URL)

	err := revocation.New([]*x509.Certificate{ca}).Check(leaf)
	require.ErrorIs(t, err, coremtls.ErrRevoked)
}

func TestUnreachableResponderFailMode(t *testing.T) {
	t.Parallel()
	ca, caKey := makeCA(t)
	// Port 1 refuses connections; one fast attempt keeps the test quick.
	leaf := makeLeaf(t, ca, caKey, 42, "http://127.0.0.1:1")
	opts := []revocation.Option{revocation.WithMaxAttempts(1), revocation.WithTimeout(time.Second)}

	require.NoError(t, revocation.New([]*x509.Certificate{ca}, opts...).Check(leaf))
	require.Error(t, revocation.New([]*x509.Certificate{ca},
		append(opts, revocation.WithFailMode(revocation.FailClosed))...).Check(leaf))
}

func TestNoResponderURL(t *testing.T) {
	t.Parallel()
	ca, caKey := makeCA(t)
	leaf := makeLeaf(t, ca, caKey, 42, "") // no OCSPServer

	require.NoError(t, revocation.New([]*x509.Certificate{ca}).Check(leaf))
	require.ErrorIs(t,
		revocation.New([]*x509.Certificate{ca}, revocation.WithFailMode(revocation.FailClosed)).Check(leaf),
		revocation.ErrNoResponder)
}

func TestNoIssuer(t *testing.T) {
	t.Parallel()
	ca, caKey := makeCA(t)
	leaf := makeLeaf(t, ca, caKey, 42, "http://example.invalid")

	require.NoError(t, revocation.New(nil).Check(leaf))
	require.ErrorIs(t,
		revocation.New(nil, revocation.WithFailMode(revocation.FailClosed)).Check(leaf),
		revocation.ErrNoIssuer)
}

func TestCacheBounded(t *testing.T) {
	t.Parallel()
	ca, caKey := makeCA(t)
	var hits atomic.Int32
	srv := responder(t, ca, caKey, ocsp.Good, &hits)
	leafA := makeLeaf(t, ca, caKey, 42, srv.URL)
	leafB := makeLeaf(t, ca, caKey, 43, srv.URL)

	c := revocation.New([]*x509.Certificate{ca}, revocation.WithMaxCacheEntries(1))
	require.NoError(t, c.Check(leafA)) // hit 1: caches A
	require.NoError(t, c.Check(leafB)) // hit 2: caches B, evicts A (cap 1)
	require.NoError(t, c.Check(leafA)) // hit 3: A was evicted → re-queries
	require.Equal(t, int32(3), hits.Load())
}

func TestNilCertIsNoOp(t *testing.T) {
	t.Parallel()
	require.NoError(t, revocation.New(nil).Check(nil))
}
