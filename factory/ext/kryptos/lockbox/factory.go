// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/altessa-s/go-atlas-core/collections/slices"
	"github.com/altessa-s/go-atlas-core/io/files"

	coreerrs "github.com/altessa-s/go-atlas-core/errors"
	secretsconfig "github.com/altessa-s/go-atlas/config/secrets"
	secretsfactory "github.com/altessa-s/go-atlas/factory/ext/kryptos"
	kryptos "github.com/altessa-s/go-kryptos"
	lockboxprovider "github.com/altessa-s/go-kryptos/providers/lockbox"
)

// DefaultIAMTimeout bounds one IAM token exchange.
const DefaultIAMTimeout = 10 * time.Second

// Builder returns the provider builder of the lockbox type, ready for
// secretsfactory.ManagerBuilder.UseProvider. The provider is configured by
// the lockbox section.
func Builder[T any]() secretsfactory.ProviderBuilder[T] {
	return provider[T]
}

// Provider is a Lockbox provider that owns its service-account token source:
// Close closes both.
type Provider[T any] struct {
	*lockboxprovider.Storage[T]
	tokens *lockboxprovider.ServiceAccountTokens
}

// Close closes the storage, then the token source it used.
func (p *Provider[T]) Close(ctx context.Context) error {
	return errors.Join(p.Storage.Close(ctx), p.tokens.Close(ctx))
}

// NewProvider creates the Yandex Cloud Lockbox provider of T secrets from the
// lockbox section, reading the private key from its file and exchanging
// service-account JWTs for IAM tokens. A nil section fails with
// secretsfactory.ErrProviderConfigMissing.
func NewProvider[T any](ctx context.Context, cfg *secretsconfig.Lockbox) (*Provider[T], error) {
	if cfg == nil {
		return nil, fmt.Errorf("%w: lockbox section is required for provider lockbox", secretsfactory.ErrProviderConfigMissing)
	}

	privKey, err := files.ReadRegular(cfg.PrivateKeyPath)
	if err != nil {
		return nil, coreerrs.Wrap(err, "failed to read Lockbox private key")
	}
	defer clear(privKey)

	tokens, err := lockboxprovider.NewServiceAccountTokens(&http.Client{Timeout: DefaultIAMTimeout},
		cfg.KeyID, cfg.ServiceKeyID, privKey)
	if err != nil {
		return nil, coreerrs.Wrap(err, "create Lockbox token source")
	}

	var opts []lockboxprovider.Option[T]
	opts = slices.AppendIf(opts, len(cfg.Labels) > 0, lockboxprovider.WithLabels[T](cfg.Labels))
	opts = slices.AppendIf(opts, cfg.IgnoreInvalidKeys, lockboxprovider.WithIgnoreInvalidKeys[T]())

	storage, err := lockboxprovider.New[T](ctx, cfg.FolderID, tokens, opts...)
	if err != nil {
		return nil, errors.Join(err, tokens.Close(context.WithoutCancel(ctx)))
	}
	return &Provider[T]{Storage: storage, tokens: tokens}, nil
}

// provider adapts NewProvider to the kryptos.Provider interface without
// wrapping a nil provider in a non-nil interface.
func provider[T any](ctx context.Context, cfg *secretsconfig.Config) (kryptos.Provider[T], error) {
	p, err := NewProvider[T](ctx, cfg.Lockbox)
	if err != nil {
		return nil, err
	}
	return p, nil
}
