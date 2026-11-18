// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package observabilityconfig

import (
	"testing"

	"github.com/stretchr/testify/assert"

	netproxy "github.com/altessa-s/go-atlas-net/proxyconfig"
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
			Mode: netproxy.ModeURL,
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
			Mode: netproxy.ModeURL,
			// URL missing — Mode=url requires URL.
		},
	}
	assert.Error(t, cfg.Validate())
}

func TestTracingOTLP_Validate_AcceptsProxyWithHTTPProtocol(t *testing.T) {
	t.Parallel()

	// The factory applies the proxy block to the HTTP exporter as well,
	// so both protocols share one egress policy.
	cases := []struct {
		name  string
		proxy proxyconfig.Config
	}{
		{"mode_none", proxyconfig.Config{Mode: netproxy.ModeNone}},
		{"mode_url", proxyconfig.Config{Mode: netproxy.ModeURL, URL: "http://proxy.corp:3128"}},
		{"mode_host", proxyconfig.Config{Mode: netproxy.ModeHost, Host: "proxy.corp", Port: 3128}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cfg := TracingOTLP{
				Endpoint: "localhost:4318",
				Protocol: OTLPProtocolHTTP,
				Proxy:    &tc.proxy,
			}
			assert.NoError(t, cfg.Validate())
		})
	}
}
