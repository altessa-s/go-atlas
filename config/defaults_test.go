// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDefaultAuth(t *testing.T) {
	cfg := DefaultAuth()
	require.NotNil(t, cfg.OIDC)
	require.NotNil(t, cfg.OPA)
}

func TestDefaultGrpc(t *testing.T) {
	cfg := DefaultGrpc()
	require.NotEmpty(t, cfg.ListenAddress)
	require.NoError(t, cfg.Validate())
}

func TestDefaultHttp(t *testing.T) {
	cfg := DefaultHttp()
	require.NotEmpty(t, cfg.ListenAddress)
	require.NotZero(t, cfg.MaxRequestPayloadSize)
	require.NoError(t, cfg.Validate())
}

func TestDefaultOIDC(t *testing.T) {
	cfg := DefaultOIDC()
	require.NotZero(t, cfg.ClockSkew)
}

func TestDefaultRedis(t *testing.T) {
	cfg := DefaultRedis()
	require.NotEmpty(t, cfg.Hosts)
	require.NoError(t, cfg.Validate())
}
