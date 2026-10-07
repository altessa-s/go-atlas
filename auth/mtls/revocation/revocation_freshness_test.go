// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package revocation_test

import (
	"crypto"
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

// windowResponder serves a signed response with the given status and validity
// window for the requested serial, counting hits.
func windowResponder(t *testing.T, ca *testhelpers.CA, status int, thisUpdate, nextUpdate time.Time, hits *atomic.Int32) *httptest.Server {
	t.Helper()
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
		der, err := ocsp.CreateResponse(ca.Cert, ca.Cert, ocsp.Response{
			Status:       status,
			SerialNumber: req.SerialNumber,
			ThisUpdate:   thisUpdate,
			NextUpdate:   nextUpdate,
			RevokedAt:    thisUpdate,
			IssuerHash:   crypto.SHA256,
		}, ca.Key)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		_, _ = w.Write(der)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestCheckRejectsStaleGoodResponse(t *testing.T) {
	t.Parallel()

	now := time.Now()
	tests := []struct {
		name       string
		thisUpdate time.Time
		nextUpdate time.Time
	}{
		{"nextUpdate elapsed", now.Add(-2 * time.Hour), now.Add(-time.Hour)},
		{"thisUpdate in the future", now.Add(time.Hour), now.Add(2 * time.Hour)},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ca := makeCA(t)
			var hits atomic.Int32
			srv := windowResponder(t, ca, ocsp.Good, tc.thisUpdate, tc.nextUpdate, &hits)
			leaf := makeLeaf(t, ca, 42, srv.URL)

			closed := revocation.New([]*x509.Certificate{ca.Cert}, revocation.WithFailMode(revocation.FailClosed))
			require.ErrorIs(t, closed.Check(leaf), revocation.ErrStaleResponse)
			require.ErrorIs(t, closed.Check(leaf), revocation.ErrStaleResponse)
			require.Equal(t, int32(2), hits.Load(), "a stale response must not be cached")

			open := revocation.New([]*x509.Certificate{ca.Cert}, revocation.WithFailMode(revocation.FailOpen))
			require.NoError(t, open.Check(leaf), "fail-open keeps accepting an indeterminate status")
		})
	}
}

func TestCheckAcceptsGoodResponseWithinSkew(t *testing.T) {
	t.Parallel()
	ca := makeCA(t)
	var hits atomic.Int32
	now := time.Now()
	skew := revocation.DefaultClockSkew
	srv := windowResponder(t, ca, ocsp.Good, now.Add(skew/2), now.Add(-skew/2), &hits)
	leaf := makeLeaf(t, ca, 42, srv.URL)

	c := revocation.New([]*x509.Certificate{ca.Cert}, revocation.WithFailMode(revocation.FailClosed))
	require.NoError(t, c.Check(leaf))

	strict := revocation.New([]*x509.Certificate{ca.Cert},
		revocation.WithFailMode(revocation.FailClosed), revocation.WithClockSkew(0))
	require.ErrorIs(t, strict.Check(leaf), revocation.ErrStaleResponse)
}

func TestCheckHonorsStaleRevokedResponse(t *testing.T) {
	t.Parallel()
	ca := makeCA(t)
	var hits atomic.Int32
	now := time.Now()
	srv := windowResponder(t, ca, ocsp.Revoked, now.Add(-2*time.Hour), now.Add(-time.Hour), &hits)
	leaf := makeLeaf(t, ca, 42, srv.URL)

	err := revocation.New([]*x509.Certificate{ca.Cert}, revocation.WithFailMode(revocation.FailOpen)).Check(leaf)
	require.ErrorIs(t, err, coremtls.ErrRevoked)
}
