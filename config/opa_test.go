// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package config

import (
	"testing"

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
