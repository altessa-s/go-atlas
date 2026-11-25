// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package revocation_test

import (
	"bytes"
	"crypto"
	"crypto/x509"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas-core/collections/slices"
	"github.com/altessa-s/go-atlas/auth/mtls/revocation"
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

// newChecker builds a Checker that may reach the loopback test responders the
// default client refuses.
func newChecker(issuers []*x509.Certificate, opts ...revocation.Option) *revocation.Checker {
	return revocation.New(issuers, append([]revocation.Option{revocation.WithInsecureAllowPrivateResponders()}, opts...)...)
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

	c := newChecker([]*x509.Certificate{ca.Cert})
	require.NoError(t, c.Check(t.Context(), leaf))
	require.NoError(t, c.Check(t.Context(), leaf)) // served from cache
	require.Equal(t, int32(1), hits.Load())
}

func TestCheckRevoked(t *testing.T) {
	t.Parallel()
	ca := makeCA(t)
	var hits atomic.Int32
	srv := responder(t, ca, ocsp.Revoked, &hits)
	leaf := makeLeaf(t, ca, 42, srv.URL)

	err := newChecker([]*x509.Certificate{ca.Cert}).Check(t.Context(), leaf)
	require.ErrorIs(t, err, coremtls.ErrRevoked)
}

func TestUnreachableResponderFailMode(t *testing.T) {
	t.Parallel()
	ca := makeCA(t)
	// Port 1 refuses connections; one fast attempt keeps the test quick.
	leaf := makeLeaf(t, ca, 42, "http://127.0.0.1:1")
	opts := []revocation.Option{revocation.WithMaxAttempts(1), revocation.WithTimeout(time.Second)}

	require.Error(t, newChecker([]*x509.Certificate{ca.Cert}, opts...).Check(t.Context(), leaf)) // fail-closed by default
	require.NoError(t, newChecker([]*x509.Certificate{ca.Cert},
		append(opts, revocation.WithFailMode(revocation.FailOpen))...).Check(t.Context(), leaf))
}

func TestNoResponderURL(t *testing.T) {
	t.Parallel()
	ca := makeCA(t)
	leaf := makeLeaf(t, ca, 42, "") // no OCSPServer

	require.ErrorIs(t, newChecker([]*x509.Certificate{ca.Cert}).Check(t.Context(), leaf), revocation.ErrNoResponder)
	require.NoError(t,
		newChecker([]*x509.Certificate{ca.Cert}, revocation.WithFailMode(revocation.FailOpen)).Check(t.Context(), leaf))
}

func TestNoIssuer(t *testing.T) {
	t.Parallel()
	ca := makeCA(t)
	leaf := makeLeaf(t, ca, 42, "http://example.invalid")

	require.ErrorIs(t, newChecker(nil).Check(t.Context(), leaf), revocation.ErrNoIssuer)
	require.NoError(t, newChecker(nil, revocation.WithFailMode(revocation.FailOpen)).Check(t.Context(), leaf))
}

func TestDefaultFailModeIsFailClosed(t *testing.T) {
	t.Parallel()
	require.Equal(t, revocation.FailClosed, revocation.DefaultFailMode)
}

// redirector answers every request with a redirect to target, counting hits.
func redirector(t testing.TB, target string, hits *atomic.Int32) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		http.Redirect(w, r, target, http.StatusTemporaryRedirect)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestDefaultClientRefusesCrossHostRedirect(t *testing.T) {
	t.Parallel()
	ca := makeCA(t)
	var targetHits, redirectHits atomic.Int32
	target := responder(t, ca, ocsp.Good, &targetHits) // another host:port
	srv := redirector(t, target.URL, &redirectHits)
	leaf := makeLeaf(t, ca, 42, srv.URL)

	err := newChecker([]*x509.Certificate{ca.Cert}).Check(t.Context(), leaf)
	require.ErrorIs(t, err, revocation.ErrCrossHostRedirect)
	require.Equal(t, int32(1), redirectHits.Load()) // terminal: not retried
	require.Zero(t, targetHits.Load())              // the redirect was never followed
}

func TestDefaultClientFollowsSameHostRedirect(t *testing.T) {
	t.Parallel()
	ca := makeCA(t)
	var hits atomic.Int32
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	mux.Handle("/ocsp", responder(t, ca, ocsp.Good, &hits).Config.Handler)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/ocsp", http.StatusTemporaryRedirect)
	})
	leaf := makeLeaf(t, ca, 42, srv.URL)

	require.NoError(t, newChecker([]*x509.Certificate{ca.Cert}).Check(t.Context(), leaf))
	require.Equal(t, int32(1), hits.Load())
}

