// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package loader_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

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

	_, err := l.Load(cfg)
	require.NoError(t, err)

	require.Equal(t, "my-app", cfg.AppName)
	require.Equal(t, 8080, cfg.Port)
	require.Equal(t, 5*time.Second, cfg.Timeout)
	require.Equal(t, "localhost", cfg.Database.Host)
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

	_, err := l.Load(cfg)
	require.NoError(t, err)

	require.Equal(t, "env-app", cfg.AppName)
	require.Equal(t, 9090, cfg.Port)
	require.True(t, cfg.Debug)
	require.Equal(t, 10*time.Second, cfg.Timeout)
	require.Equal(t, "db-prod", cfg.Database.Host)
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
	require.NoError(t, os.WriteFile(configPath, []byte(content), 0644))

	cfg := &TestConfig{}
	l := loader.New(nil, loader.WithPath(configPath))

	_, err := l.Load(cfg)
	require.NoError(t, err)

	require.Equal(t, "file-app", cfg.AppName)
	require.Equal(t, 7070, cfg.Port)
	require.Equal(t, "db-file", cfg.Database.Host)
	require.Len(t, cfg.Tags, 2)
	require.Equal(t, "tag1", cfg.Tags[0])
	require.Equal(t, "tag2", cfg.Tags[1])
}

func TestLoad_DurationIntegerDefault(t *testing.T) {
	cfg := &durationIntConfig{}
	l := loader.New(nil)

	_, err := l.Load(cfg)
	require.NoError(t, err)
	require.Equal(t, time.Duration(-1), cfg.Lifetime)
}

func TestLoad_DurationZeroDefault(t *testing.T) {
	cfg := &durationZeroConfig{}
	l := loader.New(nil)

	_, err := l.Load(cfg)
	require.NoError(t, err)
	require.Equal(t, time.Duration(0), cfg.Lifetime)
}

func TestLoad_DurationStringDefault(t *testing.T) {
	cfg := &durationStringConfig{}
	l := loader.New(nil)

	_, err := l.Load(cfg)
	require.NoError(t, err)
	require.Equal(t, 5*time.Second, cfg.Lifetime)
}

func TestLoad_DurationInvalidDefault(t *testing.T) {
	cfg := &durationInvalidConfig{}
	l := loader.New(nil)

	_, err := l.Load(cfg)
	require.Error(t, err)
}
