// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package loader_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

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

type benchItem struct {
	Name    string        `yaml:"name"`
	Enabled bool          `yaml:"enabled" default:"true"`
	Weight  int           `yaml:"weight" default:"10"`
	Timeout time.Duration `yaml:"timeout" default:"15s"`
}

type benchNested struct {
	Host string `yaml:"host" default:"localhost"`
	Port int    `yaml:"port" default:"5432"`
}

type benchConfig struct {
	Name    string               `yaml:"name" default:"svc"`
	Enabled bool                 `yaml:"enabled" default:"true"`
	Limit   int                  `yaml:"limit" default:"5"`
	DB      *benchNested         `yaml:"db"`
	Cache   *benchNested         `yaml:"cache"`
	Items   []benchItem          `yaml:"items"`
	ByName  map[string]benchItem `yaml:"byName"`
}

// BenchmarkLoad_FileEnvCollections loads two files with explicit zeros, map
// and slice elements and environment overrides end to end.
func BenchmarkLoad_FileEnvCollections(b *testing.B) {
	dir := b.TempDir()
	files := map[string]string{
		"a.yaml": "name: bench\nenabled: false\nlimit: 0\ndb:\n  port: 0\nitems:\n  - name: a\n  - name: b\n    weight: 0\n" +
			"byName:\n  x: {name: x, enabled: false}\n  y: {name: y}\n",
		"b.yaml": "items:\n  - name: c\n    timeout: 0s\nbyName:\n  z: {weight: 0}\n",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
			b.Fatalf("write %s: %v", name, err)
		}
	}
	b.Setenv("BENCH_LIMIT", "7")
	b.Setenv("BENCH_DB__HOST", "db")
	b.Setenv("BENCH_ITEMS__0__WEIGHT", "3")

	l := loader.New(nil, loader.WithPath(dir), loader.WithEnvPrefix("BENCH_"))

	for b.Loop() {
		if _, err := l.Load(&benchConfig{}); err != nil {
			b.Fatalf("load: %v", err)
		}
	}
}
