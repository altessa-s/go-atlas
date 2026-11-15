// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package secretsconfig_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	secretsconfig "github.com/altessa-s/go-atlas/config/secrets"
	validation "github.com/go-ozzo/ozzo-validation/v4"
)

func secretsFieldErrors(t *testing.T, err error) validation.Errors {
	t.Helper()
	verrs, ok := errors.AsType[validation.Errors](err)
	require.True(t, ok, "want validation.Errors, got %v", err)
	return verrs
}

func TestDefaultVault(t *testing.T) {
	t.Parallel()

	v := secretsconfig.DefaultVault()
	require.Equal(t, "kv", v.MountPath)
	require.NoError(t, v.Validate())

	verrs := secretsFieldErrors(t, (&secretsconfig.Vault{}).Validate())
	require.Contains(t, verrs, "MountPath")
	require.Contains(t, verrs, "SecretPath")
}

func TestConfig_ProviderSections(t *testing.T) {
	t.Parallel()

	tests := []struct {
		provider secretsconfig.Provider
		field    string
	}{
		{secretsconfig.ProviderVault, "Vault"},
		{secretsconfig.ProviderGCP, "GCP"},
		{secretsconfig.ProviderLockbox, "Lockbox"},
	}
	for _, tc := range tests {
		t.Run(string(tc.provider), func(t *testing.T) {
			t.Parallel()
			cfg := secretsconfig.Default()
			cfg.Provider = tc.provider
			cfg.Vault = nil
			require.Contains(t, secretsFieldErrors(t, cfg.Validate()), tc.field)
		})
	}

	t.Run("valid provider sections", func(t *testing.T) {
		t.Parallel()
		vault := secretsconfig.Default()
		vault.Provider = secretsconfig.ProviderVault
		vault.Vault = new(secretsconfig.DefaultVault())
		require.NoError(t, vault.Validate())

		gcp := secretsconfig.Default()
		gcp.Provider = secretsconfig.ProviderGCP
		gcp.GCP = &secretsconfig.GCP{ProjectID: "p", ServiceAccountPath: "/sa.json"}
		require.NoError(t, gcp.Validate())

		lockbox := secretsconfig.Default()
		lockbox.Provider = secretsconfig.ProviderLockbox
		lockbox.Lockbox = &secretsconfig.Lockbox{FolderID: "f", KeyID: "k", ServiceKeyID: "s", PrivateKeyPath: "/k.pem"}
		require.NoError(t, lockbox.Validate())
	})

	t.Run("invalid provider sections", func(t *testing.T) {
		t.Parallel()
		verrs := secretsFieldErrors(t, (&secretsconfig.GCP{}).Validate())
		require.Contains(t, verrs, "ProjectID")
		require.Contains(t, verrs, "ServiceAccountPath")

		verrs = secretsFieldErrors(t, (&secretsconfig.Lockbox{}).Validate())
		for _, f := range []string{"FolderID", "KeyID", "ServiceKeyID", "PrivateKeyPath"} {
			require.Contains(t, verrs, f)
		}
	})
}

func TestConfig_Validate_Fields(t *testing.T) {
	t.Parallel()

	cfg := secretsconfig.Default()
	cfg.Provider = "aws"
	cfg.UpdateSchedule = ""
	cfg.Cache.MaxSize = -1
	verrs := secretsFieldErrors(t, cfg.Validate())
	require.Contains(t, verrs, "Provider")
	require.Contains(t, verrs, "UpdateSchedule")
	require.Contains(t, verrs, "Cache")
}
