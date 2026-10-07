// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package ocsp

import (
	"bytes"
	"compress/gzip"
	"crypto/tls"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/testhelpers"

	"golang.org/x/crypto/ocsp"
)

// responderSpec shapes the response served by testResponder.
type responderSpec struct {
	serial     *big.Int // nil: echo the requested serial
	thisUpdate time.Time
	nextUpdate time.Time
	gzipBody   []byte // non-nil: serve this gzip-encoded body instead of an OCSP response
}

// testResponder serves a CA-signed "good" OCSP response shaped by spec.
func testResponder(t *testing.T, ca *testhelpers.CA, spec responderSpec) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if spec.gzipBody != nil {
			w.Header().Set("Content-Encoding", "gzip")
			_, _ = w.Write(spec.gzipBody)
			return
		}
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
		serial := req.SerialNumber
		if spec.serial != nil {
			serial = spec.serial
		}
		der, err := ocsp.CreateResponse(ca.Cert, ca.Cert, ocsp.Response{
			Status:       ocsp.Good,
			SerialNumber: serial,
			ThisUpdate:   spec.thisUpdate,
			NextUpdate:   spec.nextUpdate,
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

// chainFor mints a leaf pointing at srv and returns it chained to its issuer.
func chainFor(t *testing.T, ca *testhelpers.CA, srv *httptest.Server) *tls.Certificate {
	t.Helper()
	leaf := ca.SignLeaf(t, testhelpers.WithOCSPServers(srv.URL))
	leaf.Certificate = append(leaf.Certificate, ca.Cert.Raw)
	return &leaf
}

func gzipBytes(t *testing.T, data []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	_, err := w.Write(data)
	require.NoError(t, err)
	require.NoError(t, w.Close())
	return buf.Bytes()
}

func TestGetOCSPStaple_ResponseValidation(t *testing.T) {
	t.Parallel()

	now := time.Now()
	tests := []struct {
		name    string
		spec    responderSpec
		wantErr error // nil: success expected
		anyErr  bool
	}{
		{
			name: "matching serial and fresh window",
			spec: responderSpec{thisUpdate: now.Add(-time.Minute), nextUpdate: now.Add(time.Hour)},
		},
		{
			name: "zero nextUpdate is accepted",
			spec: responderSpec{thisUpdate: now.Add(-time.Minute)},
		},
		{
			name: "within clock skew",
			spec: responderSpec{thisUpdate: now.Add(ocspClockSkew / 2), nextUpdate: now.Add(-ocspClockSkew / 2)},
		},
		{
			name:   "response for another serial",
			spec:   responderSpec{serial: big.NewInt(999999), thisUpdate: now.Add(-time.Minute), nextUpdate: now.Add(time.Hour)},
			anyErr: true,
		},
		{
			name:    "nextUpdate elapsed",
			spec:    responderSpec{thisUpdate: now.Add(-2 * time.Hour), nextUpdate: now.Add(-time.Hour)},
			wantErr: ErrStaleResponse,
		},
		{
			name:    "thisUpdate in the future",
			spec:    responderSpec{thisUpdate: now.Add(time.Hour), nextUpdate: now.Add(2 * time.Hour)},
			wantErr: ErrStaleResponse,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ca := testhelpers.NewCA(t)
			cert := chainFor(t, ca, testResponder(t, ca, tc.spec))

			soft := NewOCSPStapler()
			staple, err := soft.GetOCSPStaple(t.Context(), cert)

			if tc.wantErr == nil && !tc.anyErr {
				require.NoError(t, err)
				require.NotEmpty(t, staple)
				return
			}
			require.Error(t, err)
			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
			}
			require.Empty(t, staple)
			soft.mu.RLock()
			require.Empty(t, soft.cache, "a rejected response must not be cached")
			soft.mu.RUnlock()

			hard := NewOCSPStapler(WithFailureMode(FailureModeHard))
			stapled, err := StapleCertificate(t.Context(), hard, cert)
			require.Error(t, err, "hard mode must abort the handshake")
			require.Nil(t, stapled)
		})
	}
}

func TestGetOCSPStaple_GzipResponseBody(t *testing.T) {
	t.Parallel()

	t.Run("decoded body over the cap is rejected", func(t *testing.T) {
		t.Parallel()
		ca := testhelpers.NewCA(t)
		bomb := gzipBytes(t, make([]byte, maxOCSPResponseSize+1))
		cert := chainFor(t, ca, testResponder(t, ca, responderSpec{gzipBody: bomb}))

		_, err := NewOCSPStapler(WithCompression()).GetOCSPStaple(t.Context(), cert)
		require.Error(t, err)
	})

	t.Run("small gzip body is decoded and parsed", func(t *testing.T) {
		t.Parallel()
		ca := testhelpers.NewCA(t)
		leaf := ca.SignLeaf(t)
		now := time.Now()
		der, err := ocsp.CreateResponse(ca.Cert, ca.Cert, ocsp.Response{
			Status:       ocsp.Good,
			SerialNumber: leaf.Leaf.SerialNumber,
			ThisUpdate:   now.Add(-time.Minute),
			NextUpdate:   now.Add(time.Hour),
		}, ca.Key)
		require.NoError(t, err)
		srv := testResponder(t, ca, responderSpec{gzipBody: gzipBytes(t, der)})

		// Re-mint the leaf with the same serial pointing at the gzip responder.
		cert := ca.SignLeaf(t, testhelpers.WithSerial(leaf.Leaf.SerialNumber.Int64()), testhelpers.WithOCSPServers(srv.URL))
		cert.Certificate = append(cert.Certificate, ca.Cert.Raw)

		staple, err := NewOCSPStapler(WithCompression()).GetOCSPStaple(t.Context(), &cert)
		require.NoError(t, err)
		require.Equal(t, der, staple)
	})
}
