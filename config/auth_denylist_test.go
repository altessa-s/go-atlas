// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func validDenylistFilter() ProbabilisticFilterConfig {
	return ProbabilisticFilterConfig{
		Type: ProbabilisticFilterTypeBloom,
		Bloom: &ProbabilisticFilterBloomConfig{
			ExpectedItems: 1000000,
		},
	}
}

func TestDenylist_Validate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		cfg     Denylist
		wantErr bool
	}{
		{
			name:    "disabled skips validation",
			cfg:     Denylist{Enabled: false},
			wantErr: false,
		},
		{
			name: "disabled ignores invalid sub-fields",
			cfg: Denylist{
				Enabled:         false,
				KeyPrefix:       "",
				RebuildInterval: -1 * time.Second,
			},
			wantErr: false,
		},
		{
			name: "valid enabled config",
			cfg: Denylist{
				Enabled:         true,
				KeyPrefix:       "denylist:revoked:",
				RebuildInterval: 5 * time.Minute,
				Filter:          validDenylistFilter(),
			},
			wantErr: false,
		},
		{
			name: "enabled with zero rebuild interval",
			cfg: Denylist{
				Enabled:   true,
				KeyPrefix: "denylist:revoked:",
				Filter:    validDenylistFilter(),
			},
			wantErr: false,
		},
		{
			name: "enabled with empty key prefix fails",
			cfg: Denylist{
				Enabled:   true,
				KeyPrefix: "",
				Filter:    validDenylistFilter(),
			},
			wantErr: true,
		},
		{
			name: "enabled with negative rebuild interval fails",
			cfg: Denylist{
				Enabled:         true,
				KeyPrefix:       "denylist:revoked:",
				RebuildInterval: -1 * time.Second,
				Filter:          validDenylistFilter(),
			},
			wantErr: true,
		},
		{
			name: "enabled with invalid filter propagates",
			cfg: Denylist{
				Enabled:   true,
				KeyPrefix: "denylist:revoked:",
				// Bloom type without a Bloom config is invalid.
				Filter: ProbabilisticFilterConfig{Type: ProbabilisticFilterTypeBloom},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := tt.cfg.Validate()
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestDefaultDenylist(t *testing.T) {
	t.Parallel()

	cfg := DefaultDenylist()
	require.False(t, cfg.Enabled)
	require.Equal(t, DefaultDenylistKeyPrefix, cfg.KeyPrefix)
	require.Equal(t, ProbabilisticFilterTypeBloom, cfg.Filter.Type)
	// Shape-only default validates while disabled.
	require.NoError(t, cfg.Validate())
}

func TestDenylist_IsEnabled(t *testing.T) {
	t.Parallel()

	require.False(t, (*Denylist)(nil).IsEnabled())
	require.False(t, (&Denylist{Enabled: false}).IsEnabled())
	require.True(t, (&Denylist{Enabled: true}).IsEnabled())
}

func TestDefaultAuth_WiresDenylist(t *testing.T) {
	t.Parallel()

	auth := DefaultAuth()
	require.Equal(t, DefaultDenylistKeyPrefix, auth.Denylist.KeyPrefix)
	require.False(t, auth.Denylist.Enabled)
}
