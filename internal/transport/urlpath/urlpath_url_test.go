// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package urlpath_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/transport/urlpath"
)

func TestIsCanonicalURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		target string
		want   bool
	}{
		{"/", true},
		{"/v1/healthz", true},
		{"/v1/healthz/", true},
		{"/users/al%20ice", true},
		{"/users/caf%C3%A9", true},
		{"/users/a%5Cb", true},
		{"/users/a%252Fb", true},
		{"/users/50%25", true},
		{"/v1%2Fhealthz", false},
		{"/v1%2fhealthz", false},
		{"/users/alice%2Fadmin", false},
		{"/v1/healthz%2F", false},
		{"/%2Fadmin", false},
		{"/a/%2E%2E/admin", false},
		{"/a/%2e%2e/admin", false},
		{"/a/.%2E/admin", false},
		{"/%2e/admin", false},
		{"/users/%2E%2E", false},
		{"//admin", false},
		{"/public/../admin", false},
	}
	for _, tc := range tests {
		t.Run(tc.target, func(t *testing.T) {
			t.Parallel()
			u, err := url.ParseRequestURI(tc.target)
			require.NoError(t, err)
			require.Equal(t, tc.want, urlpath.IsCanonicalURL(u))
		})
	}
}

// A RawPath that does not encode Path is ignored by the routers, which fall
// back to the default encoding of Path; an encoded slash in it still makes
// the request non-canonical.
func TestIsCanonicalURL_RawPathMismatch(t *testing.T) {
	t.Parallel()

	require.True(t, urlpath.IsCanonicalURL(&url.URL{Path: "/v1/healthz", RawPath: "/something/else"}))
	require.False(t, urlpath.IsCanonicalURL(&url.URL{Path: "/v1/healthz", RawPath: "/other%2Fpath"}))
	require.False(t, urlpath.IsCanonicalURL(&url.URL{Path: "/v1/../healthz", RawPath: "/v1/healthz"}))
}

const maxDepth = 8

// depthRouters returns routers that match the escaped path, each with one
// wildcard route per depth; the handler reports the depth it was registered
// for and, where the router decodes them, the segments it matched.
func depthRouters() map[string]func(w *dispatch) http.Handler {
	names := []string{"a", "b", "c", "d", "e", "f", "g", "h"}

	return map[string]func(*dispatch) http.Handler{
		"net/http.ServeMux": func(d *dispatch) http.Handler {
			m := http.NewServeMux()
			for depth := 1; depth <= maxDepth; depth++ {
				pattern := "/{" + strings.Join(names[:depth], "}/{") + "}"
				m.HandleFunc(pattern, func(_ http.ResponseWriter, r *http.Request) {
					d.depth = depth
					for _, n := range names[:depth] {
						d.segments = append(d.segments, r.PathValue(n))
					}
				})
			}
			return m
		},
		"gorilla/mux UseEncodedPath": func(d *dispatch) http.Handler {
			m := mux.NewRouter().UseEncodedPath()
			for depth := 1; depth <= maxDepth; depth++ {
				pattern := "/{" + strings.Join(names[:depth], "}/{") + "}"
				m.HandleFunc(pattern, func(http.ResponseWriter, *http.Request) { d.depth = depth })
			}
			return m
		},
		"gorilla/mux": func(d *dispatch) http.Handler {
			m := mux.NewRouter()
			for depth := 1; depth <= maxDepth; depth++ {
				pattern := "/{" + strings.Join(names[:depth], "}/{") + "}"
				m.HandleFunc(pattern, func(_ http.ResponseWriter, r *http.Request) {
					d.depth = depth
					for _, n := range names[:depth] {
						d.segments = append(d.segments, mux.Vars(r)[n])
					}
				})
			}
			return m
		},
	}
}

type dispatch struct {
	depth    int
	segments []string
}

// For a canonical request path every router, whether it matches the decoded
// or the escaped path, dispatches on exactly the segments of the decoded path
// a policy is looked up for.
func FuzzIsCanonicalURL_AgreesWithRouters(f *testing.F) {
	for _, seed := range []string{
		"/v1/healthz", "/v1%2Fhealthz", "/v1%2fhealthz", "/users/alice%2Fadmin", "/a/%2E%2E/admin", "/%2e/admin",
		"/users/a%5Cb", "/users/al%20ice", "/users/a%252Fb", "/users/%2E%2E", "//admin", "/a/./b", "/x%2F..%2Fadmin",
	} {
		f.Add(seed)
	}
	routers := depthRouters()

	f.Fuzz(func(t *testing.T, target string) {
		if !strings.HasPrefix(target, "/") || strings.HasSuffix(target, "/") {
			return
		}
		u, err := url.ParseRequestURI(target)
		if err != nil || u.Path == "" || strings.HasSuffix(u.Path, "/") || !urlpath.IsCanonicalURL(u) {
			return
		}
		want := strings.Split(u.Path[1:], "/")
		if len(want) > maxDepth {
			return
		}

		for name, newRouter := range routers {
			var got dispatch
			req := &http.Request{Method: http.MethodGet, URL: u, Host: "example.com", Header: http.Header{}}
			rec := httptest.NewRecorder()
			newRouter(&got).ServeHTTP(rec, req)

			require.Equalf(t, http.StatusOK, rec.Code, "%s must serve %q, not redirect or miss", name, target)
			require.Equalf(t, len(want), got.depth, "%s matched another number of segments for %q", name, target)
			if got.segments != nil {
				require.Equalf(t, want, got.segments, "%s matched other segments for %q", name, target)
			}
		}
	})
}
