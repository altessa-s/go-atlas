// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package securityheaders

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/transport/http/server/middlewares/defaults"
)

func serve(t *testing.T, h http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec
}

func TestCacheControl(t *testing.T) {
	t.Parallel()

	ok := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})
	require.Equal(t, "no-store", serve(t, Middleware(WithCacheControl("no-store"))(ok), "/").Header().Get(HeaderCacheControl))
	require.Empty(t, serve(t, Middleware()(ok), "/").Header().Get(HeaderCacheControl), "unset by default")

	override := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set(HeaderCacheControl, "public, max-age=60")
	})
	require.Equal(t, "public, max-age=60",
		serve(t, Middleware(WithCacheControl("no-store"))(override), "/").Header().Get(HeaderCacheControl),
		"a handler overrides the default")
}

func TestNoDefaultIgnores(t *testing.T) {
	t.Parallel()

	ok := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})
	require.Empty(t, serve(t, Middleware()(ok), "/healthz").Header().Get(HeaderXFrameOptions),
		"health paths are skipped by default")
	require.Equal(t, "DENY", serve(t, Middleware(WithNoDefaultIgnores())(ok), "/healthz").Header().Get(HeaderXFrameOptions))

	explicit := Middleware(
		WithNoDefaultIgnores(),
		WithIgnorePatterns(regexp.MustCompile(`^/static/`)),
		WithIgnorePaths("/healthz"),
	)(ok)
	require.Empty(t, serve(t, explicit, "/static/app.js").Header().Get(HeaderXFrameOptions), "explicit patterns stay")
	require.Empty(t, serve(t, explicit, "/healthz").Header().Get(HeaderXFrameOptions), "explicit paths stay")
	require.Equal(t, "DENY", serve(t, explicit, "/metrics").Header().Get(HeaderXFrameOptions))

	// The default patterns passed explicitly are kept.
	explicitDefaults := Middleware(WithNoDefaultIgnores(), WithIgnorePatterns(defaults.IgnorePatterns...))(ok)
	require.Empty(t, serve(t, explicitDefaults, "/healthz").Header().Get(HeaderXFrameOptions))
}
