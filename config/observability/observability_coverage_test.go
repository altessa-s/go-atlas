// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package observabilityconfig_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	observabilityconfig "github.com/altessa-s/go-atlas/config/observability"
)

// --- Config -----------------------------------------------------------

func TestDefault(t *testing.T) {
	t.Parallel()

	got := observabilityconfig.Default()
	require.NotNil(t, got.Metrics)
	require.NotNil(t, got.Tracing)
	require.Equal(t, observabilityconfig.DefaultMetrics(), *got.Metrics)
	require.Equal(t, observabilityconfig.DefaultTracing(), *got.Tracing)
	require.NoError(t, got.Validate())
}

func TestConfig_Validate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		cfg     observabilityconfig.Config
		wantErr bool
	}{
		{name: "zero value (both nil) is valid", cfg: observabilityconfig.Config{}},
		{name: "defaults are valid", cfg: observabilityconfig.Default()},
		{
			name: "invalid nested Metrics propagates",
			cfg: observabilityconfig.Config{
				Metrics: &observabilityconfig.Metrics{Enabled: true, Type: "bogus"},
			},
			wantErr: true,
		},
		{
			name: "invalid nested Tracing propagates",
			cfg: observabilityconfig.Config{
				Tracing: &observabilityconfig.Tracing{Enabled: true, Type: observabilityconfig.TracingTypeOTLP},
			},
			wantErr: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := tc.cfg.Validate()
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
		})
	}
}

// --- Metrics ------------------------------------------------------------

func TestDefaultMetrics(t *testing.T) {
	t.Parallel()

	got := observabilityconfig.DefaultMetrics()
	require.False(t, got.Enabled)
	require.Equal(t, observabilityconfig.MetricsTypePrometheus, got.Type)
	require.NoError(t, got.Validate())
}

func TestMetrics_Validate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		cfg     observabilityconfig.Metrics
		wantErr bool
	}{
		{
			name: "disabled skips validation entirely, even with a garbage type",
			cfg:  observabilityconfig.Metrics{Enabled: false, Type: "not-a-real-type"},
		},
		{name: "enabled with prometheus passes", cfg: observabilityconfig.Metrics{Enabled: true, Type: observabilityconfig.MetricsTypePrometheus}},
		{name: "enabled with noop passes", cfg: observabilityconfig.Metrics{Enabled: true, Type: observabilityconfig.MetricsTypeNoop}},
		{name: "enabled with unknown type fails", cfg: observabilityconfig.Metrics{Enabled: true, Type: "bogus"}, wantErr: true},
		{name: "enabled with empty type fails", cfg: observabilityconfig.Metrics{Enabled: true}, wantErr: true},
		{
			name: "enabled with adapters set passes",
			cfg: observabilityconfig.Metrics{
				Enabled: true, Type: observabilityconfig.MetricsTypePrometheus,
				Adapters: &observabilityconfig.MetricsAdapters{Prometheus: &observabilityconfig.MetricsPrometheus{CustomRegistry: true}},
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := tc.cfg.Validate()
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
		})
	}
}

// --- Health ---------------------------------------------------------------

func TestDefaultHealth(t *testing.T) {
	t.Parallel()

	got := observabilityconfig.DefaultHealth()
	require.Equal(t, 5*time.Second, got.HealthCheckInterval)
	require.Equal(t, 32, got.NumShards)
	require.NoError(t, got.Validate())
}

