// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package std_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/transport/http/server/router"
	"github.com/altessa-s/go-atlas/transport/http/server/router/std"
)

// serve sends method path to h and returns the status and body.
func serve(t *testing.T, h http.Handler, method, path string) (int, string) {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(method, path, nil))
	return rec.Code, rec.Body.String()
}

// reply answers with body.
func reply(body string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(body)) }
}

// tag returns middleware that appends name to the X-Seen header.
func tag(name string) router.Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Add("X-Seen", name)
			next.ServeHTTP(w, r)
		})
	}
}

// A subrouter registers its routes under its full prefix on the shared mux,
// and two subrouters of one prefix (pprof and metrics under /internal) coexist.
func TestSubrouterFullPaths(t *testing.T) {
	t.Parallel()
	r := std.New()
	r.PathPrefix("/internal").Subrouter().PathPrefix("/pprof").Subrouter().HandleFunc("/heap", reply("heap"))
	metrics := r.PathPrefix("/internal").Subrouter().PathPrefix("/metrics").Subrouter()
	metrics.Handle("/", reply("metrics"))
	api := r.PathPrefix("/api/").Subrouter()
	api.HandleFunc("/users/{id}", func(w http.ResponseWriter, req *http.Request) {
		_, _ = w.Write([]byte("user " + req.PathValue("id")))
	}).Methods(http.MethodGet)
	api.HandleFunc("DELETE /users/{id}", reply("deleted"))

	for _, tc := range []struct {
		method, path string
		code         int
		body         string
	}{
		{http.MethodGet, "/internal/pprof/heap", http.StatusOK, "heap"},
		{http.MethodGet, "/internal/metrics/", http.StatusOK, "metrics"},
		{http.MethodGet, "/internal/metrics/anything", http.StatusOK, "metrics"},
		{http.MethodGet, "/internal/metrics", http.StatusTemporaryRedirect, ""},
		{http.MethodGet, "/api/users/7", http.StatusOK, "user 7"},
		{http.MethodPost, "/api/users/7", http.StatusMethodNotAllowed, ""},
		{http.MethodDelete, "/api/users/7", http.StatusOK, "deleted"},
		{http.MethodGet, "/heap", http.StatusNotFound, ""},
	} {
		code, body := serve(t, r, tc.method, tc.path)
		require.Equal(t, tc.code, code, "%s %s", tc.method, tc.path)
		if tc.body != "" {
			require.Equal(t, tc.body, body, "%s %s", tc.method, tc.path)
		}
	}
}

// Middleware of a subrouter wraps its own routes (and its subrouters'), the
// root's wraps every request, and any router of the tree serves the tree.
func TestSubrouterMiddlewareScope(t *testing.T) {
	t.Parallel()
	r := std.New()
	r.Use(tag("root"))
	r.HandleFunc("/public", reply("public"))
	admin := r.PathPrefix("/admin").Subrouter()
	admin.Use(tag("admin"))
	admin.HandleFunc("/panel", reply("panel"))
	deep := admin.Subrouter()
	deep.Use(tag("deep"))
	deep.HandleFunc("/deep", reply("deep"))

	for _, tc := range []struct {
		path string
		seen []string
	}{
		{"/public", []string{"root"}},
		{"/admin/panel", []string{"root", "admin"}},
		{"/admin/deep", []string{"root", "admin", "deep"}},
		{"/missing", []string{"root"}},
	} {
		rec := httptest.NewRecorder()
		admin.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tc.path, nil))
		require.Equal(t, tc.seen, rec.Header().Values("X-Seen"), tc.path)
	}
}

// Registration on any router of an initialized tree panics.
func TestSubrouterSealedWithTree(t *testing.T) {
	t.Parallel()
	r := std.New()
	sub := r.PathPrefix("/v1").Subrouter()
	sub.HandleFunc("/x", reply("x"))
	r.Initialize()
	require.Panics(t, func() { sub.HandleFunc("/y", reply("y")) })
	require.Panics(t, func() { sub.Use(tag("late")) })
}
