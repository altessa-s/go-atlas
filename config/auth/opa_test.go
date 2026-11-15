// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package authconfig

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	proxyconfig "github.com/altessa-s/go-atlas/config/proxy"
)

func TestOPAGitLab_Validate_NoProxy(t *testing.T) {
	t.Parallel()

	cfg := OPAGitLab{
		Endpoint:  "https://gitlab.example.com",
		Token:     "tok",
		ProjectID: 42,
	}
	assert.NoError(t, cfg.Validate())
}

func TestOPAGitLab_Validate_ValidProxy(t *testing.T) {
	t.Parallel()

	cfg := OPAGitLab{
		Endpoint:  "https://gitlab.example.com",
		Token:     "tok",
		ProjectID: 42,
		Proxy: &proxyconfig.Config{
			Mode: proxyconfig.ModeURL,
			URL:  "http://proxy.corp:3128",
		},
	}
	assert.NoError(t, cfg.Validate())
}

func TestOPAGitLab_Validate_InvalidProxyPropagates(t *testing.T) {
	t.Parallel()

	cfg := OPAGitLab{
		Endpoint:  "https://gitlab.example.com",
		Token:     "tok",
		ProjectID: 42,
		Proxy: &proxyconfig.Config{
			Mode: proxyconfig.ModeURL,
			// URL missing — Mode=url requires URL.
		},
	}
	assert.Error(t, cfg.Validate())
}

func TestOPAS3_Validate_NoProxy(t *testing.T) {
	t.Parallel()

	cfg := OPAS3{Bucket: "policies"}
	assert.NoError(t, cfg.Validate())
}

func TestOPAS3_Validate_ValidProxy(t *testing.T) {
	t.Parallel()

	cfg := OPAS3{
		Bucket: "policies",
		Proxy: &proxyconfig.Config{
			Mode: proxyconfig.ModeURL,
			URL:  "http://proxy.corp:3128",
		},
	}
	assert.NoError(t, cfg.Validate())
}

func TestOPAS3_Validate_InvalidProxyPropagates(t *testing.T) {
	t.Parallel()

	cfg := OPAS3{
		Bucket: "policies",
		Proxy: &proxyconfig.Config{
			Mode: proxyconfig.ModeURL,
			// URL missing — Mode=url requires URL.
		},
	}
	assert.Error(t, cfg.Validate())
}

func TestOPAS3_Validate_EndpointScheme(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name      string
		endpoint  string
		allowHTTP bool
		wantErr   bool
	}{
		{name: "default AWS endpoint", endpoint: ""},
		{name: "https endpoint", endpoint: "https://minio.internal:9000"},
		{name: "http endpoint rejected", endpoint: "http://minio.internal:9000", wantErr: true},
		{name: "http endpoint with opt-in", endpoint: "http://minio.internal:9000", allowHTTP: true},
		{name: "schemeless endpoint rejected", endpoint: "minio.internal:9000", wantErr: true, allowHTTP: true},
		{name: "https without host rejected", endpoint: "https://", wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cfg := OPAS3{Bucket: "policies", Endpoint: tc.endpoint, AllowInsecureHTTP: tc.allowHTTP}
			if tc.wantErr {
				assert.Error(t, cfg.Validate())
				return
			}
			assert.NoError(t, cfg.Validate())
		})
	}
}

func TestOPACache_Validate_MaxSize(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		cache   OPACache
		wantErr bool
	}{
		{
			// Zero is a meaningful value ("use the default"), unlike the
			// Min-guarded fields where it is a silent misconfiguration.
			name:  "zero means default",
			cache: OPACache{Enabled: true, TTL: time.Minute, MaxSize: 0},
		},
		{
			name:  "positive cap",
			cache: OPACache{Enabled: true, TTL: time.Minute, MaxSize: 500},
		},
		{
			name:    "negative cap",
			cache:   OPACache{Enabled: true, TTL: time.Minute, MaxSize: -1},
			wantErr: true,
		},
		{
			// The cap is inert while caching is off, but a negative value is
			// still a typo worth reporting.
			name:    "negative cap while disabled",
			cache:   OPACache{Enabled: false, MaxSize: -1},
			wantErr: true,
		},
		{
			name:  "defaults validate",
			cache: DefaultOPACache(),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := tc.cache.Validate()
			if tc.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
		})
	}
}

func TestDefaultOPACache(t *testing.T) {
	t.Parallel()

	cfg := DefaultOPACache()
	assert.False(t, cfg.Enabled, "caching is opt-in")
	assert.Equal(t, DefaultOPACacheTTL, cfg.TTL)
	assert.Equal(t, DefaultOPACacheMaxSize, cfg.MaxSize)
}

// Checksums are lower-case SHA-256 hex digests keyed by a non-empty file
// name: the sources compare the digest they compute verbatim, so any other
// form could never match.
func TestOPA_Validate_Checksums(t *testing.T) {
	t.Parallel()
	const digest = "9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08"
	for _, tc := range []struct {
		name      string
		checksums map[string]string
		wantErr   bool
	}{
		{name: "none"},
		{name: "valid", checksums: map[string]string{"authz.rego": digest}},
		{name: "upper case", checksums: map[string]string{"authz.rego": strings.ToUpper(digest)}, wantErr: true},
		{name: "short", checksums: map[string]string{"authz.rego": digest[:63]}, wantErr: true},
		{name: "empty name", checksums: map[string]string{"": digest}, wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			gl := OPAGitLab{Endpoint: "https://gitlab.example.com", Token: "tok", ProjectID: 42, Checksums: tc.checksums}
			s3 := OPAS3{Bucket: "policies", Checksums: tc.checksums}
			if tc.wantErr {
				assert.Error(t, gl.Validate())
				assert.Error(t, s3.Validate())
				return
			}
			assert.NoError(t, gl.Validate())
			assert.NoError(t, s3.Validate())
		})
	}
}
