// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/redis/go-redis/v9"

	"github.com/altessa-s/go-atlas/auth/oidc"
	"github.com/altessa-s/go-atlas/config"
	"github.com/altessa-s/go-atlas/core/collections/slices"

	corefactory "github.com/altessa-s/go-atlas/core/factory"
	corescheduler "github.com/altessa-s/go-atlas/core/scheduler"
	probfilterfactory "github.com/altessa-s/go-atlas/data/probfilter/factory"
)

// ProviderBuilder assembles an [oidc.Provider] from configuration and
// injected dependencies using a fluent API with deferred error accumulation.
type ProviderBuilder struct {
	corefactory.Base
	cfg  *config.OIDC
	errs []error

	// Dependencies (set via Use*).
	scheduler         corescheduler.TaskRegistrar
	redisClient       redis.UniversalClient
	tokenCache        oidc.Cacher
	revocationStorage oidc.RevocationStorage
}

// New creates a new [ProviderBuilder] for the given OIDC config.
// A nil cfg is accepted; the error surfaces at [ProviderBuilder.Build] time.
func New(cfg *config.OIDC) *ProviderBuilder {
	return &ProviderBuilder{
		Base: corefactory.NewBase(slog.New(slog.DiscardHandler)),
		cfg:  cfg,
	}
}

// Build assembles and returns the OIDC provider. All errors accumulated
// during the fluent chain are returned here.
func (b *ProviderBuilder) Build(ctx context.Context) (*oidc.Provider, error) {
	if err := errors.Join(b.errs...); err != nil {
		return nil, err
	}

	if b.cfg == nil {
		return nil, fmt.Errorf("configuration is required")
	}

	opts, err := b.buildProviderOptions()
	if err != nil {
		return nil, err
	}

	return oidc.NewProvider(ctx, b.cfg.DiscoveryUrl, opts...)
}

// buildProviderOptions translates config + builder deps into oidc.Option slice.
func (b *ProviderBuilder) buildProviderOptions() ([]oidc.Option, error) {
	cfg := b.cfg

	opts := make([]oidc.Option, 0, estimatedOptionsCount)
	opts = append(opts, oidc.WithLogger(b.Logger()))

	if cfg.IsJWKSConfigured() {
		opts = append(opts, oidc.WithJwksHTTPTimeout(cfg.JWKS.HTTPTimeout))
	}

	if cfg.IsValidationConfigured() {
		valOpts, err := validationOptionsFromConfig(cfg.Validation, cfg.ClockSkew)
		if err != nil {
			return nil, b.WrapError(err, "failed to build validation options")
		}
		opts = append(opts, oidc.WithDefaultValidationOptions(valOpts...))
	}

	presetOpts, err := b.buildPresetOptions()
	if err != nil {
		return nil, err
	}
	opts = append(opts, presetOpts...)

	if cfg.IsCacheConfigured() && cfg.Cache.Enabled && b.tokenCache != nil {
		opts = append(opts, oidc.WithTokenCache(b.tokenCache),
			oidc.WithTokensCacheKeyPrefix(cfg.Cache.TokensKeyPrefix),
			oidc.WithRevokedTokensCacheKeyPrefix(cfg.Cache.RevokedTokensKeyPrefix),
		)
	}

	opts = slices.AppendIfFunc(opts, cfg.IsIntrospectionConfigured(), func() []oidc.Option {
		return []oidc.Option{oidc.WithIntrospection(cfg.Introspection.ClientId, cfg.Introspection.ClientSecret.Expose())}
	})

	revOpts, err := b.buildRevocationOptions()
	if err != nil {
		return nil, err
	}
	opts = append(opts, revOpts...)

	opts = append(opts, b.buildSchedulerOptions()...)

	return opts, nil
}

