// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package yaml3_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/altessa-s/go-atlas/config/loader/backend"
	"github.com/altessa-s/go-atlas/config/loader/backend/yaml3"
)

type StrictTLS struct {
	CertFile string `yaml:"certFile"`
}

type strictInterceptor struct {
	Enabled bool `yaml:"enabled"`
}

// strictOpaque decodes itself and accepts any keys.
type strictOpaque struct {
	keys int
}

func (o *strictOpaque) UnmarshalYAML(node *yaml.Node) error {
	o.keys = len(node.Content) / 2
	return nil
}

type strictConfig struct {
	*StrictTLS `yaml:",inline"`

	Cache   strictInterceptor            `yaml:"cache"`
	List    []strictInterceptor          `yaml:"list"`
	ByName  map[string]strictInterceptor `yaml:"byName"`
	Meta    map[string]any               `yaml:"meta"`
	Any     any                          `yaml:"any"`
	Opaque  strictOpaque                 `yaml:"opaque"`
	Timeout int                          `yaml:"timeout"`
}

func TestBackend_DecodeStrict(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		input       string
		wantUnknown []string // substrings of the error; nil means success
		wantErr     bool     // an error that is not about unknown fields
	}{
		{
			name: "known keys",
			input: "certFile: a.pem\ncache:\n  enabled: true\nlist:\n  - enabled: true\nbyName:\n  x:\n    enabled: true\n" +
				"meta:\n  free:\n    form: 1\nany:\n  free: 1\nopaque:\n  anything: 1\n",
		},
		{
			name:        "misspelled nested key",
			input:       "cache:\n  enable: true\n",
			wantUnknown: []string{"field enable not found"},
		},
		{
			name:        "every unknown key is reported",
			input:       "cache:\n  enable: true\nlist:\n  - enabld: true\nbyName:\n  x:\n    on: true\ntimeuot: 1\n",
			wantUnknown: []string{"enable", "enabld", "on", "timeuot"},
		},
		{
			name:        "key case must match",
			input:       "Cache:\n  enabled: true\n",
			wantUnknown: []string{"field Cache not found"},
		},
		{
			name:        "unknown key next to a type mismatch",
			input:       "timeout: soon\ncache:\n  enable: true\n",
			wantUnknown: []string{"enable", "soon"},
		},
		{
			name:        "anchor holder is unknown",
			input:       "base: &b\n  enabled: true\ncache: *b\n",
			wantUnknown: []string{"field base not found"},
		},
		{
			name:    "type mismatch alone",
			input:   "timeout: soon\n",
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var cfg strictConfig
			err := (&yaml3.Backend{}).DecodeStrict(strings.NewReader(tc.input), &cfg)
			switch {
			case tc.wantUnknown != nil:
				require.ErrorIs(t, err, backend.ErrUnknownField)
				_, ok := errors.AsType[*yaml.TypeError](err)
				require.True(t, ok, "the yaml.v3 error stays inspectable")
				for _, want := range tc.wantUnknown {
					require.ErrorContains(t, err, want)
				}
			case tc.wantErr:
				require.Error(t, err)
				require.NotErrorIs(t, err, backend.ErrUnknownField)
			default:
				require.NoError(t, err)
				require.Equal(t, "a.pem", cfg.CertFile)
				require.True(t, cfg.Cache.Enabled)
				require.Equal(t, 1, cfg.Opaque.keys)
			}
		})
	}
}

// Decode keeps ignoring unknown keys.
func TestBackend_DecodeIgnoresUnknownFields(t *testing.T) {
	t.Parallel()

	var cfg strictConfig
	require.NoError(t, (&yaml3.Backend{}).Decode(strings.NewReader("cache:\n  enable: true\n"), &cfg))
	require.False(t, cfg.Cache.Enabled)
}
