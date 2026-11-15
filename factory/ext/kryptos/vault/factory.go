// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"context"
	"errors"

	"github.com/altessa-s/go-atlas-core/collections/slices"

	secretsconfig "github.com/altessa-s/go-atlas/config/secrets"
	secretsfactory "github.com/altessa-s/go-atlas/factory/ext/kryptos"
	kryptos "github.com/altessa-s/go-kryptos"
	vaultprovider "github.com/altessa-s/go-kryptos/providers/vault"
	vaultApi "github.com/hashicorp/vault/api"
)

// ErrClientRequired is returned when the Vault provider is built without a
// Vault client.
var ErrClientRequired = errors.New("vault client is required")

// Builder returns the provider builder of the vault type, ready for
// secretsfactory.ManagerBuilder.UseProvider. The provider reads the KV engine
// through client, configured by the vault section.
func Builder(client *vaultApi.Client) secretsfactory.ProviderBuilder {
	return builder{client: client}
}

// NewProvider creates the Vault KV provider of T secrets from the vault
// section (nil keeps the provider defaults).
func NewProvider[T any](client *vaultApi.Client, cfg *secretsconfig.Vault) (*vaultprovider.Storage[T], error) {
	if client == nil {
		return nil, ErrClientRequired
	}

	var opts []vaultprovider.Option[T]
	if cfg != nil {
		opts = slices.AppendIf(opts, cfg.MountPath != "", vaultprovider.WithMountPath[T](cfg.MountPath))
		opts = slices.AppendIf(opts, cfg.SecretPath != "", vaultprovider.WithSecretPath[T](cfg.SecretPath))
		opts = slices.AppendIf(opts, cfg.CAS, vaultprovider.WithCheckAndSet[T]())
	}

	return vaultprovider.New[T](client, opts...)
}

type builder struct {
	client *vaultApi.Client
}

func (b builder) Build(_ context.Context, cfg *secretsconfig.Config) (kryptos.Provider[any], error) {
	return provider[any](b.client, cfg)
}

func (b builder) BuildString(_ context.Context, cfg *secretsconfig.Config) (kryptos.Provider[string], error) {
	return provider[string](b.client, cfg)
}

// provider adapts NewProvider to the kryptos.Provider interface without
// wrapping a nil storage in a non-nil interface.
func provider[T any](client *vaultApi.Client, cfg *secretsconfig.Config) (kryptos.Provider[T], error) {
	p, err := NewProvider[T](client, cfg.Vault)
	if err != nil {
		return nil, err
	}
	return p, nil
}
