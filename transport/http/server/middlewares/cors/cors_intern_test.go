// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package cors

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas-core/text/strings/intern"
)

// TestPreflight_DoesNotInternRequestValues pins that client-supplied preflight
// values never reach the process-wide string interner: an attacker could
// otherwise fill it with unique method and header names.
//
// Serial: it measures the size of the process-wide GlobalInterner, which a
// parallel test could grow concurrently.
func TestPreflight_DoesNotInternRequestValues(t *testing.T) {
	handler := Middleware(
		WithAllowedOrigins("https://example.com"),
		WithAllowedMethods("GET", "POST"),
		WithAllowedHeaders("Content-Type"),
	)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))

	before := intern.Default().Size()
	for i := range 64 {
		req := httptest.NewRequest(http.MethodOptions, "/", nil)
		req.Header.Set("Origin", "https://example.com")
		req.Header.Set(HeaderAccessControlRequestMethod, "m"+strconv.Itoa(i)+"-unique-method")
		req.Header.Set(HeaderAccessControlRequestHeaders, "X-Unique-"+strconv.Itoa(i))
		handler.ServeHTTP(httptest.NewRecorder(), req)
	}
	for i := range 64 {
		req := httptest.NewRequest(http.MethodOptions, "/", nil)
		req.Header.Set("Origin", "https://example.com")
		req.Header.Set(HeaderAccessControlRequestMethod, "post")
		req.Header.Set(HeaderAccessControlRequestHeaders, "X-Other-Unique-"+strconv.Itoa(i))
		handler.ServeHTTP(httptest.NewRecorder(), req)
	}

	require.Equal(t, before, intern.Default().Size())
}

func TestPreflight_MatchesCaseInsensitively(t *testing.T) {
	t.Parallel()

	handler := Middleware(
		WithAllowedOrigins("https://example.com"),
		WithAllowedMethods("GET", "POST"),
		WithAllowedHeaders("Content-Type"),
	)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))

	req := httptest.NewRequest(http.MethodOptions, "/", nil)
	req.Header.Set("Origin", "https://example.com")
	req.Header.Set(HeaderAccessControlRequestMethod, "post")
	req.Header.Set(HeaderAccessControlRequestHeaders, "CONTENT-TYPE")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	require.Equal(t, "https://example.com", rec.Header().Get(HeaderAccessControlAllowOrigin))
	require.NotEmpty(t, rec.Header().Get(HeaderAccessControlAllowMethods))
}
