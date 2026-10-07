// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package config_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	authconfig "github.com/altessa-s/go-atlas/config/auth"
	grpcconfig "github.com/altessa-s/go-atlas/config/grpc"
	httpconfig "github.com/altessa-s/go-atlas/config/http"
	redisconfig "github.com/altessa-s/go-atlas/config/redis"
)

func TestDefaultAuth(t *testing.T) {
	cfg := authconfig.Default()
	require.NotNil(t, cfg.OIDC)
	require.NotNil(t, cfg.OPA)
}

func TestDefaultGrpc(t *testing.T) {
	cfg := grpcconfig.Default()
	require.NotEmpty(t, cfg.ListenAddress)
	require.NoError(t, cfg.Validate())
}

func TestDefaultHttp(t *testing.T) {
	cfg := httpconfig.Default()
	require.NotEmpty(t, cfg.ListenAddress)
	require.NoError(t, cfg.Validate())
}

func TestDefaultOIDC(t *testing.T) {
	cfg := authconfig.DefaultOIDC()
	require.Equal(t, 30*time.Second, cfg.ClockSkew, "must match the documented clockSkew default")
}

func TestDefaultRedis(t *testing.T) {
	cfg := redisconfig.Default()
	require.NotEmpty(t, cfg.Hosts)
	require.NoError(t, cfg.Validate())
}
