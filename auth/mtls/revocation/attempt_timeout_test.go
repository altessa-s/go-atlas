// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package revocation_test

import (
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
)

// leafWithResponders mints a client-auth leaf naming several OCSP responders.
func leafWithResponders(t *testing.T, ca *testhelpers.CA, serial int64, urls ...string) *x509.Certificate {
	t.Helper()
	return ca.SignLeaf(t,
		testhelpers.WithSerial(serial),
		testhelpers.WithCommonName("client"),
		testhelpers.WithExtKeyUsage(x509.ExtKeyUsageClientAuth),
		testhelpers.WithOCSPServers(urls...),
	).Leaf
}

// A stalled request must cost one attempt, not the whole check: the retry
// reaches the responder once it answers again.
func TestStalledAttemptLeavesRoomForRetry(t *testing.T) {
	t.Parallel()
	ca := makeCA(t)
	var hits, goodHits atomic.Int32
	good := responder(t, ca, ocsp.Good, &goodHits).Config.Handler
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if hits.Add(1) == 1 {
			_, _ = io.Copy(io.Discard, r.Body)
			select { // stall until the client gives up on this attempt
			case <-r.Context().Done():
			case <-release:
			}
			return
		}
		good.ServeHTTP(w, r)
	}))
	t.Cleanup(srv.Close)
	t.Cleanup(func() { close(release) })
	leaf := makeLeaf(t, ca, 42, srv.URL)

	c := newChecker([]*x509.Certificate{ca.Cert},
		revocation.WithTimeout(30*time.Second), revocation.WithAttemptTimeout(200*time.Millisecond))
	require.NoError(t, c.Check(leaf))
	require.Equal(t, int32(2), hits.Load())
}

// Every responder URL of the certificate is a candidate, not only the first.
func TestFallsBackToNextResponderURL(t *testing.T) {
	t.Parallel()
	ca := makeCA(t)
	var downHits, goodHits atomic.Int32
	down := statusResponder(t, http.StatusServiceUnavailable, &downHits)
	good := responder(t, ca, ocsp.Good, &goodHits)
	leaf := leafWithResponders(t, ca, 42, down.URL, good.URL)

	require.NoError(t, newChecker([]*x509.Certificate{ca.Cert}).Check(leaf))
	require.Equal(t, int32(1), downHits.Load())
	require.Equal(t, int32(1), goodHits.Load())
}

// A certificate naming more responders than the retry count has each tried.
func TestTriesEveryResponderURL(t *testing.T) {
	t.Parallel()
	ca := makeCA(t)
	var downHits, goodHits atomic.Int32
	down := statusResponder(t, http.StatusServiceUnavailable, &downHits)
	good := responder(t, ca, ocsp.Good, &goodHits)
	leaf := leafWithResponders(t, ca, 42, down.URL+"/a", down.URL+"/b", good.URL)

	c := newChecker([]*x509.Certificate{ca.Cert}, revocation.WithMaxAttempts(1))
	require.NoError(t, c.Check(leaf))
	require.Equal(t, int32(2), downHits.Load())
}

// A responder URL the checker may not contact is skipped, not requested, and
// does not hide a usable one; with none usable the first refusal is reported.
func TestRefusedResponderURLIsSkipped(t *testing.T) {
	t.Parallel()
	ca := makeCA(t)
	var hits atomic.Int32
	good := responder(t, ca, ocsp.Good, &hits)

	leaf := leafWithResponders(t, ca, 42, "ldap://ocsp.example.org/", good.URL)
	require.NoError(t, newChecker([]*x509.Certificate{ca.Cert}).Check(leaf))
	require.Equal(t, int32(1), hits.Load())

	refused := leafWithResponders(t, ca, 43, "ldap://ocsp.example.org/", good.URL)
	c := newChecker([]*x509.Certificate{ca.Cert}, revocation.WithAllowedResponders("ocsp.example.org"))
	require.ErrorIs(t, c.Check(refused), revocation.ErrResponderNotAllowed)
	require.Equal(t, int32(1), hits.Load())
}
