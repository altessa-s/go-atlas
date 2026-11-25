// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package gitlab

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/testhelpers"

	corehash "github.com/altessa-s/go-atlas-core/encoding/hash"
)

const testPolicy = "package authz\ndefault allow := false\n"

// newFakeGitLab serves a one-file repository tree and its raw content over
// TLS, the way the GitLab REST API does.
func newFakeGitLab(t *testing.T, files map[string]string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/projects/42/repository/commits/main", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}`))
	})
	mux.HandleFunc("/api/v4/projects/42/repository/tree", func(w http.ResponseWriter, _ *http.Request) {
		nodes := make([]map[string]string, 0, len(files))
		for name := range files {
			nodes = append(nodes, map[string]string{"id": "sha-" + name, "name": name, "type": "blob", "path": "policies/" + name})
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(nodes)
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
	return srv
}

func newFakeSource(t *testing.T, srv *httptest.Server, opts ...Option) *Source {
	t.Helper()
	base := []Option{
		WithHTTPClient(srv.Client()),
		WithEndpoint(srv.URL),
		WithToken("test-token"),
		WithProjectID(42),
		WithDir("policies"),
	}
	source, err := New(append(base, opts...)...)
	require.NoError(t, err)
	t.Cleanup(func() { _ = source.Close() })
	return source
}

func TestNew_RejectsPlainHTTPEndpoint(t *testing.T) {
	t.Parallel()

	base := []Option{WithHTTPClient(&http.Client{}), WithToken("test-token"), WithProjectID(42)}

	_, err := New(append(base, WithEndpoint("http://gitlab.example.com"))...)
	require.ErrorIs(t, err, ErrInsecureEndpoint)

	_, err = New(append(base, WithEndpoint("gitlab.example.com"))...)
	require.ErrorIs(t, err, ErrInsecureEndpoint)

	source, err := New(append(base, WithEndpoint("http://gitlab.example.com"), WithAllowInsecureHTTP())...)
	require.NoError(t, err)
	require.NoError(t, source.Close())
}

func TestFetch_VerifiesChecksums(t *testing.T) {
	t.Parallel()

	srv := newFakeGitLab(t, map[string]string{"authz.rego": testPolicy})
	digest := corehash.SHA256HexBytes([]byte(testPolicy))

	tests := []struct {
		name      string
		checksums map[string]string
		wantErr   error
	}{
		{name: "match", checksums: map[string]string{"authz.rego": digest}},
		{name: "mismatch", checksums: map[string]string{"authz.rego": strings.Repeat("0", 64)}, wantErr: ErrChecksumMismatch},
		{name: "unexpected file", checksums: map[string]string{}, wantErr: ErrUnexpectedPolicyFile},
		{name: "missing file", checksums: map[string]string{"authz.rego": digest, "other.rego": digest}, wantErr: ErrMissingPolicyFile},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			bundle, err := newFakeSource(t, srv, WithChecksums(tc.checksums)).Fetch(t.Context())
			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, testPolicy, string(bundle.Modules["authz.rego"]))
		})
	}
}

// Verification stays optional, but a source built without a manifest says so
// once, at construction.
func TestNew_WarnsWithoutChecksums(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		opts      []Option
		wantWarns int
	}{
		{name: "no manifest", wantWarns: 1},
		{name: "manifest", opts: []Option{WithChecksums(map[string]string{"authz.rego": strings.Repeat("0", 64)})}},
		{name: "empty manifest rejects every file", opts: []Option{WithChecksums(map[string]string{})}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var logs bytes.Buffer
			base := []Option{
				WithHTTPClient(&http.Client{}),
				WithEndpoint("https://gitlab.example.com"),
				WithToken("test-token"),
				WithProjectID(42),
				WithLogger(slog.New(slog.NewTextHandler(&logs, nil))),
			}
			source, err := New(append(base, tc.opts...)...)
			require.NoError(t, err)
			require.NoError(t, source.Close())
			require.Equal(t, tc.wantWarns, strings.Count(logs.String(), "level=WARN"))
		})
	}
}

func TestFetch_RejectsOversizedFile(t *testing.T) {
	t.Parallel()

	srv := newFakeGitLab(t, map[string]string{"authz.rego": testPolicy})

	_, err := newFakeSource(t, srv, WithMaxFileBytes(8)).Fetch(t.Context())
	require.ErrorIs(t, err, ErrFileTooLarge)

	bundle, err := newFakeSource(t, srv, WithMaxFileBytes(int64(len(testPolicy)))).Fetch(t.Context())
	require.NoError(t, err)
	require.Len(t, bundle.Modules, 1)
}

func TestFetchRejectsUnresolvedCommit(t *testing.T) {
	t.Parallel()
	for _, response := range []string{`null`, `{"id":"main"}`, `{"id":"zzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzz"}`} {
		t.Run(response, func(t *testing.T) {
			t.Parallel()
			var paths []string
			client := &http.Client{Transport: testhelpers.RoundTripFunc(func(req *http.Request) (*http.Response, error) {
				paths = append(paths, req.URL.Path)
				return &http.Response{Request: req, StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(response))}, nil
			})}
			src, err := New(WithProjectID(42), WithToken("test"), WithEndpoint("https://gitlab.test"), WithHTTPClient(client))
			require.NoError(t, err)
			_, err = src.Fetch(t.Context())
			require.Error(t, err)
			require.Equal(t, []string{"/api/v4/projects/42/repository/commits/main"}, paths)
		})
	}
}
