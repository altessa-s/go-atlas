// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTlsClient_Normalize_SkipVerify_without_env(t *testing.T) {
	t.Setenv(EnvAllowInsecureTLS, "")

	c := &TlsClient{SkipVerify: true}
	c.Normalize()

	assert.False(t, c.SkipVerify, "SkipVerify must be reset to false without "+EnvAllowInsecureTLS)
}

func TestTlsClient_Normalize_SkipVerify_with_env(t *testing.T) {
	t.Setenv(EnvAllowInsecureTLS, "true")

	c := &TlsClient{SkipVerify: true}
	c.Normalize()

	assert.True(t, c.SkipVerify, "SkipVerify must remain true when "+EnvAllowInsecureTLS+" is set")
}

func TestTlsClient_Normalize_SkipVerify_with_env_case_insensitive(t *testing.T) {
	t.Setenv(EnvAllowInsecureTLS, "TRUE")

	c := &TlsClient{SkipVerify: true}
	c.Normalize()

	assert.True(t, c.SkipVerify, "SkipVerify must remain true when "+EnvAllowInsecureTLS+" is TRUE (case-insensitive)")
}

func TestTlsClient_Normalize_SkipVerify_false_unaffected(t *testing.T) {
	t.Parallel()

	c := &TlsClient{SkipVerify: false}
	c.Normalize()

	assert.False(t, c.SkipVerify)
}
