// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/auth/opa/sources/gitlab"

	corehash "github.com/altessa-s/go-atlas-core/encoding/hash"
	s3source "github.com/altessa-s/go-atlas/auth/opa/sources/s3"
	authconfig "github.com/altessa-s/go-atlas/config/auth"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
)

// White-box access is needed to build a source without fetching through the
// SSRF-protected client, which refuses the loopback test servers.

const integrityPolicy = "package authz\ndefault allow := false\n"

func TestGitLabSource_AllowInsecureHTTP(t *testing.T) {
	t.Parallel()

	gl := &authconfig.OPAGitLab{Endpoint: "http://gitlab.example.com", Token: "tok", ProjectID: 42}
	b := New(&authconfig.OPA{Source: authconfig.OPASourceGitLab, GitLab: gl})
	_, err := b.buildGitLabSource()
	require.ErrorIs(t, err, gitlab.ErrInsecureEndpoint)

	gl.AllowInsecureHTTP = true
	source, err := b.buildGitLabSource()
	require.NoError(t, err)
	require.NoError(t, source.Close())
}

func TestGitLabSource_Checksums(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/projects/42/repository/tree", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]map[string]string{
			{"id": "sha-authz", "name": "authz.rego", "type": "blob", "path": "policies/authz.rego"},
		})
	})
	mux.HandleFunc("/api/v4/projects/42/repository/files/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/projects/42/repository/files/policies/authz.rego/raw" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(integrityPolicy))
	})
	srv := httptest.NewTLSServer(mux)
	t.Cleanup(srv.Close)

	for _, tc := range []struct {
		name    string
		digest  string
		wantErr error
	}{
		{name: "match", digest: corehash.SHA256HexBytes([]byte(integrityPolicy))},
		{name: "mismatch", digest: strings.Repeat("0", 64), wantErr: gitlab.ErrChecksumMismatch},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			b := New(&authconfig.OPA{Source: authconfig.OPASourceGitLab, GitLab: &authconfig.OPAGitLab{
				Endpoint: srv.URL, Token: "tok", ProjectID: 42, Dir: "policies",
				Checksums: map[string]string{"authz.rego": tc.digest},
			}})
			opts, err := b.gitLabOptions()
			require.NoError(t, err)
			source, err := gitlab.New(append(opts, gitlab.WithHTTPClient(srv.Client()))...)
			require.NoError(t, err)
			t.Cleanup(func() { _ = source.Close() })

			_, err = source.Fetch(t.Context())
			if tc.wantErr == nil {
				require.NoError(t, err)
				return
			}
			require.ErrorIs(t, err, tc.wantErr)
		})
	}
}

func TestS3Source_Checksums(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name    string
		digest  string
		wantErr error
	}{
		{name: "match", digest: corehash.SHA256HexBytes([]byte(integrityPolicy))},
		{name: "mismatch", digest: strings.Repeat("0", 64), wantErr: s3source.ErrChecksumMismatch},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			client := fakeS3{"policies/authz.rego": []byte(integrityPolicy)}
			b := New(&authconfig.OPA{Source: authconfig.OPASourceS3, FileExtensions: []string{".rego"}, S3: &authconfig.OPAS3{
				Bucket: "bucket", Prefix: "policies/",
				Checksums: map[string]string{"authz.rego": tc.digest},
			}}).UseS3Client(client)
			source, err := b.buildS3Source(t.Context())
			require.NoError(t, err)
			t.Cleanup(func() { _ = source.Close() })

			_, err = source.Fetch(t.Context())
			if tc.wantErr == nil {
				require.NoError(t, err)
				return
			}
			require.ErrorIs(t, err, tc.wantErr)
		})
	}
}

// fakeS3 serves its objects, keyed by object key, as one listing page.
type fakeS3 map[string][]byte

func (f fakeS3) ListObjectsV2(_ context.Context, in *awss3.ListObjectsV2Input, _ ...func(*awss3.Options)) (*awss3.ListObjectsV2Output, error) {
	out := &awss3.ListObjectsV2Output{IsTruncated: new(false)}
	for key := range f {
		if in.Prefix == nil || strings.HasPrefix(key, *in.Prefix) {
			out.Contents = append(out.Contents, types.Object{Key: new(key)})
		}
	}
	return out, nil
}

func (f fakeS3) GetObject(_ context.Context, in *awss3.GetObjectInput, _ ...func(*awss3.Options)) (*awss3.GetObjectOutput, error) {
	content, ok := f[*in.Key]
	if !ok {
		return nil, errors.New("NoSuchKey")
	}
	return &awss3.GetObjectOutput{Body: io.NopCloser(bytes.NewReader(content))}, nil
}
