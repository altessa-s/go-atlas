// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package gitlab

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/auth/opa"
)

type treeNode struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"`
	Path string `json:"path"`
}

// newPagedGitLab serves tree pages (one page per slice element) and raw
// file contents keyed by file name.
func newPagedGitLab(t *testing.T, pages [][]treeNode, files map[string]string, treeStatus int) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var treeCalls atomic.Int32
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/projects/42/repository/tree", func(w http.ResponseWriter, r *http.Request) {
		treeCalls.Add(1)
		if treeStatus != 0 {
			w.WriteHeader(treeStatus)
			return
		}
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		page = max(page, 1)
		if page < len(pages) {
			w.Header().Set("X-Next-Page", strconv.Itoa(page+1))
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(pages[page-1])
	})
	mux.HandleFunc("/api/v4/projects/42/repository/files/", func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/api/v4/projects/42/repository/files/policies/"), "/raw")
		content, ok := files[name]
		if !ok {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(content))
	})
	srv := httptest.NewTLSServer(mux)
	t.Cleanup(srv.Close)
	return srv, &treeCalls
}

func blob(name string) treeNode {
	return treeNode{ID: "sha-" + name, Name: name, Type: "blob", Path: "policies/" + name}
}

func TestFetch_PaginatesAndLoadsData(t *testing.T) {
	t.Parallel()
	pages := [][]treeNode{
		{blob("a.rego"), {ID: "t1", Name: "sub", Type: "tree", Path: "policies/sub"}},
		{blob("roles.json"), blob("README.md")},
	}
	files := map[string]string{"a.rego": testPolicy, "roles.json": `{"admin":["alice"]}`, "README.md": "x"}
	srv, calls := newPagedGitLab(t, pages, files, 0)

	bundle, err := newFakeSource(t, srv, WithIncludeData(), WithLogger(slog.New(slog.DiscardHandler))).Fetch(t.Context())
	require.NoError(t, err)
	require.EqualValues(t, 2, calls.Load(), "both tree pages must be listed")
	require.Equal(t, map[string][]byte{"a.rego": []byte(testPolicy)}, bundle.Modules)
	require.Equal(t, map[string]any{"roles": map[string]any{"admin": []any{"alice"}}}, bundle.Data)
}

func TestFetch_IgnoresDataWithoutIncludeData(t *testing.T) {
	t.Parallel()
	srv, _ := newPagedGitLab(t, [][]treeNode{{blob("a.rego"), blob("roles.json")}},
		map[string]string{"a.rego": testPolicy, "roles.json": `{}`}, 0)

	bundle, err := newFakeSource(t, srv).Fetch(t.Context())
	require.NoError(t, err)
	require.Nil(t, bundle.Data)
	require.Len(t, bundle.Modules, 1)
}

func TestFetch_Errors(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		pages   [][]treeNode
		files   map[string]string
		status  int
		wantErr error
	}{
		{name: "tree listing fails", status: http.StatusInternalServerError},
		{name: "no policy files", pages: [][]treeNode{{blob("README.md")}}, files: map[string]string{"README.md": "x"}, wantErr: opa.ErrNoPolicyFiles},
		{name: "invalid json data", pages: [][]treeNode{{blob("a.rego"), blob("d.json")}}, files: map[string]string{"a.rego": testPolicy, "d.json": "{"}},
		{name: "policy download fails", pages: [][]treeNode{{blob("gone.rego")}}, files: map[string]string{}},
		{name: "data download fails", pages: [][]treeNode{{blob("a.rego"), blob("gone.json")}}, files: map[string]string{"a.rego": testPolicy}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			srv, _ := newPagedGitLab(t, tc.pages, tc.files, tc.status)
			_, err := newFakeSource(t, srv, WithIncludeData()).Fetch(t.Context())
			require.Error(t, err)
			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
			}
		})
	}
}

func TestOptions_StringAndPointerForms(t *testing.T) {
	t.Parallel()
	dir, ref, endpoint, token := " policies ", "main", "https://gitlab.example.com", "tok"
	blank := "  "
	base := []Option{WithHTTPClient(&http.Client{}), WithProjectID(7)}

	s, err := New(append(base,
		WithDir(&dir), WithRef(&ref), WithEndpoint(&endpoint), WithToken(&token),
		// Blank and nil values never override a set value.
		WithDir(blank), WithDir(&blank), WithDir((*string)(nil)),
		WithRef(blank), WithRef(&blank), WithRef((*string)(nil)),
		WithEndpoint(blank), WithEndpoint(&blank), WithEndpoint((*string)(nil)),
		WithToken(blank), WithToken(&blank), WithToken((*string)(nil)),
		WithLogger(nil), WithHTTPClient(nil), WithMaxFileBytes(0),
	)...)
	require.NoError(t, err)
	t.Cleanup(func() { _ = s.Close() })
	require.Equal(t, "gitlab:7/policies@main", s.Name())

	_, err = New(append(base, WithEndpoint(&blank), WithToken("t"))...)
	require.ErrorIs(t, err, ErrEndpointRequired)
	_, err = New(append(base, WithEndpoint(endpoint), WithToken(&blank))...)
	require.ErrorIs(t, err, ErrTokenRequired)
}

func TestNew_RejectsUnparsableEndpoint(t *testing.T) {
	t.Parallel()
	_, err := New(WithHTTPClient(&http.Client{}), WithProjectID(1), WithToken("t"), WithEndpoint("https://%zz"))
	require.ErrorIs(t, err, ErrInsecureEndpoint)
}
