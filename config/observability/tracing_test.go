// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package observabilityconfig

import (
	"testing"
	"time"

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

func TestTracingBatchValidation(t *testing.T) {
	for _, tc := range []struct {
		name  string
		batch TracingBatch
		valid bool
	}{
		{"defaults", TracingBatch{}, true},
		{"small queue", TracingBatch{MaxQueueSize: new(1)}, true},
		{"batch fits", TracingBatch{MaxQueueSize: new(2), MaxExportBatchSize: new(2)}, true},
		{"zero queue", TracingBatch{MaxQueueSize: new(0)}, false},
		{"negative batch", TracingBatch{MaxExportBatchSize: new(-1)}, false},
		{"batch exceeds queue", TracingBatch{MaxQueueSize: new(1), MaxExportBatchSize: new(2)}, false},
		{"batch exceeds default queue", TracingBatch{MaxExportBatchSize: new(2049)}, false},
		{"negative delay", TracingBatch{ScheduledDelay: new(-time.Second)}, false},
		{"zero timeout", TracingBatch{ExportTimeout: new(time.Duration(0))}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := Tracing{Enabled: true, Type: TracingTypeNoop, ServiceName: "test", Batch: tc.batch}
			if tc.valid {
				assert.NoError(t, cfg.Validate())
			} else {
				assert.Error(t, cfg.Validate())
			}
		})
	}
}