func TestHealth_Validate(t *testing.T) {
	t.Parallel()

	valid := observabilityconfig.DefaultHealth()

	tests := []struct {
		name    string
		mutate  func(h *observabilityconfig.Health)
		wantErr bool
	}{
		{name: "defaults pass", mutate: func(*observabilityconfig.Health) {}},
		{name: "zero HealthCheckInterval fails", mutate: func(h *observabilityconfig.Health) { h.HealthCheckInterval = 0 }, wantErr: true},
		{name: "negative WatcherChannelBuffer fails", mutate: func(h *observabilityconfig.Health) { h.WatcherChannelBuffer = -1 }, wantErr: true},
		{name: "negative MaxWatchersPerService fails", mutate: func(h *observabilityconfig.Health) { h.MaxWatchersPerService = -1 }, wantErr: true},
		{name: "zero MaxWatchersPerService passes (0 == unlimited)", mutate: func(h *observabilityconfig.Health) { h.MaxWatchersPerService = 0 }},
		{name: "zero NumShards fails", mutate: func(h *observabilityconfig.Health) { h.NumShards = 0 }, wantErr: true},
		{name: "zero MaxConcurrentHealthChecks fails", mutate: func(h *observabilityconfig.Health) { h.MaxConcurrentHealthChecks = 0 }, wantErr: true},
		{name: "zero StatusCacheTTL passes (NonNegativeDuration)", mutate: func(h *observabilityconfig.Health) { h.StatusCacheTTL = 0 }},
		{name: "zero CheckTimeout fails", mutate: func(h *observabilityconfig.Health) { h.CheckTimeout = 0 }, wantErr: true},
		{name: "zero AdaptiveBufferThreshold fails", mutate: func(h *observabilityconfig.Health) { h.AdaptiveBufferThreshold = 0 }, wantErr: true},
		{name: "zero AdaptiveBufferMultiplier fails", mutate: func(h *observabilityconfig.Health) { h.AdaptiveBufferMultiplier = 0 }, wantErr: true},
		{name: "zero MaxAdaptiveBuffer fails", mutate: func(h *observabilityconfig.Health) { h.MaxAdaptiveBuffer = 0 }, wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cfg := valid
			tc.mutate(&cfg)
			err := cfg.Validate()
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
		})
	}
}

// --- Logger -----------------------------------------------------------

func TestDefaultLogger(t *testing.T) {
	t.Parallel()

	got := observabilityconfig.DefaultLogger()
	require.Equal(t, observabilityconfig.LoggerLevelError, got.Level)
	require.Equal(t, observabilityconfig.LoggerConsoleOutputStdout, got.Output)
	require.NoError(t, got.Validate())
}

