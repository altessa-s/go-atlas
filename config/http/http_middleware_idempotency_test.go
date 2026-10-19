// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package httpconfig

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func enabledIdempotencyConfig() IdempotencyMiddleware {
	cfg := DefaultIdempotencyMiddleware()
	cfg.Enabled = true
	return cfg
}

func TestDefaultIdempotencyMiddleware_KeyLogMode(t *testing.T) {
	t.Parallel()

	cfg := DefaultIdempotencyMiddleware()
	require.Equal(t, IdempotencyKeyLogModeHashed, cfg.KeyLogMode,
		"the default must keep raw keys out of the log")
	enabled := enabledIdempotencyConfig()
	require.NoError(t, enabled.Validate())
}

func TestIdempotencyMiddleware_Validate_KeyLogMode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		mode    IdempotencyKeyLogMode
		wantErr bool
	}{
		{"hashed", IdempotencyKeyLogModeHashed, false},
		{"full", IdempotencyKeyLogModeFull, false},
		{"off", IdempotencyKeyLogModeOff, false},
		{"empty", IdempotencyKeyLogMode(""), true},
		{"unknown", IdempotencyKeyLogMode("verbose"), true},
		{"wrong case", IdempotencyKeyLogMode("Hashed"), true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			cfg := enabledIdempotencyConfig()
			cfg.KeyLogMode = tc.mode

			err := cfg.Validate()
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestIdempotencyMiddleware_Validate_DisabledSkipsKeyLogMode(t *testing.T) {
	t.Parallel()

	cfg := DefaultIdempotencyMiddleware()
	cfg.Enabled = false
	cfg.KeyLogMode = IdempotencyKeyLogMode("verbose")

	require.NoError(t, cfg.Validate())
}

func TestAllIdempotencyKeyLogModes(t *testing.T) {
	t.Parallel()

	require.Equal(t,
		[]IdempotencyKeyLogMode{
			IdempotencyKeyLogModeHashed,
			IdempotencyKeyLogModeFull,
			IdempotencyKeyLogModeOff,
		},
		AllIdempotencyKeyLogModes())
}

func TestIdempotencyMiddleware_Validate_KeySource(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		mutate  func(*IdempotencyMiddleware)
		wantErr bool
	}{
		"default uuidv4":           {mutate: func(*IdempotencyMiddleware) {}},
		"empty format is uuidv4":   {mutate: func(c *IdempotencyMiddleware) { c.KeyFormat = "" }},
		"opaque":                   {mutate: func(c *IdempotencyMiddleware) { c.KeyFormat = "opaque"; c.MaxKeyLength = 64 }},
		"pattern":                  {mutate: func(c *IdempotencyMiddleware) { c.KeyFormat = "pattern"; c.KeyPattern = `^req-\d+$` }},
		"pattern without regexp":   {mutate: func(c *IdempotencyMiddleware) { c.KeyFormat = "pattern" }, wantErr: true},
		"pattern does not compile": {mutate: func(c *IdempotencyMiddleware) { c.KeyFormat = "pattern"; c.KeyPattern = "(" }, wantErr: true},
		"unknown format":           {mutate: func(c *IdempotencyMiddleware) { c.KeyFormat = "base64" }, wantErr: true},
		"negative length":          {mutate: func(c *IdempotencyMiddleware) { c.MaxKeyLength = -1 }, wantErr: true},
		"query param":              {mutate: func(c *IdempotencyMiddleware) { c.KeyQueryParam = "requestId" }},
		"mandatory lists": {mutate: func(c *IdempotencyMiddleware) {
			c.MandatoryPaths = []string{"/o"}
			c.MandatoryPatterns = []string{"^/p"}
		}},
		"empty mandatory path":  {mutate: func(c *IdempotencyMiddleware) { c.MandatoryPaths = []string{""} }, wantErr: true},
		"bad mandatory pattern": {mutate: func(c *IdempotencyMiddleware) { c.MandatoryPatterns = []string{"["} }, wantErr: true},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			cfg := enabledIdempotencyConfig()
			tc.mutate(&cfg)
			if tc.wantErr {
				require.Error(t, cfg.Validate())
				return
			}
			require.NoError(t, cfg.Validate())
		})
	}
}
