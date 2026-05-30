// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package loader_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/altessa-s/go-atlas/config/loader"
)

func BenchmarkLoad_Defaults(b *testing.B) {
	cfg := &TestConfig{}
	l := loader.New(nil)

	b.ResetTimer()
	for b.Loop() {
		_, _ = l.Load(cfg)
	}
}

func BenchmarkLoad_Env(b *testing.B) {
	b.Setenv("APP_NAME", "bench-app")
	b.Setenv("PORT", "1234")

	cfg := &TestConfig{}
	l := loader.New(nil)

	b.ResetTimer()
	for b.Loop() {
		_, _ = l.Load(cfg)
	}
}

func BenchmarkLoad_File(b *testing.B) {
	content := `
appName: bench-app
port: 1234
database:
  host: localhost
  port: 5432
`
	tmpDir := b.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		b.Fatalf("Failed to write config file: %v", err)
	}

	l := loader.New(nil, loader.WithPath(configPath))
	cfg := &TestConfig{}

	b.ResetTimer()
	for b.Loop() {
		_, _ = l.Load(cfg)
	}
}