func TestCacheBounded(t *testing.T) {
	t.Parallel()
	ca := makeCA(t)
	var hits atomic.Int32
	srv := responder(t, ca, ocsp.Good, &hits)
	leafA := makeLeaf(t, ca, 42, srv.URL)
	leafB := makeLeaf(t, ca, 43, srv.URL)

	c := newChecker([]*x509.Certificate{ca.Cert}, revocation.WithMaxCacheEntries(1))
	require.NoError(t, c.Check(t.Context(), leafA)) // hit 1: caches A
	require.NoError(t, c.Check(t.Context(), leafB)) // hit 2: caches B, evicts A (cap 1)
	require.NoError(t, c.Check(t.Context(), leafA)) // hit 3: A was evicted → re-queries
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
		_ = newChecker([]*x509.Certificate{ca.Cert}, revocation.WithMaxAttempts(3)).Check(t.Context(), leaf)
		require.Greater(t, hits.Load(), int32(1)) // retried beyond the first attempt
	})

	t.Run("4xx is terminal", func(t *testing.T) {
		t.Parallel()
		var hits atomic.Int32
		srv := statusResponder(t, http.StatusBadRequest, &hits)
		leaf := makeLeaf(t, ca, 43, srv.URL)
		_ = newChecker([]*x509.Certificate{ca.Cert}, revocation.WithMaxAttempts(3)).Check(t.Context(), leaf)
		require.Equal(t, int32(1), hits.Load())
	})
}

func TestSingleflightDedup(t *testing.T) {
	t.Parallel()
	ca := makeCA(t)
	var hits atomic.Int32
	leaf := makeLeaf(t, ca, 42, "http://ocsp.example.org")

	// The responder is an in-memory round tripper and the test runs in a
	// synctest bubble, so synctest.Wait returns only when every caller is
	// blocked: in its own request to the responder, or waiting on the flight.
	synctest.Test(t, func(t *testing.T) {
		release := make(chan struct{})
		transport := testhelpers.RoundTripFunc(func(r *http.Request) (*http.Response, error) {
			hits.Add(1)
			<-release // hold the request in-flight so concurrent callers must dedup
			body, err := io.ReadAll(r.Body)
			if err != nil {
				return nil, err
			}
			req, err := ocsp.ParseRequest(body)
			if err != nil {
				return nil, err
			}
			der, err := ocsp.CreateResponse(ca.Cert, ca.Cert, ocsp.Response{
				Status:       ocsp.Good,
				SerialNumber: req.SerialNumber,
				ThisUpdate:   time.Now().Add(-time.Minute),
				NextUpdate:   time.Now().Add(time.Hour),
				IssuerHash:   crypto.SHA256,
			}, ca.Key)
			if err != nil {
				return nil, err
			}
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewReader(der))}, nil
		})
		c := revocation.New([]*x509.Certificate{ca.Cert}, revocation.WithHTTPClient(&http.Client{Transport: transport}))

		const n = 8
		errs := make([]error, n)
		var wg sync.WaitGroup
		for i := range n {
			wg.Go(func() { errs[i] = c.Check(t.Context(), leaf) })
		}
		synctest.Wait()
		// One shared query reaches the responder; broken dedup would let others in too.
		require.Equal(t, int32(1), hits.Load())
		close(release)
		wg.Wait()

		require.Equal(t, int32(1), hits.Load())
		for _, e := range errs {
			require.NoError(t, e)
		}
	})
}

func TestNilCertIsNoOp(t *testing.T) {
	t.Parallel()
	require.NoError(t, newChecker(nil).Check(t.Context(), nil))
}
