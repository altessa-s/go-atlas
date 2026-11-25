// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package revocation_test

import (
	"crypto/x509"
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

// indefiniteResponders are responders that answer without a definitive, valid,
// fresh status. None of them may end the check while another responder is
// untried.
func indefiniteResponders(t *testing.T, ca *testhelpers.CA) map[string]func(hits *atomic.Int32) *httptest.Server {
	t.Helper()
	now := time.Now()
	return map[string]func(hits *atomic.Int32) *httptest.Server{
		"HTTP 404": func(hits *atomic.Int32) *httptest.Server { return statusResponder(t, http.StatusNotFound, hits) },
		"unparseable body": func(hits *atomic.Int32) *httptest.Server {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				hits.Add(1)
				_, _ = w.Write([]byte("not an OCSP response"))
			}))
			t.Cleanup(srv.Close)
			return srv
		},
		"stale good": func(hits *atomic.Int32) *httptest.Server {
			return windowResponder(t, ca, ocsp.Good, now.Add(-2*time.Hour), now.Add(-time.Hour), hits)
		},
		"unknown status": func(hits *atomic.Int32) *httptest.Server { return responder(t, ca, ocsp.Unknown, hits) },
		"unauthorized signer": func(hits *atomic.Int32) *httptest.Server {
			ordinary := ca.SignLeaf(t, testhelpers.WithExtKeyUsage(x509.ExtKeyUsageClientAuth))
			return delegatedResponder(t, ca, ordinary, ocsp.Good, hits)
		},
		"cross-host redirect": func(hits *atomic.Int32) *httptest.Server {
			return redirector(t, "http://ocsp.example.org/", hits)
		},
	}
}

// A responder that gives no definitive answer must not hide a revocation the
// next responder of the certificate would report — under fail-open that would
// accept a revoked certificate.
func TestIndefiniteResponderDoesNotHideRevocation(t *testing.T) {
	t.Parallel()
	ca := makeCA(t)

	for name, first := range indefiniteResponders(t, ca) {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			var firstHits, revokedHits atomic.Int32
			revoked := responder(t, ca, ocsp.Revoked, &revokedHits)
			leaf := leafWithResponders(t, ca, 42, first(&firstHits).URL, revoked.URL)

			for _, mode := range []revocation.FailMode{revocation.FailClosed, revocation.FailOpen} {
				c := newChecker([]*x509.Certificate{ca.Cert}, revocation.WithFailMode(mode))
				require.ErrorIs(t, c.Check(t.Context(), leaf), coremtls.ErrRevoked)
			}
			require.Equal(t, int32(2), firstHits.Load(), "an indefinite responder is asked once per check")
			require.Equal(t, int32(2), revokedHits.Load())
		})
	}
}

func TestIndefiniteResponderFallsBackToGood(t *testing.T) {
	t.Parallel()
	ca := makeCA(t)

	for name, first := range indefiniteResponders(t, ca) {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			var firstHits, goodHits atomic.Int32
			good := responder(t, ca, ocsp.Good, &goodHits)
			leaf := leafWithResponders(t, ca, 42, first(&firstHits).URL, good.URL)

			require.NoError(t, newChecker([]*x509.Certificate{ca.Cert}).Check(t.Context(), leaf))
			require.Equal(t, int32(1), firstHits.Load())
			require.Equal(t, int32(1), goodHits.Load())
		})
	}
}

// A revoked status is final: no later responder can overrule it.
func TestRevokedFromFirstResponderIsFinal(t *testing.T) {
	t.Parallel()
	ca := makeCA(t)
	var revokedHits, goodHits atomic.Int32
	revoked := responder(t, ca, ocsp.Revoked, &revokedHits)
	good := responder(t, ca, ocsp.Good, &goodHits)
	leaf := leafWithResponders(t, ca, 42, revoked.URL, good.URL)

	require.ErrorIs(t, newChecker([]*x509.Certificate{ca.Cert}).Check(t.Context(), leaf), coremtls.ErrRevoked)
	require.Zero(t, goodHits.Load())
}

// When no responder gives a definitive answer, the error carries every
// responder's failure.
func TestAllRespondersFailJoinsErrors(t *testing.T) {
	t.Parallel()
	ca := makeCA(t)
	var staleHits, unknownHits atomic.Int32
	now := time.Now()
	stale := windowResponder(t, ca, ocsp.Good, now.Add(-2*time.Hour), now.Add(-time.Hour), &staleHits)
	unknown := responder(t, ca, ocsp.Unknown, &unknownHits)
	leaf := leafWithResponders(t, ca, 42, stale.URL, unknown.URL)

	err := newChecker([]*x509.Certificate{ca.Cert}).Check(t.Context(), leaf)
	require.ErrorIs(t, err, revocation.ErrStaleResponse)
	require.ErrorIs(t, err, revocation.ErrUnknownStatus)
	require.Equal(t, int32(1), staleHits.Load())
	require.Equal(t, int32(1), unknownHits.Load())

	open := newChecker([]*x509.Certificate{ca.Cert}, revocation.WithFailMode(revocation.FailOpen))
	require.NoError(t, open.Check(t.Context(), leaf), "fail-open keeps accepting an indeterminate status")
}
