// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"context"
	"fmt"

	"github.com/altessa-s/go-atlas-core/collections/slices"
	"github.com/altessa-s/go-atlas-core/io/files"

	coreerrs "github.com/altessa-s/go-atlas-core/errors"
	secretsconfig "github.com/altessa-s/go-atlas/config/secrets"
	secretsfactory "github.com/altessa-s/go-atlas/factory/ext/kryptos"
	kryptos "github.com/altessa-s/go-kryptos"
	gcpprovider "github.com/altessa-s/go-kryptos/providers/gcp"
)

// Builder returns the provider builder of the gcp type, ready for
// secretsfactory.ManagerBuilder.UseProvider. The provider is configured by
// the gcp section.
func Builder() secretsfactory.ProviderBuilder {
	return builder{}
}

// NewProvider creates the GCP Secret Manager provider of T secrets from the
// gcp section. A nil section fails with secretsfactory.ErrProviderConfigMissing.
func NewProvider[T any](ctx context.Context, cfg *secretsconfig.GCP) (*gcpprovider.Storage[T], error) {
	if cfg == nil {
		return nil, fmt.Errorf("%w: gcp section is required for provider gcp", secretsfactory.ErrProviderConfigMissing)
	}

	credentials, err := files.ReadRegular(cfg.ServiceAccountPath)
	if err != nil {
		return nil, coreerrs.Wrap(err, "failed to read GCP service account credentials")
	}

	opts := []gcpprovider.Option[T]{gcpprovider.WithClearCredentials[T]()}
	opts = slices.AppendIf(opts, len(cfg.Labels) > 0, gcpprovider.WithLabels[T](cfg.Labels))
	opts = slices.AppendIf(opts, cfg.IgnoreInvalidKeys, gcpprovider.WithIgnoreInvalidKeys[T]())

	return gcpprovider.New[T](ctx, cfg.ProjectID, credentials, opts...)
}

type builder struct{}

func (builder) Build(ctx context.Context, cfg *secretsconfig.Config) (kryptos.Provider[any], error) {
	return provider[any](ctx, cfg)
}

func (builder) BuildString(ctx context.Context, cfg *secretsconfig.Config) (kryptos.Provider[string], error) {
	return provider[string](ctx, cfg)
}

// provider adapts NewProvider to the kryptos.Provider interface without
// wrapping a nil storage in a non-nil interface.
func provider[T any](ctx context.Context, cfg *secretsconfig.Config) (kryptos.Provider[T], error) {
	p, err := NewProvider[T](ctx, cfg.GCP)
	if err != nil {
		return nil, err
	}
	return p, nil
}
