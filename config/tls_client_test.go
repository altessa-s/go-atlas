// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package config_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/config"
)

// Serial: t.Setenv modifies a process-wide env var; t.Parallel panics when Setenv is in use.
func TestTlsClient_Normalize_SkipVerify_without_env(t *testing.T) {
	t.Setenv(config.EnvAllowInsecureTLS, "")

	c := &config.TlsClient{SkipVerify: true}
	c.Normalize()

	require.False(t, c.SkipVerify, "SkipVerify must be reset to false without "+config.EnvAllowInsecureTLS)
}

// Serial: see TestTlsClient_Normalize_SkipVerify_without_env.
func TestTlsClient_Normalize_SkipVerify_with_env(t *testing.T) {
	t.Setenv(config.EnvAllowInsecureTLS, "true")

	c := &config.TlsClient{SkipVerify: true}
	c.Normalize()

	require.True(t, c.SkipVerify, "SkipVerify must remain true when "+config.EnvAllowInsecureTLS+" is set")
}

// Serial: see TestTlsClient_Normalize_SkipVerify_without_env.
func TestTlsClient_Normalize_SkipVerify_with_env_case_insensitive(t *testing.T) {
	t.Setenv(config.EnvAllowInsecureTLS, "TRUE")

	c := &config.TlsClient{SkipVerify: true}
	c.Normalize()

	require.True(t, c.SkipVerify, "SkipVerify must remain true when "+config.EnvAllowInsecureTLS+" is TRUE (case-insensitive)")
}

func TestTlsClient_Normalize_SkipVerify_false_unaffected(t *testing.T) {
	t.Parallel()

	c := &config.TlsClient{SkipVerify: false}
	c.Normalize()

	require.False(t, c.SkipVerify)
}

func TestTlsClient_Normalize_SkipVerifyMode_defaults_to_enforce(t *testing.T) {
	t.Parallel()

	c := &config.TlsClient{}
	c.Normalize()

	require.Equal(t, config.TLSSkipVerifyModeEnforce, c.SkipVerifyMode,
		"empty SkipVerifyMode must default to enforce to match the YAML loader")
}

func TestTlsClient_Normalize_SkipVerifyMode_preserved(t *testing.T) {
	t.Parallel()

	c := &config.TlsClient{SkipVerifyMode: config.TLSSkipVerifyModeDisabled}
	c.Normalize()

	require.Equal(t, config.TLSSkipVerifyModeDisabled, c.SkipVerifyMode,
		"an explicit SkipVerifyMode must not be overwritten")
}
