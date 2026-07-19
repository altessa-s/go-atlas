// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package revocation_test

import (
	"crypto"
	"crypto/x509"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/auth/mtls/revocation"
	"github.com/altessa-s/go-atlas/core/collections/slices"
	"github.com/altessa-s/go-atlas/internal/testhelpers"

	"golang.org/x/crypto/ocsp"

	coremtls "github.com/altessa-s/go-atlas/auth/mtls"
)

// makeCA returns a shared in-memory test CA with a pinned subject key id;
// leaves chain to it through their AuthorityKeyId.
func makeCA(t testing.TB) *testhelpers.CA {
	t.Helper()
	return testhelpers.NewCA(t, testhelpers.WithSubjectKeyID([]byte{0x01, 0x02, 0x03}))
}

// makeLeaf mints a client-auth leaf with the given serial and optional OCSP
// responder URL, signed by ca.
func makeLeaf(t testing.TB, ca *testhelpers.CA, serial int64, ocspURL string) *x509.Certificate {
	t.Helper()
	opts := []testhelpers.CertOption{
		testhelpers.WithSerial(serial),
		testhelpers.WithCommonName("client"),
		testhelpers.WithExtKeyUsage(x509.ExtKeyUsageClientAuth),
	}
	opts = slices.AppendIf(opts, ocspURL != "", testhelpers.WithOCSPServers(ocspURL))
	return ca.SignLeaf(t, opts...).Leaf
}

// responder serves a signed OCSP response for any request, counting hits.
func responder(t testing.TB, ca *testhelpers.CA, status int, hits *atomic.Int32) *httptest.Server {
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
		der, err := ocsp.CreateResponse(ca.Cert, ca.Cert, tmpl, ca.Key)
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
	ca := makeCA(t)
	var hits atomic.Int32
	srv := responder(t, ca, ocsp.Good, &hits)
	leaf := makeLeaf(t, ca, 42, srv.URL)

	c := revocation.New([]*x509.Certificate{ca.Cert})
	require.NoError(t, c.Check(leaf))
	require.NoError(t, c.Check(leaf)) // served from cache
	require.Equal(t, int32(1), hits.Load())
}

func TestCheckRevoked(t *testing.T) {
	t.Parallel()
	ca := makeCA(t)
	var hits atomic.Int32
	srv := responder(t, ca, ocsp.Revoked, &hits)
	leaf := makeLeaf(t, ca, 42, srv.URL)

	err := revocation.New([]*x509.Certificate{ca.Cert}).Check(leaf)
	require.ErrorIs(t, err, coremtls.ErrRevoked)
}

func TestUnreachableResponderFailMode(t *testing.T) {
	t.Parallel()
	ca := makeCA(t)
	// Port 1 refuses connections; one fast attempt keeps the test quick.
	leaf := makeLeaf(t, ca, 42, "http://127.0.0.1:1")
	opts := []revocation.Option{revocation.WithMaxAttempts(1), revocation.WithTimeout(time.Second)}

	require.NoError(t, revocation.New([]*x509.Certificate{ca.Cert}, opts...).Check(leaf))
	require.Error(t, revocation.New([]*x509.Certificate{ca.Cert},
		append(opts, revocation.WithFailMode(revocation.FailClosed))...).Check(leaf))
}

func TestNoResponderURL(t *testing.T) {
	t.Parallel()
	ca := makeCA(t)
	leaf := makeLeaf(t, ca, 42, "") // no OCSPServer

	require.NoError(t, revocation.New([]*x509.Certificate{ca.Cert}).Check(leaf))
	require.ErrorIs(t,
		revocation.New([]*x509.Certificate{ca.Cert}, revocation.WithFailMode(revocation.FailClosed)).Check(leaf),
		revocation.ErrNoResponder)
}

