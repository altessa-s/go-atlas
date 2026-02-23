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

type TestConfig struct {
	AppName string        `yaml:"appName" env:"APP_NAME" default:"my-app"`
	Port    int           `yaml:"port" env:"PORT" default:"8080"`
	Debug   bool          `yaml:"debug" env:"DEBUG"`
	Timeout time.Duration `yaml:"timeout" env:"TIMEOUT" default:"5s"`

	Database struct {
		Host string `yaml:"host" env:"DB_HOST" default:"localhost"`
		Port int    `yaml:"port" env:"DB_PORT" default:"5432"`
	} `yaml:"database"`

	Tags []string `yaml:"tags" env:"TAGS"`
}

type durationIntConfig struct {
	Lifetime time.Duration `yaml:"lifetime" default:"-1"`
}

type durationZeroConfig struct {
	Lifetime time.Duration `yaml:"lifetime" default:"0"`
}

type durationStringConfig struct {
	Lifetime time.Duration `yaml:"lifetime" default:"5s"`
}

type durationInvalidConfig struct {
	Lifetime time.Duration `yaml:"lifetime" default:"abc"`
}

func TestLoad_Defaults(t *testing.T) {
	cfg := &TestConfig{}
	l := loader.New(nil) // Default backend (YAML)

	if _, err := l.Load(cfg); err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if cfg.AppName != "my-app" {
		t.Errorf("AppName default mismatch: got %q, want %q", cfg.AppName, "my-app")
	}
	if cfg.Port != 8080 {
		t.Errorf("Port default mismatch: got %d, want %d", cfg.Port, 8080)
	}
	if cfg.Timeout != 5*time.Second {
		t.Errorf("Timeout default mismatch: got %v, want %v", cfg.Timeout, 5*time.Second)
	}
	if cfg.Database.Host != "localhost" {
		t.Errorf("Database.Host default mismatch: got %q, want %q", cfg.Database.Host, "localhost")
	}
}

func TestLoad_Env(t *testing.T) {
	os.Setenv("APP_NAME", "env-app")
	os.Setenv("PORT", "9090")
	os.Setenv("DEBUG", "true")
	os.Setenv("TIMEOUT", "10s")
	os.Setenv("DB_HOST", "db-prod")
	// Array/Slice via env is complex, tested separately or trusted via specific env loader

	defer func() {
		os.Unsetenv("APP_NAME")
		os.Unsetenv("PORT")
		os.Unsetenv("DEBUG")
		os.Unsetenv("TIMEOUT")
		os.Unsetenv("DB_HOST")
	}()

	cfg := &TestConfig{}
	l := loader.New(nil)

	if _, err := l.Load(cfg); err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if cfg.AppName != "env-app" {
		t.Errorf("AppName env mismatch: got %q, want %q", cfg.AppName, "env-app")
	}
	if cfg.Port != 9090 {
		t.Errorf("Port env mismatch: got %d, want %d", cfg.Port, 9090)
	}
	if !cfg.Debug {
		t.Errorf("Debug env mismatch: got %v, want true", cfg.Debug)
	}
	if cfg.Timeout != 10*time.Second {
		t.Errorf("Timeout env mismatch: got %v, want %v", cfg.Timeout, 10*time.Second)
	}
	if cfg.Database.Host != "db-prod" {
		t.Errorf("Database.Host env mismatch: got %q, want %q", cfg.Database.Host, "db-prod")
	}
}

func TestLoad_File(t *testing.T) {
	content := `
appName: file-app
port: 7070
database:
  host: db-file
tags:
  - tag1
  - tag2
`
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	cfg := &TestConfig{}
	l := loader.New(nil, loader.WithPath(configPath))

	if _, err := l.Load(cfg); err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if cfg.AppName != "file-app" {
		t.Errorf("AppName file mismatch: got %q, want %q", cfg.AppName, "file-app")
	}
	if cfg.Port != 7070 {
		t.Errorf("Port file mismatch: got %d, want %d", cfg.Port, 7070)
	}
	if cfg.Database.Host != "db-file" {
		t.Errorf("Database.Host file mismatch: got %q, want %q", cfg.Database.Host, "db-file")
	}
	if len(cfg.Tags) != 2 || cfg.Tags[0] != "tag1" || cfg.Tags[1] != "tag2" {
		t.Errorf("Tags file mismatch: got %v", cfg.Tags)
	}
}

func TestLoad_DurationIntegerDefault(t *testing.T) {
	cfg := &durationIntConfig{}
	l := loader.New(nil)

	if _, err := l.Load(cfg); err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if cfg.Lifetime != time.Duration(-1) {
		t.Errorf("Lifetime default mismatch: got %v, want %v", cfg.Lifetime, time.Duration(-1))
	}
}

func TestLoad_DurationZeroDefault(t *testing.T) {
	cfg := &durationZeroConfig{}
	l := loader.New(nil)

	if _, err := l.Load(cfg); err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if cfg.Lifetime != 0 {
		t.Errorf("Lifetime default mismatch: got %v, want %v", cfg.Lifetime, time.Duration(0))
	}
}

func TestLoad_DurationStringDefault(t *testing.T) {
	cfg := &durationStringConfig{}
	l := loader.New(nil)

	if _, err := l.Load(cfg); err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if cfg.Lifetime != 5*time.Second {
		t.Errorf("Lifetime default mismatch: got %v, want %v", cfg.Lifetime, 5*time.Second)
	}
}

func TestLoad_DurationInvalidDefault(t *testing.T) {
	cfg := &durationInvalidConfig{}
	l := loader.New(nil)

	if _, err := l.Load(cfg); err == nil {
		t.Fatal("expected error for invalid duration default, got nil")
	}
}
