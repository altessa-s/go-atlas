// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
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
		Proxy: &Proxy{
			Mode: ProxyModeURL,
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
		Proxy: &Proxy{
			Mode: ProxyModeURL,
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
		Proxy: &Proxy{
			Mode: ProxyModeURL,
			URL:  "http://proxy.corp:3128",
		},
	}
	assert.NoError(t, cfg.Validate())
}

func TestOPAS3_Validate_InvalidProxyPropagates(t *testing.T) {
	t.Parallel()

	cfg := OPAS3{
		Bucket: "policies",
		Proxy: &Proxy{
			Mode: ProxyModeURL,
			// URL missing — Mode=url requires URL.
		},
	}
	assert.Error(t, cfg.Validate())
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
