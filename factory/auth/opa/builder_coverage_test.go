// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"log/slog"
	"slices"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/observability/health"

	httpclient "github.com/altessa-s/go-atlas-net/http/client"
	netproxy "github.com/altessa-s/go-atlas-net/proxyconfig"
	s3source "github.com/altessa-s/go-atlas/auth/opa/sources/s3"
	authconfig "github.com/altessa-s/go-atlas/config/auth"
	proxyconfig "github.com/altessa-s/go-atlas/config/proxy"
	corefactory "github.com/altessa-s/go-atlas/factory/core"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
)

const adminPolicy = "package profiles.authz\nimport rego.v1\nallow if input.role == \"admin\"\n"

func requireAdminOnly(t *testing.T, cfg *authconfig.OPA, b *ManagerBuilder) {
	t.Helper()
	m, err := b.Build(t.Context())
	require.NoError(t, err)
	t.Cleanup(func() { _ = m.Close() })

	res, err := m.Evaluator().Evaluate(t.Context(), map[string]any{"role": "admin"})
	require.NoError(t, err)
	require.True(t, res.Allow)
	res, err = m.Evaluator().Evaluate(t.Context(), map[string]any{"role": "user"})
	require.NoError(t, err)
	require.False(t, res.Allow)
	require.Equal(t, cfg.WatchBundle, m.IsWatching())
}

func TestBuild_EmbedSource(t *testing.T) {
	t.Parallel()
	fsys := fstest.MapFS{"policies/authz.rego": {Data: []byte(adminPolicy)}}
	cfg := authconfig.DefaultOPA()
	cfg.Source = authconfig.OPASourceEmbed
	cfg.Query = "data.profiles.authz.allow"
	cfg.WatchBundle = true

	coord := health.New()
	t.Cleanup(coord.Close)
	requireAdminOnly(t, &cfg, New(&cfg).
		UseLogger(slog.New(slog.DiscardHandler)).
		UseHealthCoordinator(coord).
		UseEmbedFS(fsys, "policies"))
	require.Contains(t, slices.Collect(coord.ListServices()), "opa")
}

func TestBuild_EmbedSourceRequiresFS(t *testing.T) {
	t.Parallel()
	cfg := authconfig.DefaultOPA()
	cfg.Source = authconfig.OPASourceEmbed
	_, err := New(&cfg).UseDefaultLogger().Build(t.Context())
	require.ErrorIs(t, err, corefactory.ErrDependencyRequired)
}

func TestBuild_EmbedSourceEmptyFS(t *testing.T) {
	t.Parallel()
	cfg := authconfig.DefaultOPA()
	cfg.Source = authconfig.OPASourceEmbed
	_, err := New(&cfg).UseEmbedFS(fstest.MapFS{"x/readme.txt": {Data: []byte("x")}}, "x").Build(t.Context())
	require.Error(t, err)
}

func TestBuild_S3SourceWithInjectedClient(t *testing.T) {
	t.Parallel()
	cfg := authconfig.DefaultOPA()
	cfg.Source = authconfig.OPASourceS3
	cfg.Query = "data.profiles.authz.allow"
	cfg.S3 = &authconfig.OPAS3{Bucket: "bucket", Prefix: "policies/"}
	requireAdminOnly(t, &cfg, New(&cfg).UseS3Client(fakeS3{"policies/authz.rego": []byte(adminPolicy)}))
}

func TestBuild_S3SourceRequiresConfig(t *testing.T) {
	t.Parallel()
	b := New(&authconfig.OPA{Source: authconfig.OPASourceS3})
	_, err := b.buildS3Source(t.Context())
	require.ErrorIs(t, err, corefactory.ErrConfigRequired)
}

func TestBuild_InvalidQueryClosesSource(t *testing.T) {
	t.Parallel()
	cfg := filesystemConfig(t)
	cfg.Query = "data.profiles.authz.allow[" // does not parse
	_, err := New(cfg).Build(t.Context())
	require.Error(t, err)
}

func TestBuild_MissingBundleDirectory(t *testing.T) {
	t.Parallel()
	cfg := filesystemConfig(t)
	cfg.BundlePath = t.TempDir() + "/missing"
	_, err := New(cfg).Build(t.Context())
	require.Error(t, err)
}

func TestResolveS3Client_FromConfig(t *testing.T) {
	t.Parallel()
	b := New(&authconfig.OPA{})
	client, err := b.resolveS3Client(t.Context(), &authconfig.OPAS3{
		Bucket:    "bucket",
		Region:    "eu-west-1",
		Endpoint:  "https://s3.example.com",
		AccessKey: "AKID",
		SecretKey: "SECRET",
		PathStyle: true,
	})
	require.NoError(t, err)
	s3c, ok := client.(*awss3.Client)
	require.True(t, ok, "want *s3.Client, got %T", client)
	o := s3c.Options()
	require.Equal(t, "eu-west-1", o.Region)
	require.NotNil(t, o.BaseEndpoint)
	require.Equal(t, "https://s3.example.com", *o.BaseEndpoint)
	require.True(t, o.UsePathStyle)
	creds, err := o.Credentials.Retrieve(t.Context())
	require.NoError(t, err)
	require.Equal(t, "AKID", creds.AccessKeyID)
}

// The S3 source cannot see the endpoint of an injected client, so the factory
// enforces https on the endpoint it builds the client for: SigV4 protects the
// credentials, not the policy content, which a MITM on http could swap.
func TestResolveS3Client_RequiresHTTPSEndpoint(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name      string
		endpoint  string
		allowHTTP bool
		wantErr   bool
	}{
		{name: "https", endpoint: "https://minio.internal:9000"},
		{name: "http rejected", endpoint: "http://minio.internal:9000", wantErr: true},
		{name: "http with opt-in", endpoint: "http://minio.internal:9000", allowHTTP: true},
		{name: "schemeless rejected", endpoint: "minio.internal:9000", allowHTTP: true, wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			b := New(&authconfig.OPA{})
			_, err := b.resolveS3Client(t.Context(), &authconfig.OPAS3{
				Bucket:            "bucket",
				Region:            "eu-west-1",
				Endpoint:          tc.endpoint,
				AllowInsecureHTTP: tc.allowHTTP,
			})
			if tc.wantErr {
				require.ErrorIs(t, err, s3source.ErrInsecureEndpoint)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestResolveS3Client_RejectsProxy(t *testing.T) {
	t.Parallel()
	b := New(&authconfig.OPA{})
	_, err := b.resolveS3Client(t.Context(), &authconfig.OPAS3{
		Bucket: "bucket",
		Proxy:  &proxyconfig.Config{Mode: netproxy.ModeURL, URL: "http://proxy.example:8080"},
	})
	require.ErrorIs(t, err, httpclient.ErrSSRFUnsafeTransport)
}
