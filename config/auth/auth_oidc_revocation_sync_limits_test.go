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

func TestOIDCRevocation_SyncLimitDefaults(t *testing.T) {
	t.Parallel()

	r := authconfig.DefaultOIDCRevocation()
	require.Equal(t, 5*time.Minute, r.SyncTimeout)
	require.EqualValues(t, 256<<20, r.MaxResponseBytes)
	require.False(t, r.AllowEmpty)
}

func TestOIDCRevocation_ValidateSyncLimits(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		mutate    func(r *authconfig.OIDCRevocation)
		wantField string
	}{
		{name: "defaults"},
		{name: "zero sync timeout disables the timeout", mutate: func(r *authconfig.OIDCRevocation) { r.SyncTimeout = 0 }},
		{
			name:      "negative sync timeout",
			mutate:    func(r *authconfig.OIDCRevocation) { r.SyncTimeout = -time.Second },
			wantField: "SyncTimeout",
		},
		{name: "one byte cap", mutate: func(r *authconfig.OIDCRevocation) { r.MaxResponseBytes = 1 }},
		{
			name:      "zero cap",
			mutate:    func(r *authconfig.OIDCRevocation) { r.MaxResponseBytes = 0 },
			wantField: "MaxResponseBytes",
		},
		{
			name:      "negative cap",
			mutate:    func(r *authconfig.OIDCRevocation) { r.MaxResponseBytes = -1 },
			wantField: "MaxResponseBytes",
		},
		{name: "allow empty", mutate: func(r *authconfig.OIDCRevocation) { r.AllowEmpty = true }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			r := validRevocation()
			if tc.mutate != nil {
				tc.mutate(r)
			}
			err := r.Validate()
			if tc.wantField == "" {
				require.NoError(t, err)
				return
			}
			require.Contains(t, fieldErrors(t, err), tc.wantField)
		})
	}
}

// The sync-limit keys take their defaults when absent and load explicit
// values, including the zero syncTimeout that disables the timeout.
func TestOIDCRevocation_SyncLimitsLoadFromYAML(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		yaml           string
		wantTimeout    time.Duration
		wantMaxBytes   int64
		wantAllowEmpty bool
	}{
		{name: "absent keys take the defaults", wantTimeout: 5 * time.Minute, wantMaxBytes: 256 << 20},
		{
			name:           "explicit values",
			yaml:           "    syncTimeout: 30s\n    maxResponseBytes: 1048576\n    allowEmpty: true\n",
			wantTimeout:    30 * time.Second,
			wantMaxBytes:   1 << 20,
			wantAllowEmpty: true,
		},
		{name: "explicit zero timeout opts out", yaml: "    syncTimeout: 0s\n", wantMaxBytes: 256 << 20},
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
			require.Equal(t, tc.wantTimeout, cfg.OIDC.Revocation.SyncTimeout)
			require.Equal(t, tc.wantMaxBytes, cfg.OIDC.Revocation.MaxResponseBytes)
			require.Equal(t, tc.wantAllowEmpty, cfg.OIDC.Revocation.AllowEmpty)
		})
	}
}

func TestOIDCRevocation_DisabledWithoutResponseCap(t *testing.T) {
	t.Parallel()
	require.NoError(t, (&authconfig.OIDCRevocation{}).Validate())
}
