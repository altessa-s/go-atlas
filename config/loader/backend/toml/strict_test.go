// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package toml_test

import (
	"strings"
	"testing"

	burnt "github.com/BurntSushi/toml"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/config/loader/backend"
	"github.com/altessa-s/go-atlas/config/loader/backend/toml"
)

type StrictBase struct {
	Enabled bool `toml:"enabled"`
}

type strictInterceptor struct {
	StrictBase

	Limit int `toml:"limit"`
}

// strictOpaque decodes itself and accepts any keys.
type strictOpaque struct {
	keys int
}

func (o *strictOpaque) UnmarshalTOML(v any) error {
	m, _ := v.(map[string]any)
	o.keys = len(m)
	return nil
}

type strictConfig struct {
	Cache     strictInterceptor            `toml:"cache"`
	Ptr       *strictInterceptor           `toml:"ptr"`
	List      []strictInterceptor          `toml:"list"`
	ByName    map[string]strictInterceptor `toml:"byName"`
	Meta      map[string]any               `toml:"meta"`
	Any       any                          `toml:"any"`
	Raw       burnt.Primitive              `toml:"raw"`
	Opaque    strictOpaque                 `toml:"opaque"`
	Untagged  int
	Timeout   int `toml:"timeout"`
	Ignored   int `toml:"-"`
	unexposed int
}

func TestBackend_DecodeStrict(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		input       string
		wantUnknown []string // keys the error names; nil means success
	}{
		{
			name: "known keys",
			input: `untagged = 1
[cache]
enabled = true
limit = 2
[ptr]
ENABLED = true
[[list]]
enabled = true
[byName.x]
enabled = true
[meta.free]
form = 1
[meta.free.deeper]
x = [1, 2]
[any]
q = { r = 1 }
[raw]
k = 1
[opaque]
anything = 1
`,
		},
		{
			name:        "misspelled nested key",
			input:       "[cache]\nenable = true\n",
			wantUnknown: []string{"cache.enable"},
		},
		{
			name: "every unknown key is reported",
			input: `timeuot = 1
ignored = 1
[ptr]
enabld = true
[[list]]
enabled = true
on = true
[byName.x]
lmit = 1
`,
			wantUnknown: []string{"timeuot", "ignored", "ptr.enabld", "list.on", "byName.x.lmit"},
		},
		{
			name:        "unknown inline table key",
			input:       "cache = { enabled = true, extra = { a = 1 } }\n",
			wantUnknown: []string{"cache.extra"},
		},
		{
			name:        "unknown keys with characters Go and TOML escape differently",
			input:       "\"enable\\u0007\" = true\n\"limit\\u000B\" = 1\n\"q\\\"\\\\\" = 1\n",
			wantUnknown: []string{"enable", "limit", "q"},
		},
		{
			name:        "unknown top-level table",
			input:       "[x-defaults]\nenabled = true\n",
			wantUnknown: []string{"x-defaults"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var cfg strictConfig
			err := (&toml.Backend{}).DecodeStrict(strings.NewReader(tc.input), &cfg)
			if tc.wantUnknown == nil {
				require.NoError(t, err)
				require.True(t, cfg.Cache.Enabled)
				require.True(t, cfg.Ptr.Enabled, "keys match fields case-insensitively")
				require.Equal(t, 1, cfg.Untagged)
				require.Equal(t, 1, cfg.Opaque.keys)
				return
			}
			require.ErrorIs(t, err, backend.ErrUnknownField)
			for _, want := range tc.wantUnknown {
				require.ErrorContains(t, err, want)
			}
		})
	}
}

// An unknown table is reported once, without the keys beneath it.
func TestBackend_DecodeStrictUnknownTableChildren(t *testing.T) {
	t.Parallel()

	var cfg strictConfig
	input := "[x]\nenable = true\n[x.y]\nz = 1\n[cache]\nextra = { a = { b = 1 } }\n"
	err := (&toml.Backend{}).DecodeStrict(strings.NewReader(input), &cfg)
	require.ErrorIs(t, err, backend.ErrUnknownField)
	require.EqualError(t, err, "unknown field: x, cache.extra in type toml_test.strictConfig")
}

// Decode keeps ignoring unknown keys.
func TestBackend_DecodeIgnoresUnknownFields(t *testing.T) {
	t.Parallel()

	var cfg strictConfig
	require.NoError(t, (&toml.Backend{}).Decode(strings.NewReader("[cache]\nenable = true\n"), &cfg))
	require.False(t, cfg.Cache.Enabled)
}

type DominanceSettings struct {
	Known    int `toml:"known"`
	Settings struct {
		Strict int `toml:"strict"`
	} `toml:"settings"`
}

type DominanceA struct {
	Enabled bool `toml:"enabled"`
}

type DominanceB struct {
	Enabled bool `toml:"enabled"`
}

type DominanceTagged struct {
	Limit int
}

type dominanceConfig struct {
	DominanceSettings // flattened, declared first

	// Settings is a direct field and dominates the embedded struct's
	// promoted "settings"; it accepts free-form content.
	Settings map[string]any `toml:"settings"`
}

type dominanceAmbiguousConfig struct {
	DominanceA
	DominanceB
}

type dominanceTaggedConfig struct {
	DominanceTagged

	// Override carries the tag "limit" and dominates the untagged
	// promoted Limit.
	Override map[string]any `toml:"limit"`
}

// Field selection follows the decoder's dominance rules, not field order.
func TestBackend_DecodeStrictFieldDominance(t *testing.T) {
	t.Parallel()

	b := &toml.Backend{}

	t.Run("direct field over embedded", func(t *testing.T) {
		t.Parallel()
		var cfg dominanceConfig
		require.NoError(t, b.DecodeStrict(strings.NewReader("known = 1\n[settings]\nfree = 1\n"), &cfg))
		require.Equal(t, 1, cfg.Known)
	})
	t.Run("tagged field over untagged promoted", func(t *testing.T) {
		t.Parallel()
		var cfg dominanceTaggedConfig
		require.NoError(t, b.DecodeStrict(strings.NewReader("[limit]\nfree = 1\n"), &cfg))
	})
	t.Run("ambiguous embedded fields bind to nothing", func(t *testing.T) {
		t.Parallel()
		var cfg dominanceAmbiguousConfig
		err := b.DecodeStrict(strings.NewReader("enabled = true\n"), &cfg)
		require.ErrorIs(t, err, backend.ErrUnknownField)
		require.ErrorContains(t, err, "enabled")
	})
}
