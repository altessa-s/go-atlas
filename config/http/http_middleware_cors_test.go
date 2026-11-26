// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package httpconfig_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-nomos/loader"

	httpconfig "github.com/altessa-s/go-atlas/config/http"
)

func TestCORSMiddlewareAllowsLiteralExactOrigins(t *testing.T) {
	t.Parallel()

	for _, origin := range []string{"*", "https://*.example.com", "https://app*.example.com"} {
		t.Run(origin, func(t *testing.T) {
			t.Parallel()
			cfg := httpconfig.DefaultCORSMiddleware()
			cfg.Enabled = true
			cfg.AllowedOrigins = []string{origin}
			require.NoError(t, cfg.Validate())
			cfg.AllowCredentials = true
			require.NoError(t, cfg.Validate())
		})
	}
}

func TestCORSMiddlewareValidatesOriginPatternsFromYAML(t *testing.T) {
	t.Parallel()

	type wrapper struct {
		CORS httpconfig.CORSMiddleware `yaml:"cors"`
	}
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "http.yaml"), []byte(`cors:
  enabled: true
  allowCredentials: true
  allowedOriginPatterns:
    - '^https://app[^.]\.example\.com$'
`), 0o600))
	cfg := wrapper{CORS: httpconfig.DefaultCORSMiddleware()}
	_, err := loader.New(nil, loader.WithPath(dir)).Load(&cfg)
	require.NoError(t, err)
	require.ErrorIs(t, cfg.CORS.Validate(), httpconfig.ErrCredentialedWildcardOrigin)
	cfg.CORS.AllowedOriginPatterns = []string{`^https://example\.(com|.*)$`}
	require.ErrorIs(t, cfg.CORS.Validate(), httpconfig.ErrCredentialedWildcardOrigin)
	cfg.CORS.AllowCredentials = false
	require.NoError(t, cfg.CORS.Validate())
	cfg.CORS.AllowCredentials = true
	cfg.CORS.AllowedOriginPatterns = []string{`^https://api\.example\.com:8[0-9]{3}$`}
	require.NoError(t, cfg.CORS.Validate())
}
