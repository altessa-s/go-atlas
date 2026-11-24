// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/stretchr/testify/require"

	egressrule "github.com/altessa-s/go-atlas-net/egressrule"
	httpclient "github.com/altessa-s/go-atlas-net/http/client"
	netproxy "github.com/altessa-s/go-atlas-net/proxyconfig"
	authconfig "github.com/altessa-s/go-atlas/config/auth"
	httpconfig "github.com/altessa-s/go-atlas/config/http"
	proxyconfig "github.com/altessa-s/go-atlas/config/proxy"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
)

func TestRejectsProxyWithSSRFProtection(t *testing.T) {
	t.Parallel()
	proxy := &proxyconfig.Config{Mode: netproxy.ModeURL, URL: "http://proxy.example:8080"}
	b := New(&authconfig.OPA{GitLab: &authconfig.OPAGitLab{Proxy: proxy}})
	_, err := b.buildGitLabSource()
	require.ErrorIs(t, err, httpclient.ErrSSRFUnsafeTransport)
	_, err = b.resolveS3Client(t.Context(), &authconfig.OPAS3{Proxy: proxy})
	require.ErrorIs(t, err, httpclient.ErrSSRFUnsafeTransport)
}

func TestManagerBuilder_Build_GitLabSSRF(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/projects/42/repository/tree", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `[{"id":"abc","name":"authz.rego","type":"blob","path":"authz.rego"}]`)
	})
	mux.HandleFunc("/api/v4/projects/42/repository/files/authz.rego/raw", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "package authz\nallow := true\n")
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	for _, tc := range []struct {
		name     string
		cfg      *httpconfig.ClientSSRF
		blocked  bool
		wantRule string
	}{
		{name: "absent", blocked: true},
		{name: "allowed CIDR", cfg: &httpconfig.ClientSSRF{AllowedCIDRs: []string{"127.0.0.0/8"}}},
		{name: "disabled", cfg: &httpconfig.ClientSSRF{Disabled: true}},
		{name: "matching deny rule", cfg: &httpconfig.ClientSSRF{
			AllowedCIDRs: []string{"127.0.0.0/8"},
			Deny:         []egressrule.Rule{{Host: "127.0.0.1"}},
		}, blocked: true, wantRule: "deny[0] host=127.0.0.1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			cfg := &authconfig.OPA{
				Source: authconfig.OPASourceGitLab,
				Query:  "data.authz.allow",
				GitLab: &authconfig.OPAGitLab{
					Endpoint:          srv.URL,
					Token:             "tok",
					ProjectID:         42,
					AllowInsecureHTTP: true,
					SSRF:              tc.cfg,
				},
			}
			m, err := New(cfg).Build(t.Context())
			if tc.blocked {
				require.ErrorIs(t, err, httpclient.ErrSSRFBlocked)
				require.Equal(t, tc.wantRule, httpclient.IsSSRFError(err).Rule)
				return
			}
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, m.Close()) })
		})
	}
}

func TestResolveS3Client_SSRF(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		_, _ = io.WriteString(w, `<ListBucketResult><IsTruncated>false</IsTruncated></ListBucketResult>`)
	}))
	t.Cleanup(srv.Close)

	for _, tc := range []struct {
		name    string
		cfg     *httpconfig.ClientSSRF
		wantErr error
	}{
		{name: "absent uses SDK client"},
		{name: "empty uses protected client", cfg: &httpconfig.ClientSSRF{}, wantErr: httpclient.ErrSSRFBlocked},
		{name: "allowed CIDR", cfg: &httpconfig.ClientSSRF{AllowedCIDRs: []string{"127.0.0.0/8"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			client, err := New(&authconfig.OPA{}).resolveS3Client(t.Context(), &authconfig.OPAS3{
				Endpoint:          srv.URL,
				AllowInsecureHTTP: true,
				Region:            "us-east-1",
				AccessKey:         "test-access-key",
				SecretKey:         "test-secret-key",
				PathStyle:         true,
				SSRF:              tc.cfg,
			})
			require.NoError(t, err)
			_, err = client.ListObjectsV2(t.Context(), &awss3.ListObjectsV2Input{Bucket: aws.String("policies")},
				func(o *awss3.Options) { o.Retryer = aws.NopRetryer{} })
			require.ErrorIs(t, err, tc.wantErr)
		})
	}
}

func TestResolveS3Client_SSRFWithInstanceRole(t *testing.T) {
	for _, key := range []string{
		"AWS_ACCESS_KEY_ID", "AWS_ACCESS_KEY", "AWS_SECRET_ACCESS_KEY", "AWS_SECRET_KEY",
		"AWS_PROFILE", "AWS_DEFAULT_PROFILE", "AWS_WEB_IDENTITY_TOKEN_FILE",
		"AWS_CONTAINER_CREDENTIALS_RELATIVE_URI", "AWS_CONTAINER_CREDENTIALS_FULL_URI",
	} {
		t.Setenv(key, "")
	}
	t.Setenv("AWS_CONFIG_FILE", t.TempDir()+"/config")
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", t.TempDir()+"/credentials")
	t.Setenv("AWS_EC2_METADATA_DISABLED", "false")

	mux := http.NewServeMux()
	mux.HandleFunc("PUT /latest/api/token", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-Aws-Ec2-Metadata-Token-Ttl-Seconds", "60")
		_, _ = io.WriteString(w, "token")
	})
	mux.HandleFunc("GET /latest/meta-data/iam/security-credentials/{$}", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "test-role")
	})
	mux.HandleFunc("GET /latest/meta-data/iam/security-credentials/test-role", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"Code":"Success","AccessKeyId":"test-key","SecretAccessKey":"secret","Token":"token","Expiration":"2099-01-01T00:00:00Z"}`)
	})
	metadata := httptest.NewServer(mux)
	t.Cleanup(metadata.Close)
	t.Setenv("AWS_EC2_METADATA_SERVICE_ENDPOINT", metadata.URL)

	client, err := New(&authconfig.OPA{}).resolveS3Client(t.Context(), &authconfig.OPAS3{
		Region: "us-east-1",
		SSRF:   &httpconfig.ClientSSRF{},
	})
	require.NoError(t, err)
	creds, err := client.(*awss3.Client).Options().Credentials.Retrieve(t.Context())
	require.NoError(t, err)
	require.Equal(t, "test-key", creds.AccessKeyID)
}
