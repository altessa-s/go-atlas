// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestOIDC_Validate_NoProxy(t *testing.T) {
	t.Parallel()

	cfg := OIDC{
		DiscoveryUrl: "https://example.com/.well-known/openid-configuration",
	}
	assert.NoError(t, cfg.Validate())
}

func TestOIDC_Validate_ValidProxy(t *testing.T) {
	t.Parallel()

	cfg := OIDC{
		DiscoveryUrl: "https://example.com/.well-known/openid-configuration",
		Proxy: &HTTPProxy{
			Mode: HTTPProxyModeURL,
			URL:  "http://proxy.corp:3128",
		},
	}
	assert.NoError(t, cfg.Validate())
}

func TestOIDC_Validate_InvalidProxyPropagates(t *testing.T) {
	t.Parallel()

	cfg := OIDC{
		DiscoveryUrl: "https://example.com/.well-known/openid-configuration",
		Proxy: &HTTPProxy{
			Mode: HTTPProxyModeURL,
			// URL missing — Mode=url requires URL.
		},
	}
	assert.Error(t, cfg.Validate())
}
