// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/config"
)

func TestNew(t *testing.T) {
	t.Parallel()

	logger := slog.Default()

	f := New(WithLogger(logger))

	assert.NotNil(t, f)
	assert.Equal(t, logger, f.Logger())
}

func TestNew_NoOptions(t *testing.T) {
	t.Parallel()

	f := New()

	assert.NotNil(t, f)
	// defaultOptions sets logger to DiscardHandler
	assert.NotNil(t, f.Logger())
}

func TestFactory_CreateClientConfigFromConfig_Nil(t *testing.T) {
	t.Parallel()

	f := New()

	cfg, err := f.CreateClientConfigFromConfig(nil)

	assert.NoError(t, err)
	assert.Nil(t, cfg)
}

func TestFactory_CreateClientConfigFromConfig_WithServerName(t *testing.T) {
	t.Parallel()

	f := New()
	cfg := &config.TlsClient{
		ServerName: "example.com",
	}

	tlsConfig, err := f.CreateClientConfigFromConfig(cfg)

	require.NoError(t, err)
	require.NotNil(t, tlsConfig)
	assert.Equal(t, "example.com", tlsConfig.ServerName)
	assert.False(t, tlsConfig.InsecureSkipVerify)
}

func TestFactory_CreateClientConfigFromConfig_SkipVerify_without_env(t *testing.T) {
	t.Setenv(config.EnvAllowInsecureTLS, "")

	f := New()
	cfg := &config.TlsClient{
		ServerName: "example.com",
		SkipVerify: true,
	}
	cfg.Normalize() // env-guard resets SkipVerify

	tlsConfig, err := f.CreateClientConfigFromConfig(cfg)

	require.NoError(t, err)
	require.NotNil(t, tlsConfig)
	assert.False(t, tlsConfig.InsecureSkipVerify, "InsecureSkipVerify must stay false without env guard")
}

func TestFactory_CreateClientConfigFromConfig_SkipVerify_with_env(t *testing.T) {
	t.Setenv(config.EnvAllowInsecureTLS, "true")

	f := New()
	cfg := &config.TlsClient{
		ServerName: "example.com",
		SkipVerify: true,
	}
	cfg.Normalize() // env-guard permits SkipVerify

	tlsConfig, err := f.CreateClientConfigFromConfig(cfg)

	require.NoError(t, err)
	require.NotNil(t, tlsConfig)
	assert.True(t, tlsConfig.InsecureSkipVerify, "InsecureSkipVerify must be true when env guard is set")
}

func TestFactory_CreateProvidersFromConfig_Nil(t *testing.T) {
	t.Parallel()

	f := New()

	providers, err := f.CreateProvidersFromConfig(nil)

	require.NoError(t, err)
	require.NotNil(t, providers)
}

func TestFactory_CreateFileProviderFromConfig_InvalidFiles(t *testing.T) {
	t.Parallel()

	f := New()
	cfg := &config.TlsProviderFile{
		Certificate: "/nonexistent/cert.pem",
		PrivateKey:  "/nonexistent/key.pem",
	}

	provider, err := f.CreateFileProviderFromConfig(cfg)

	assert.Nil(t, provider)
	assert.Error(t, err)
}

func TestFactory_CreateVaultProviderFromConfig_NoClient(t *testing.T) {
	t.Parallel()

	f := New()
	cfg := &config.TlsProviderVault{
		CommonName: "example.com",
		Role:       "web",
	}

	provider, err := f.CreateVaultProviderFromConfig(cfg)

	assert.Nil(t, provider)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "vault client is required")
}

func TestFactory_CreateLetsEncryptProviderFromConfig_InvalidConfig(t *testing.T) {
	t.Parallel()

	f := New()
	cfg := &config.TlsProviderLetsEncrypt{
		// Missing required fields
	}

	provider, err := f.CreateLetsEncryptProviderFromConfig(cfg)

	assert.Nil(t, provider)
	assert.Error(t, err)
}

func TestFactory_fileProviderOpts(t *testing.T) {
	t.Parallel()

	t.Run("with logger", func(t *testing.T) {
		t.Parallel()

		logger := slog.Default()
		f := New(WithLogger(logger))

		opts := f.fileProviderOpts()

		assert.Len(t, opts, 1)
	})

	t.Run("default factory", func(t *testing.T) {
		t.Parallel()

		f := New()

		opts := f.fileProviderOpts()

		// Default factory has logger from defaultOptions
		assert.Len(t, opts, 1)
	})
}

func TestFactory_vaultProviderOpts(t *testing.T) {
	t.Parallel()

	t.Run("with logger and cache dir", func(t *testing.T) {
		t.Parallel()

		logger := slog.Default()
		f := New(WithLogger(logger), WithCacheDir("/tmp/certs"))

		opts := f.vaultProviderOpts()

		assert.Len(t, opts, 2)
	})

	t.Run("default factory", func(t *testing.T) {
		t.Parallel()

		f := New()

		opts := f.vaultProviderOpts()

		// Default factory has logger from defaultOptions
		assert.Len(t, opts, 1)
	})
}

func TestFactory_letsEncryptProviderOpts(t *testing.T) {
	t.Parallel()

	t.Run("with logger and cache dir", func(t *testing.T) {
		t.Parallel()

		logger := slog.Default()
		f := New(WithLogger(logger), WithCacheDir("/tmp/certs"))

		opts := f.letsEncryptProviderOpts()

		assert.Len(t, opts, 2)
	})

	t.Run("default factory", func(t *testing.T) {
		t.Parallel()

		f := New()

		opts := f.letsEncryptProviderOpts()

		// Default factory has logger from defaultOptions
		assert.Len(t, opts, 1)
	})
}
