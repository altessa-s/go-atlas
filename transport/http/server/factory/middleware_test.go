// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/config"
)

// TestEffectiveBodyLimit checks that ServerBuilder.effectiveBodyLimit
// promotes Http.MaxRequestPayloadSize as the canonical body-size budget,
// while still letting middlewares.bodyLimit override it when enabled. This
// guards against regressing the bug where MaxRequestPayloadSize was
// documented as "10 MB enforced" but never wired to the bodylimit
// middleware, so disabling middlewares.bodyLimit silently removed the cap.
func TestEffectiveBodyLimit(t *testing.T) {
	tests := []struct {
		name        string
		cfg         *config.Http
		wantInBytes int64
	}{
		{
			name:        "nil cfg",
			cfg:         nil,
			wantInBytes: 0,
		},
		{
			name: "MaxRequestPayloadSize used when bodyLimit middleware not configured",
			cfg: &config.Http{
				MaxRequestPayloadSize: 5 * 1024 * 1024,
			},
			wantInBytes: 5 * 1024 * 1024,
		},
		{
			name: "MaxRequestPayloadSize used when bodyLimit explicitly disabled",
			cfg: &config.Http{
				MaxRequestPayloadSize: 7 * 1024 * 1024,
				Middlewares: &config.MiddlewaresConfig{
					BodyLimit: &config.HttpInterBodyLimitConfig{
						BaseHttpMiddlewareConfig: config.BaseHttpMiddlewareConfig{
							EnableMixin: config.EnableMixin{Enabled: false},
						},
						MaxSize: 99,
					},
				},
			},
			wantInBytes: 7 * 1024 * 1024,
		},
		{
			name: "bodyLimit middleware enabled overrides MaxRequestPayloadSize",
			cfg: &config.Http{
				MaxRequestPayloadSize: 7 * 1024 * 1024,
				Middlewares: &config.MiddlewaresConfig{
					BodyLimit: &config.HttpInterBodyLimitConfig{
						BaseHttpMiddlewareConfig: config.BaseHttpMiddlewareConfig{
							EnableMixin: config.EnableMixin{Enabled: true},
						},
						MaxSize: 1024,
					},
				},
			},
			wantInBytes: 1024,
		},
		{
			name: "MaxRequestPayloadSize=0 disables enforcement",
			cfg: &config.Http{
				MaxRequestPayloadSize: 0,
			},
			wantInBytes: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			b := &ServerBuilder{cfg: tt.cfg}
			require.Equal(t, tt.wantInBytes, b.effectiveBodyLimit())
		})
	}
}
