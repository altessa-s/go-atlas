// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package std_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/transport/http/server/router/std"
)

// pathSeeds are request paths worth starting from: ordinary routes, and the
// traversal and encoding tricks that make one path look like another.
var pathSeeds = []string{
	"/public",
	"/admin",
	"/admin/",
	"/admin/../public",
	"/public/../admin",
	"/./admin",
	"//admin",
	"/%61dmin",
	"/%2e%2e/admin",
	"/ADMIN",
	"/admin%00",
	"",
	"/",
}

// FuzzRoutingNeverReachesAnUnregisteredHandler is the routing oracle: a request
// may only ever be served by a handler whose pattern it actually matches.
//
// A router is an authorization boundary in practice — middleware is attached
// per route, so a request that reaches /admin's handler through a path the
// operator did not register also skipped whatever guard /admin carries. Path
// traversal and percent-encoding are how two spellings of one path stop
// agreeing, and net/http's own normalization sits between the wire and the mux.
//
// The check is not "which handler ran" but "if any ran, the path it claims to
// serve is the one the router resolved" — stated by having each handler report
// its own pattern.
func FuzzRoutingNeverReachesAnUnregisteredHandler(f *testing.F) {
	for _, seed := range pathSeeds {
		f.Add(seed, http.MethodGet)
		f.Add(seed, http.MethodPost)
	}

	f.Fuzz(func(t *testing.T, path, method string) {
		if path == "" || path[0] != '/' {
			t.Skip("net/http rejects a request target that is not a path")
		}
		if !isToken(method) {
			t.Skip("net/http rejects a method that is not a token")
		}
		if strings.ContainsAny(path, " \t") {
			// httptest.NewRequest builds a request *line*, which is
			// space-delimited; a target containing one is not something a real
			// connection delivers either.
			t.Skip("a request target cannot contain whitespace")
		}
		if _, err := url.ParseRequestURI(path); err != nil {
			// A target net/url cannot parse never reaches a handler on a real
			// connection either — the server rejects it before routing. Letting
			// it through here would only assert that httptest.NewRequest
			// panics, which it documents.
			t.Skip("not a parsable request target")
		}

		router := std.New()
		var served string
		for _, pattern := range []string{"/public", "/admin"} {
			router.HandleFunc(pattern, func(w http.ResponseWriter, _ *http.Request) {
				served = pattern
				w.WriteHeader(http.StatusOK)
			})
		}
		router.Initialize()

		req := httptest.NewRequestWithContext(t.Context(), method, path, http.NoBody)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if served == "" {
			return // Nothing matched, which is always a safe answer.
		}

		// The handler that ran must be the one whose pattern the resolved path
		// names — anything else means two spellings reached one route.
		require.Equal(t, served, req.URL.Path,
			"%q was served by the handler registered for %q", path, served)
	})
}

// isToken reports whether s is a valid HTTP method token, which is what
// httptest.NewRequest requires before the router is ever consulted.
func isToken(s string) bool {
	if s == "" {
		return false
	}
	for i := range len(s) {
		c := s[i]
		switch {
		case c >= 'A' && c <= 'Z', c >= 'a' && c <= 'z', c >= '0' && c <= '9':
		case c == '-' || c == '_' || c == '.' || c == '!' || c == '~' || c == '*' || c == '\'':
		default:
			return false
		}
	}
	return true
}
