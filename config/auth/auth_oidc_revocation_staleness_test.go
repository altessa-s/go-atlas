// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package authconfig_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-nomos/loader"

	authconfig "github.com/altessa-s/go-atlas/config/auth"
)

// revocation.maxStaleness defaults to three hours when the key is absent, and
// an explicit zero — the opt-out — survives loading.
func TestOIDCRevocation_MaxStalenessLoadsFromYAML(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		yaml string
		want time.Duration
	}{
		{name: "absent key takes the default", want: 3 * time.Hour},
		{name: "explicit value", yaml: "    maxStaleness: 20m\n", want: 20 * time.Minute},
		{name: "explicit zero opts out", yaml: "    maxStaleness: 0s\n"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			path := filepath.Join(t.TempDir(), "config.yaml")
			doc := "oidc:\n  discoveryURL: https://idp.example.com\n  revocation:\n    enabled: true\n" + tc.yaml
			require.NoError(t, os.WriteFile(path, []byte(doc), 0o600))

			var cfg struct {
				OIDC authconfig.OIDC `yaml:"oidc"`
			}
			_, err := loader.New(nil, loader.WithPath(path), loader.WithSkipEnv()).Load(&cfg)
			require.NoError(t, err)
			require.NotNil(t, cfg.OIDC.Revocation)
			require.Equal(t, tc.want, cfg.OIDC.Revocation.MaxStaleness)
		})
	}
}

// jwks.maxStaleness stays nil when the key is absent (the provider applies its
// 24h default), and an explicit zero — the opt-out — survives loading.
func TestOIDCJwks_MaxStalenessLoadsFromYAML(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		yaml string
		want *time.Duration
	}{
		{name: "absent key stays unset", yaml: "    httpTimeout: 5s\n"},
		{name: "explicit value", yaml: "    maxStaleness: 6h\n", want: new(6 * time.Hour)},
		{name: "explicit zero opts out", yaml: "    maxStaleness: 0s\n", want: new(time.Duration(0))},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			path := filepath.Join(t.TempDir(), "config.yaml")
			doc := "oidc:\n  discoveryURL: https://idp.example.com\n  jwks:\n" + tc.yaml
			require.NoError(t, os.WriteFile(path, []byte(doc), 0o600))

			var cfg struct {
				OIDC authconfig.OIDC `yaml:"oidc"`
			}
			_, err := loader.New(nil, loader.WithPath(path), loader.WithSkipEnv()).Load(&cfg)
			require.NoError(t, err)
			require.NotNil(t, cfg.OIDC.JWKS)
			require.Equal(t, tc.want, cfg.OIDC.JWKS.MaxStaleness)
		})
	}
}
