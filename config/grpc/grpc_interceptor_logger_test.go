// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package grpcconfig_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/config/loader"

	grpcconfig "github.com/altessa-s/go-atlas/config/grpc"
)

// TestLoggerInterceptor_DebugRedactDefault pins the production-safe default of debugRedact on both paths an operator can take:
// the default constructor and the loader's default-tag pass. An explicit false is kept.
func TestLoggerInterceptor_DebugRedactDefault(t *testing.T) {
	t.Parallel()

	require.True(t, grpcconfig.DefaultLoggerInterceptor().DebugRedact)

	type wrapper struct {
		Logger grpcconfig.LoggerInterceptor `yaml:"logger"`
	}
	for _, tc := range []struct {
		name string
		yaml string
		want bool
	}{
		{name: "omitted", yaml: "logger:\n  enabled: true\n", want: true},
		{name: "explicit false", yaml: "logger:\n  enabled: true\n  debugRedact: false\n", want: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(dir, "grpc.yaml"), []byte(tc.yaml), 0o600))
			cfg := &wrapper{}
			_, err := loader.New(nil, loader.WithPath(dir)).Load(cfg)
			require.NoError(t, err)
			require.Equal(t, tc.want, cfg.Logger.DebugRedact)
		})
	}
}
