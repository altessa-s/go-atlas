// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package loader_test

import (
	"io"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/altessa-s/go-atlas/config/loader"
	"github.com/altessa-s/go-atlas/config/loader/backend"
	"github.com/altessa-s/go-atlas/config/loader/backend/toml"
)

type unknownInterceptor struct {
	Enabled bool `yaml:"enabled" toml:"enabled"`
}

type unknownFieldsConfig struct {
	Interceptors struct {
		Cache unknownInterceptor `yaml:"cache" toml:"cache"`
	} `yaml:"interceptors" toml:"interceptors"`
}

// A misspelled key ("enable" for "enabled") used to be dropped silently and
// left the interceptor disabled; the loader now rejects it.
func TestLoad_UnknownFieldsRejected(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		backend backend.Backend
		files   map[string]string
		file    string
	}{
		{
			name:  "yaml",
			files: map[string]string{"a.yaml": "interceptors:\n  cache:\n    enabled: true\n", "b.yaml": "interceptors:\n  cache:\n    enable: true\n"},
			file:  "b.yaml",
		},
		{
			name:    "toml",
			backend: &toml.Backend{},
			files:   map[string]string{"a.toml": "[interceptors.cache]\nenabled = true\n", "b.toml": "[interceptors.cache]\nenable = true\n"},
			file:    "b.toml",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := loadFiles(t, tc.backend, &unknownFieldsConfig{}, tc.files)
			require.ErrorIs(t, err, loader.ErrDecode)
			require.ErrorIs(t, err, loader.ErrUnknownField)
			require.ErrorContains(t, err, tc.file)
			require.ErrorContains(t, err, "enable")

			cfg := &unknownFieldsConfig{}
			require.NoError(t, loadFiles(t, tc.backend, cfg, tc.files, loader.WithAllowUnknownFields()))
			require.True(t, cfg.Interceptors.Cache.Enabled)
		})
	}
}

// WithStrict does not change how unknown keys are treated.
func TestLoad_AllowUnknownFieldsWithStrict(t *testing.T) {
	t.Parallel()

	files := map[string]string{"a.yaml": "interceptors:\n  cache:\n    enable: true\n"}
	require.ErrorIs(t, loadFiles(t, nil, &unknownFieldsConfig{}, files, loader.WithStrict()), loader.ErrUnknownField)
	require.NoError(t, loadFiles(t, nil, &unknownFieldsConfig{}, files, loader.WithStrict(), loader.WithAllowUnknownFields()))
}

// plainYAML is a backend without backend.StrictDecoder.
type plainYAML struct{}

func (plainYAML) Decode(r io.Reader, in any) error { return yaml.NewDecoder(r).Decode(in) }
func (plainYAML) FileExtensions() []string         { return []string{"yaml"} }
func (plainYAML) StructTagName() string            { return "yaml" }

// A backend that cannot detect unknown keys keeps decoding without the check.
func TestLoad_UnknownFieldsBackendWithoutStrictDecoder(t *testing.T) {
	t.Parallel()

	cfg := &unknownFieldsConfig{}
	files := map[string]string{"a.yaml": "interceptors:\n  cache:\n    enable: true\n"}
	require.NoError(t, loadFiles(t, plainYAML{}, cfg, files))
	require.False(t, cfg.Interceptors.Cache.Enabled)
}
