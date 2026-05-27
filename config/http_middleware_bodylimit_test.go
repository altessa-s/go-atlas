// Copyright 2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package config_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/config"
	"github.com/altessa-s/go-atlas/config/loader"
)

// TestDefaultHttpInterBodyLimitConfig_RequireContentLength pins the
// production-safe YAML default. The Go API option
// bodylimit.WithRequireContentLength stays opt-in (off by default for
// backward compatibility with existing programmatic callers), but the
// YAML-facing default is the strict choice — production deployments
// rarely need chunked uploads and leaving the bypass open is worse
// than rejecting a few legitimate streaming clients. A silent flip
// here (e.g. someone editing the default to false) would re-open the
// chunked-drip attack window on every YAML-configured service.
func TestDefaultHttpInterBodyLimitConfig_RequireContentLength(t *testing.T) {
	t.Parallel()

	cfg := config.DefaultHttpInterBodyLimitConfig()
	require.True(t, cfg.RequireContentLength,
		"the YAML-facing default constructor must default RequireContentLength to true — flipping this silently re-opens the chunked-body bypass")
}

// TestLoad_HttpInterBodyLimitConfig_DefaultTagRoundTrip is the
// loader-level twin: an empty YAML must resolve to the strict default
// after the default-tag pass. This guards against a typo in the
// `default:"true"` struct tag (which the unit test above can't catch
// because DefaultHttpInterBodyLimitConfig hardcodes the same value).
// Together the two tests pin BOTH paths an operator can take to get
// "the YAML defaults" — the constructor and the loader round-trip —
// and one cannot drift away from the other.
func TestLoad_HttpInterBodyLimitConfig_DefaultTagRoundTrip(t *testing.T) {
	t.Parallel()

	type wrapper struct {
		BodyLimit config.HttpInterBodyLimitConfig `yaml:"bodyLimit"`
	}

	cfg := &wrapper{}
	_, err := loader.New(nil).Load(cfg)
	require.NoError(t, err)

	require.True(t, cfg.BodyLimit.RequireContentLength,
		"loader must materialize RequireContentLength=true from the `default:` tag — a typo here would silently invert the production default")
}
