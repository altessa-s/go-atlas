// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-nomos/loader"
	"github.com/altessa-s/go-nomos/loader/backend/yaml3"

	"github.com/altessa-s/go-atlas/factory/ext/kryptos/gcp"

	secretsconfig "github.com/altessa-s/go-atlas/config/secrets"
	secretsfactory "github.com/altessa-s/go-atlas/factory/ext/kryptos"
)

func TestNewProvider(t *testing.T) {
	t.Parallel()

	_, err := factory.NewProvider[any](t.Context(), nil)
	require.ErrorIs(t, err, secretsfactory.ErrProviderConfigMissing)

	_, err = factory.NewProvider[any](t.Context(), &secretsconfig.GCP{
		ProjectID:          "my-project",
		ServiceAccountPath: filepath.Join(t.TempDir(), "missing.json"),
		Labels:             map[string]string{"env": "prod"},
		IgnoreInvalidKeys:  true,
	})
	require.Error(t, err)
	require.NotErrorIs(t, err, secretsfactory.ErrProviderConfigMissing)
}

// Any- and string-valued managers reject a gcp provider whose section is absent with
// ErrProviderConfigMissing instead of dereferencing it.
func TestBuilder_MissingSection(t *testing.T) {
	t.Parallel()
	cfg := &secretsconfig.Config{Provider: secretsconfig.ProviderGCP}

	_, err := secretsfactory.New[any](cfg).UseProvider(secretsconfig.ProviderGCP, factory.Builder[any]()).Build(t.Context())
	require.ErrorIs(t, err, secretsfactory.ErrProviderConfigMissing)
	_, err = secretsfactory.New[string](cfg).UseProvider(secretsconfig.ProviderGCP, factory.Builder[string]()).Build(t.Context())
	require.ErrorIs(t, err, secretsfactory.ErrProviderConfigMissing)
}

// A configuration loaded without a root Validate can select gcp without its
// section; building from it fails with an error, not a panic.
func TestBuilder_LoadedConfigWithoutSection(t *testing.T) {
	t.Parallel()

	type root struct {
		Secrets *secretsconfig.Config `yaml:"secrets"`
	}
	path := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte("secrets:\n  provider: gcp\n"), 0o600))
	loaded, err := loader.New(&yaml3.Backend{}, loader.WithPath(path), loader.WithEnvPrefix("GCPSECRETSFACTORYLOADTEST")).Load(&root{})
	require.NoError(t, err)
	cfg := loaded.(*root).Secrets //nolint:forcetypeassert // Load returns the pointer it was given
	require.Nil(t, cfg.GCP)

	_, err = secretsfactory.New[string](cfg).UseProvider(secretsconfig.ProviderGCP, factory.Builder[string]()).Build(t.Context())
	require.ErrorIs(t, err, secretsfactory.ErrProviderConfigMissing)
}