func TestNoIssuer(t *testing.T) {
	t.Parallel()
	ca := makeCA(t)
	leaf := makeLeaf(t, ca, 42, "http://example.invalid")

	require.NoError(t, revocation.New(nil).Check(leaf))
	require.ErrorIs(t,
		revocation.New(nil, revocation.WithFailMode(revocation.FailClosed)).Check(leaf),
		revocation.ErrNoIssuer)
}

func TestCacheBounded(t *testing.T) {
	t.Parallel()
	ca := makeCA(t)
	var hits atomic.Int32
	srv := responder(t, ca, ocsp.Good, &hits)
	leafA := makeLeaf(t, ca, 42, srv.URL)
	leafB := makeLeaf(t, ca, 43, srv.URL)

	c := revocation.New([]*x509.Certificate{ca.Cert}, revocation.WithMaxCacheEntries(1))
	require.NoError(t, c.Check(leafA)) // hit 1: caches A
	require.NoError(t, c.Check(leafB)) // hit 2: caches B, evicts A (cap 1)
	require.NoError(t, c.Check(leafA)) // hit 3: A was evicted → re-queries
	require.Equal(t, int32(3), hits.Load())
}

// statusResponder always answers with a fixed HTTP status, counting hits.
func statusResponder(t testing.TB, code int, hits *atomic.Int32) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(code)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestRetryClassification(t *testing.T) {
	t.Parallel()
	ca := makeCA(t)

	t.Run("5xx is retried", func(t *testing.T) {
		t.Parallel()
		var hits atomic.Int32
		srv := statusResponder(t, http.StatusInternalServerError, &hits)
		leaf := makeLeaf(t, ca, 42, srv.URL)
		_ = revocation.New([]*x509.Certificate{ca.Cert}, revocation.WithMaxAttempts(3)).Check(leaf)
		require.Greater(t, hits.Load(), int32(1)) // retried beyond the first attempt
	})

	t.Run("4xx is terminal", func(t *testing.T) {
		t.Parallel()
		var hits atomic.Int32
		srv := statusResponder(t, http.StatusBadRequest, &hits)
		leaf := makeLeaf(t, ca, 43, srv.URL)
		_ = revocation.New([]*x509.Certificate{ca.Cert}, revocation.WithMaxAttempts(3)).Check(leaf)
		require.Equal(t, int32(1), hits.Load())
	})
}

func TestSingleflightDedup(t *testing.T) {
	t.Parallel()
	ca := makeCA(t)
	var hits atomic.Int32
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		<-release // hold the request in-flight so concurrent callers must dedup
		serial := big.NewInt(42)
		if body, rerr := io.ReadAll(r.Body); rerr == nil {
			if req, perr := ocsp.ParseRequest(body); perr == nil {
				serial = req.SerialNumber
			}
		}
		der, err := ocsp.CreateResponse(ca.Cert, ca.Cert, ocsp.Response{
			Status:       ocsp.Good,
			SerialNumber: serial,
			ThisUpdate:   time.Now().Add(-time.Minute),
			NextUpdate:   time.Now().Add(time.Hour),
			IssuerHash:   crypto.SHA256,
		}, ca.Key)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/ocsp-response")
		_, _ = w.Write(der)
	}))
	t.Cleanup(srv.Close)

	leaf := makeLeaf(t, ca, 42, srv.URL)
	c := revocation.New([]*x509.Certificate{ca.Cert})

	const n = 8
	errs := make([]error, n)
	var wg sync.WaitGroup
	for i := range n {
		wg.Go(func() { errs[i] = c.Check(leaf) })
	}
	// One shared query reaches the responder; broken dedup would let others in too.
	require.Eventually(t, func() bool { return hits.Load() >= 1 }, time.Second, time.Millisecond)
	time.Sleep(50 * time.Millisecond) // give any non-deduped calls time to arrive
	close(release)
	wg.Wait()

	require.Equal(t, int32(1), hits.Load())
	for _, e := range errs {
		require.NoError(t, e)
	}
}

func TestNilCertIsNoOp(t *testing.T) {
	t.Parallel()
	require.NoError(t, revocation.New(nil).Check(nil))
}
