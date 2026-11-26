// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package grpcconfig_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-nomos/loader"

	grpcconfig "github.com/altessa-s/go-atlas/config/grpc"
)

func TestCacheInterceptorPublicMethods(t *testing.T) {
	t.Parallel()

	type wrapper struct {
		Cache grpcconfig.CacheInterceptor `yaml:"cache"`
	}
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "grpc.yaml"), []byte(`cache:
  enabled: true
  publicMethods:
    - /api.Catalog/GetProduct
`), 0o600))
	var cfg wrapper
	_, err := loader.New(nil, loader.WithPath(dir)).Load(&cfg)
	require.NoError(t, err)
	require.Equal(t, []string{"/api.Catalog/GetProduct"}, cfg.Cache.PublicMethods)
	require.NoError(t, cfg.Cache.Validate())
	for _, invalid := range []string{"", "/api.Catalog/*", "GetProduct", " /api.Catalog/GetProduct"} {
		cfg.Cache.PublicMethods = []string{invalid}
		require.ErrorContains(t, cfg.Cache.Validate(), "PublicMethods")
	}
}
