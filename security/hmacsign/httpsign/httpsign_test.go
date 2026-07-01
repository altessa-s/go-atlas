// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package httpsign_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/security/hmacsign"
	"github.com/altessa-s/go-atlas/security/hmacsign/httpsign"
)

const secret = "whsec_test"

func signedRequest(t *testing.T, body string) *http.Request {
	t.Helper()
	header := hmacsign.NewSigner(hmacsign.GitHub(), []byte(secret)).Sign([]byte(body))
	req := httptest.NewRequest(http.MethodPost, "/webhook", strings.NewReader(body))
	req.Header.Set(hmacsign.GitHub().HeaderName(), header)
	return req
}

func TestMiddlewarePassesAuthentic(t *testing.T) {
	t.Parallel()

	v := hmacsign.NewVerifier(hmacsign.GitHub(), []byte(secret))
	var seen string
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body) // body must still be readable downstream
		seen = string(b)
		w.WriteHeader(http.StatusOK)
	})

	rr := httptest.NewRecorder()
	httpsign.Middleware(v)(next).ServeHTTP(rr, signedRequest(t, "hello"))

	require.Equal(t, http.StatusOK, rr.Code)
	require.Equal(t, "hello", seen)
}

func TestMiddlewareRejectsTampered(t *testing.T) {
	t.Parallel()

	v := hmacsign.NewVerifier(hmacsign.GitHub(), []byte(secret))
	req := signedRequest(t, "hello")
	req.Body = io.NopCloser(strings.NewReader("tampered")) // body no longer matches the header

	called := false
	next := http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true })

	rr := httptest.NewRecorder()
	httpsign.Middleware(v)(next).ServeHTTP(rr, req)

	require.False(t, called)
	require.Equal(t, http.StatusUnauthorized, rr.Code)
}

func TestMiddlewareRejectsOversizedBody(t *testing.T) {
	t.Parallel()

	v := hmacsign.NewVerifier(hmacsign.GitHub(), []byte(secret))
	req := httptest.NewRequest(http.MethodPost, "/webhook", strings.NewReader("way too long"))

	rr := httptest.NewRecorder()
	httpsign.Middleware(v, httpsign.WithMaxBytes(4))(
		http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}),
	).ServeHTTP(rr, req)

	require.Equal(t, http.StatusRequestEntityTooLarge, rr.Code)
}

func TestVerifyReturnsBody(t *testing.T) {
	t.Parallel()

	v := hmacsign.NewVerifier(hmacsign.GitHub(), []byte(secret))
	body, err := httpsign.Verify(v, signedRequest(t, "payload"))
	require.NoError(t, err)
	require.Equal(t, "payload", string(body))
}

func TestVerifyOversizedBody(t *testing.T) {
	t.Parallel()

	v := hmacsign.NewVerifier(hmacsign.GitHub(), []byte(secret))
	req := httptest.NewRequest(http.MethodPost, "/webhook", strings.NewReader("too long"))
	_, err := httpsign.Verify(v, req, httpsign.WithMaxBytes(2))
	require.ErrorIs(t, err, httpsign.ErrBodyTooLarge)
}

func TestCustomErrorHandler(t *testing.T) {
	t.Parallel()

	v := hmacsign.NewVerifier(hmacsign.GitHub(), []byte(secret))
	req := signedRequest(t, "hello")
	req.Body = io.NopCloser(strings.NewReader("tampered"))

	handler := httpsign.Middleware(v, httpsign.WithErrorHandler(
		func(w http.ResponseWriter, _ *http.Request, err error) {
			require.ErrorIs(t, err, hmacsign.ErrSignatureMismatch)
			w.WriteHeader(http.StatusForbidden)
		}),
	)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	require.Equal(t, http.StatusForbidden, rr.Code)
}
