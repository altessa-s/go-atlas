// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package observabilityconfig

import (
	"testing"

	"github.com/stretchr/testify/assert"

	proxyconfig "github.com/altessa-s/go-atlas/config/proxy"
)

func TestTracingOTLP_Validate_NoProxy(t *testing.T) {
	t.Parallel()

	cfg := TracingOTLP{
		Endpoint: "localhost:4317",
		Protocol: OTLPProtocolGRPC,
	}
	assert.NoError(t, cfg.Validate())
}

func TestTracingOTLP_Validate_ValidProxy(t *testing.T) {
	t.Parallel()

	cfg := TracingOTLP{
		Endpoint: "localhost:4317",
		Protocol: OTLPProtocolGRPC,
		Proxy: &proxyconfig.Config{
			Mode: proxyconfig.ModeURL,
			URL:  "http://proxy.corp:3128",
		},
	}
	assert.NoError(t, cfg.Validate())
}

func TestTracingOTLP_Validate_InvalidProxyPropagates(t *testing.T) {
	t.Parallel()

	cfg := TracingOTLP{
		Endpoint: "localhost:4317",
		Protocol: OTLPProtocolGRPC,
		Proxy: &proxyconfig.Config{
			Mode: proxyconfig.ModeURL,
			// URL missing — Mode=url requires URL.
		},
	}
	assert.Error(t, cfg.Validate())
}

func TestTracingOTLP_Validate_RejectsProxyWithHTTPProtocol(t *testing.T) {
	t.Parallel()

	// Combining a Proxy block with Protocol=http silently no-ops at
	// runtime (factory routes Proxy via WithGRPCClientOptions, ignored
	// by the otlptracehttp path). Reject at config-load instead — this
	// is especially important for Mode=none, where the silent drop
	// would let env-based proxy take over despite the operator's
	// explicit "no proxy" intent.
	cases := []struct {
		name string
		mode proxyconfig.Mode
	}{
		{"mode_none", proxyconfig.ModeNone},
		{"mode_url", proxyconfig.ModeURL},
		{"mode_host", proxyconfig.ModeHost},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cfg := TracingOTLP{
				Endpoint: "localhost:4318",
				Protocol: OTLPProtocolHTTP,
				Proxy:    &proxyconfig.Config{Mode: tc.mode},
			}
			assert.Error(t, cfg.Validate())
		})
	}
}
