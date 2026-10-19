// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package idempotency

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// send serves one request with method and target, carrying header as the key
// header ("" for none).
func send(t *testing.T, h http.Handler, method, target, header string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), method, target, strings.NewReader("{}"))
	if header != "" {
		req.Header.Set(DefaultIdempotencyKeyHeader, header)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// The key comes from the query parameter when it is set (such as
// request_id) and from the header otherwise.
func TestKeyFromQueryParam(t *testing.T) {
	t.Parallel()

	h := Middleware(newKeeper(), WithKeyQueryParam("requestId"), WithKeyFormatValidator(OpaqueKeyValidator(0)),
		WithRequestFingerprint(false))(echo)

	require.Equal(t, http.StatusCreated, send(t, h, http.MethodPost, "/orders?requestId=client-1", "").Code)
	require.Equal(t, http.StatusUnprocessableEntity, send(t, h, http.MethodPost, "/orders?requestId=client-1", "").Code)
	require.Equal(t, http.StatusUnprocessableEntity, send(t, h, http.MethodPost, "/orders?requestId=client-1", "other").Code,
		"the query parameter takes precedence over the header")

	require.Equal(t, http.StatusCreated, send(t, h, http.MethodPost, "/orders?requestId=", "header-key").Code,
		"an empty parameter falls back to the header")
	require.Equal(t, http.StatusUnprocessableEntity, send(t, h, http.MethodPost, "/orders", "header-key").Code)

	rec := send(t, h, http.MethodPost, "/orders?requestId=has%20space", "")
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Contains(t, rec.Body.String(), "invalid format")
}

func TestKeyFormatValidators(t *testing.T) {
	t.Parallel()

	require.Equal(t, http.StatusBadRequest, send(t, Middleware(newKeeper())(echo), http.MethodPost, "/o", "client-1").Code,
		"the default validator wants a UUID v4")

	pattern := Middleware(newKeeper(), WithKeyFormatValidator(PatternKeyValidator(regexp.MustCompile(`req-\d+`), 0)))(echo)
	require.Equal(t, http.StatusCreated, send(t, pattern, http.MethodPost, "/o", "req-1").Code)
	require.Equal(t, http.StatusBadRequest, send(t, pattern, http.MethodPost, "/o", "req-1x").Code)
}

func TestRequestClassifier(t *testing.T) {
	t.Parallel()

	classify := func(r *http.Request) Policy {
		switch r.URL.Path {
		case "/payments":
			return PolicyMandatory
		case "/webhooks":
			return PolicySkip
		case "/drafts":
			return PolicyOptional
		default:
			return PolicyDefault
		}
	}
	h := Middleware(newKeeper(), WithEnforceMandatory(true), WithRequestClassifier(classify))(echo)

	require.Equal(t, http.StatusBadRequest, send(t, h, http.MethodPost, "/payments", "").Code)
	require.Equal(t, http.StatusCreated, send(t, h, http.MethodPost, "/webhooks", "not-a-uuid").Code,
		"a skipped request does not even validate the key")
	require.Equal(t, http.StatusCreated, send(t, h, http.MethodPost, "/drafts", "").Code, "optional overrides enforceMandatory")
	require.Equal(t, http.StatusBadRequest, send(t, h, http.MethodPost, "/other", "").Code, "default follows enforceMandatory")

	called := false
	safe := Middleware(newKeeper(), WithRequestClassifier(func(*http.Request) Policy { called = true; return PolicyMandatory }))(echo)
	require.Equal(t, http.StatusCreated, send(t, safe, http.MethodGet, "/payments", "").Code)
	require.False(t, called, "safe methods are never classified")
}