func TestLogger_Validate(t *testing.T) {
	t.Parallel()

	valid := observabilityconfig.DefaultLogger()

	tests := []struct {
		name    string
		mutate  func(l *observabilityconfig.Logger)
		wantErr bool
	}{
		{name: "defaults pass", mutate: func(*observabilityconfig.Logger) {}},
		{name: "invalid level fails", mutate: func(l *observabilityconfig.Logger) { l.Level = "bogus" }, wantErr: true},
		{name: "valid level passes", mutate: func(l *observabilityconfig.Logger) { l.Level = observabilityconfig.LoggerLevelDebug }},
		{name: "invalid output fails", mutate: func(l *observabilityconfig.Logger) { l.Output = "bogus" }, wantErr: true},
		{name: "valid output passes", mutate: func(l *observabilityconfig.Logger) { l.Output = observabilityconfig.LoggerConsoleOutputStderr }},
		{
			name: "invalid nested Buffer propagates via NestedField",
			mutate: func(l *observabilityconfig.Logger) {
				l.Buffer = observabilityconfig.LoggerBuffer{BypassLevel: "bogus"}
			},
			wantErr: true,
		},
		{
			name: "valid Subsystems map passes",
			mutate: func(l *observabilityconfig.Logger) {
				l.Subsystems = map[string]observabilityconfig.LoggerLevel{"auth": observabilityconfig.LoggerLevelDebug}
			},
		},
		{
			name: "invalid MaskRules entry fails",
			mutate: func(l *observabilityconfig.Logger) {
				l.MaskRules = []observabilityconfig.LoggerMaskRule{{Field: "password"}}
			},
			wantErr: true,
		},
		{
			name: "valid MaskRules entry passes",
			mutate: func(l *observabilityconfig.Logger) {
				l.MaskRules = []observabilityconfig.LoggerMaskRule{{Field: "password", Type: "full"}}
			},
		},
		{
			name: "invalid Subsystems entry fails",
			mutate: func(l *observabilityconfig.Logger) {
				l.Subsystems = map[string]observabilityconfig.LoggerLevel{"auth": "bogus"}
			},
			wantErr: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cfg := valid
			tc.mutate(&cfg)
			err := cfg.Validate()
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestDefaultLoggerBuffer(t *testing.T) {
	t.Parallel()

	got := observabilityconfig.DefaultLoggerBuffer()
	require.Equal(t, 100, got.Size)
	require.Equal(t, observabilityconfig.LoggerLevelError, got.BypassLevel)
	require.NoError(t, got.Validate())
}

func TestLoggerBuffer_Validate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		cfg     observabilityconfig.LoggerBuffer
		wantErr bool
	}{
		{name: "zero value fails (empty BypassLevel)", cfg: observabilityconfig.LoggerBuffer{}, wantErr: true},
		{name: "defaults pass", cfg: observabilityconfig.DefaultLoggerBuffer()},
		{name: "invalid BypassLevel fails", cfg: observabilityconfig.LoggerBuffer{BypassLevel: "bogus"}, wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := tc.cfg.Validate()
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestLoggerMaskRule_Validate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		cfg     observabilityconfig.LoggerMaskRule
		wantErr bool
	}{
		{name: "zero value fails (missing type)", cfg: observabilityconfig.LoggerMaskRule{}, wantErr: true},
		{name: "missing field and pattern fails", cfg: observabilityconfig.LoggerMaskRule{Type: "redact"}, wantErr: true},
		{name: "field alone passes", cfg: observabilityconfig.LoggerMaskRule{Type: "redact", Field: "password"}},
		{name: "pattern alone passes", cfg: observabilityconfig.LoggerMaskRule{Type: "redact", Pattern: "^secret.*"}},
		{name: "both field and pattern pass", cfg: observabilityconfig.LoggerMaskRule{Type: "redact", Field: "password", Pattern: "^secret.*"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := tc.cfg.Validate()
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
		})
	}
}

// --- Tracing ------------------------------------------------------------

func TestDefaultTracing(t *testing.T) {
	t.Parallel()

	got := observabilityconfig.DefaultTracing()
	require.False(t, got.Enabled)
	require.Equal(t, observabilityconfig.TracingTypeOTLP, got.Type)
	require.NotNil(t, got.Sampler)
	require.NotNil(t, got.Adapters)
	require.NotNil(t, got.Adapters.OTLP)
	require.NoError(t, got.Validate())
}

func TestTracing_Validate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		cfg     observabilityconfig.Tracing
		wantErr bool
	}{
		{
			name: "disabled skips validation entirely, even with a garbage type and no service name",
			cfg:  observabilityconfig.Tracing{Enabled: false, Type: "not-a-real-type"},
		},
		{
			name:    "enabled otlp without ServiceName fails",
			cfg:     observabilityconfig.Tracing{Enabled: true, Type: observabilityconfig.TracingTypeOTLP},
			wantErr: true,
		},
		{
			name:    "enabled with unknown type fails",
			cfg:     observabilityconfig.Tracing{Enabled: true, Type: "bogus", ServiceName: "svc"},
			wantErr: true,
		},
		{
			name:    "enabled otlp with ServiceName but no Adapters fails (Adapters required for otlp)",
			cfg:     observabilityconfig.Tracing{Enabled: true, Type: observabilityconfig.TracingTypeOTLP, ServiceName: "svc"},
			wantErr: true,
		},
		{
			name: "enabled noop with ServiceName and no Adapters passes (Adapters not required for noop)",
			cfg:  observabilityconfig.Tracing{Enabled: true, Type: observabilityconfig.TracingTypeNoop, ServiceName: "svc"},
		},
		{
			name: "enabled otlp with ServiceName and valid Adapters passes",
			cfg: observabilityconfig.Tracing{
				Enabled: true, Type: observabilityconfig.TracingTypeOTLP, ServiceName: "svc",
				Adapters: &observabilityconfig.TracingAdapters{
					OTLP: &observabilityconfig.TracingOTLP{Endpoint: "localhost:4317", Protocol: observabilityconfig.OTLPProtocolGRPC},
				},
			},
		},
		{
			name: "enabled otlp with invalid nested Sampler propagates",
			cfg: observabilityconfig.Tracing{
				Enabled: true, Type: observabilityconfig.TracingTypeOTLP, ServiceName: "svc",
				Sampler: &observabilityconfig.TracingSampler{Type: "bogus"},
				Adapters: &observabilityconfig.TracingAdapters{
					OTLP: &observabilityconfig.TracingOTLP{Endpoint: "localhost:4317", Protocol: observabilityconfig.OTLPProtocolGRPC},
				},
			},
			wantErr: true,
		},
		{
			name: "enabled otlp with invalid nested Adapters propagates",
			cfg: observabilityconfig.Tracing{
				Enabled: true, Type: observabilityconfig.TracingTypeOTLP, ServiceName: "svc",
				Adapters: &observabilityconfig.TracingAdapters{
					OTLP: &observabilityconfig.TracingOTLP{Endpoint: "", Protocol: observabilityconfig.OTLPProtocolGRPC},
				},
			},
			wantErr: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := tc.cfg.Validate()
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestDefaultTracingAdapters(t *testing.T) {
	t.Parallel()

	got := observabilityconfig.DefaultTracingAdapters()
	require.NotNil(t, got.OTLP)
	require.Equal(t, observabilityconfig.DefaultTracingOTLP(), *got.OTLP)
	require.Nil(t, got.Console, "the console exporter is opt-in")
	require.NoError(t, got.Validate())
}

func TestTracingAdapters_Validate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		cfg     observabilityconfig.TracingAdapters
		wantErr bool
	}{
		{name: "both nil passes (both optional here)", cfg: observabilityconfig.TracingAdapters{}},
		{
			name: "valid OTLP passes",
			cfg:  observabilityconfig.TracingAdapters{OTLP: &observabilityconfig.TracingOTLP{Endpoint: "localhost:4317", Protocol: observabilityconfig.OTLPProtocolGRPC}},
		},
		{
			name:    "invalid OTLP propagates",
			cfg:     observabilityconfig.TracingAdapters{OTLP: &observabilityconfig.TracingOTLP{Endpoint: "", Protocol: observabilityconfig.OTLPProtocolGRPC}},
			wantErr: true,
		},
		{
			name: "console always passes (TracingConsole.Validate is a stub)",
			cfg:  observabilityconfig.TracingAdapters{Console: &observabilityconfig.TracingConsole{PrettyPrint: false}},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := tc.cfg.Validate()
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestDefaultTracingSampler(t *testing.T) {
	t.Parallel()

	got := observabilityconfig.DefaultTracingSampler()
	require.Equal(t, observabilityconfig.SamplerTypeParentBased, got.Type)
	require.InDelta(t, 1.0, got.Ratio, 0)
	require.NoError(t, got.Validate())
}

func TestTracingSampler_Validate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		cfg     observabilityconfig.TracingSampler
		wantErr bool
	}{
		{name: "zero value fails (empty type)", cfg: observabilityconfig.TracingSampler{}, wantErr: true},
		{name: "defaults pass", cfg: observabilityconfig.DefaultTracingSampler()},
		{name: "unknown type fails", cfg: observabilityconfig.TracingSampler{Type: "bogus", Ratio: 1}, wantErr: true},
		{name: "ratio below 0 fails", cfg: observabilityconfig.TracingSampler{Type: observabilityconfig.SamplerTypeAlwaysOn, Ratio: -0.01}, wantErr: true},
		{name: "ratio above 1 fails", cfg: observabilityconfig.TracingSampler{Type: observabilityconfig.SamplerTypeAlwaysOn, Ratio: 1.01}, wantErr: true},
		{name: "ratio at lower bound passes", cfg: observabilityconfig.TracingSampler{Type: observabilityconfig.SamplerTypeAlwaysOff, Ratio: 0}},
		{name: "ratio at upper bound passes", cfg: observabilityconfig.TracingSampler{Type: observabilityconfig.SamplerTypeTraceIDRatio, Ratio: 1}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := tc.cfg.Validate()
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestDefaultTracingConsole(t *testing.T) {
	t.Parallel()

	got := observabilityconfig.DefaultTracingConsole()
	require.True(t, got.PrettyPrint)
	require.True(t, got.Timestamps)
}

func TestDefaultTracingOTLP(t *testing.T) {
	t.Parallel()

	got := observabilityconfig.DefaultTracingOTLP()
	require.Equal(t, "localhost:4317", got.Endpoint)
	require.Equal(t, observabilityconfig.OTLPProtocolGRPC, got.Protocol)
	require.True(t, got.Compression)
	require.False(t, got.Insecure)
	require.NoError(t, got.Validate())
}

func TestTracingOTLP_Validate_MissingEndpointFails(t *testing.T) {
	t.Parallel()

	cfg := observabilityconfig.TracingOTLP{Protocol: observabilityconfig.OTLPProtocolGRPC}
	require.Error(t, cfg.Validate())
}

func TestTracingOTLP_Validate_InvalidProtocolFails(t *testing.T) {
	t.Parallel()

	cfg := observabilityconfig.TracingOTLP{Endpoint: "localhost:4317", Protocol: "bogus"}
	require.Error(t, cfg.Validate())
}

func TestTracingOTLP_Validate_HTTPWithoutProxyPasses(t *testing.T) {
	t.Parallel()

	cfg := observabilityconfig.TracingOTLP{Endpoint: "localhost:4318", Protocol: observabilityconfig.OTLPProtocolHTTP}
	require.NoError(t, cfg.Validate())
}