// buildPresetOptions builds preset and preset-rule options from config.
func (b *ProviderBuilder) buildPresetOptions() ([]oidc.Option, error) {
	cfg := b.cfg
	if !cfg.IsPresetsConfigured() {
		return []oidc.Option{
			oidc.WithPresets(),
			oidc.WithPresetRules(),
		}, nil
	}

	var presets []*oidc.ValidationPreset
	for i := range cfg.Presets.List {
		preset, err := presetFromConfig(&cfg.Presets.List[i], cfg.ClockSkew)
		if err != nil {
			return nil, b.WrapError(err, "failed to build preset "+cfg.Presets.List[i].Name)
		}
		presets = append(presets, preset)
	}

	var presetRules []oidc.PresetRule
	for i := range cfg.Presets.Selectors {
		presetRules = append(presetRules, presetRuleFromConfig(&cfg.Presets.Selectors[i]))
	}

	return []oidc.Option{
		oidc.WithPresets(presets...),
		oidc.WithPresetRules(presetRules...),
	}, nil
}

// buildRevocationOptions builds revocation storage options. If revocation is
// configured and no custom storage was provided via [ProviderBuilder.UseRevocationStorage],
// a filter-based storage is created automatically (requires redis client).
func (b *ProviderBuilder) buildRevocationOptions() ([]oidc.Option, error) {
	cfg := b.cfg
	if !cfg.IsRevocationConfigured() || !cfg.Revocation.Enabled {
		return nil, nil
	}

	storage := b.revocationStorage
	if storage == nil {
		var err error
		storage, err = b.buildRevocationStorage(cfg.Revocation)
		if err != nil {
			return nil, err
		}
	}

	if storage == nil {
		return nil, nil
	}

	return []oidc.Option{
		oidc.WithRevocationStorage(storage),
		oidc.WithRevocationItemType(cfg.Revocation.ItemType),
	}, nil
}

// buildRevocationStorage creates a filter-based revocation storage from config.
func (b *ProviderBuilder) buildRevocationStorage(cfg *config.OIDCRevocation) (oidc.RevocationStorage, error) {
	if err := b.RequireDependency(b.redisClient, "redis client"); err != nil {
		return nil, err
	}

	pfDefaults := config.DefaultProbabilisticFilterDefaults()
	filter, err := probfilterfactory.NewFilter("oidc-revocation", cfg.Filter, &pfDefaults).
		UseLogger(b.Logger()).
		UseRedisClient(b.redisClient).
		Build()
	if err != nil {
		return nil, b.WrapError(err, "failed to create revocation filter")
	}

	var loader oidc.DataLoader
	if cfg.Source != nil {
		if cfg.Source.File != "" {
			loader = &oidc.FileRevocationLoader{Path: cfg.Source.File}
		} else if cfg.Source.URL != "" {
			loader = &oidc.URLRevocationLoader{
				URL:    cfg.Source.URL,
				Client: http.DefaultClient,
			}
		}
	}

	return oidc.NewFilterRevocationStorage(filter, loader), nil
}

// buildSchedulerOptions builds scheduler-related options if a scheduler is available.
func (b *ProviderBuilder) buildSchedulerOptions() []oidc.Option {
	if b.scheduler == nil {
		return nil
	}

	cfg := b.cfg
	opts := []oidc.Option{oidc.WithScheduler(b.scheduler)}

	if cfg.IsJWKSConfigured() && cfg.JWKS.RefreshEnabled && cfg.JWKS.RefreshSchedule != "" {
		opts = append(opts, oidc.WithJWKSRefreshSchedule(cfg.JWKS.RefreshSchedule))
	}

	if cfg.IsRevocationConfigured() && cfg.Revocation.Enabled &&
		cfg.Revocation.SyncEnabled && cfg.Revocation.SyncSchedule != "" &&
		(b.revocationStorage != nil || b.redisClient != nil) {
		opts = append(opts, oidc.WithRevocationSyncSchedule(cfg.Revocation.SyncSchedule))
	}

	return opts
}
