// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"cmp"
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/altessa-s/go-atlas/auth/oidc"
	"github.com/altessa-s/go-atlas/config"
	"github.com/altessa-s/go-atlas/core/collections/slices"

	corefactory "github.com/altessa-s/go-atlas/core/factory"
	corescheduler "github.com/altessa-s/go-atlas/core/scheduler"
	probfilterfactory "github.com/altessa-s/go-atlas/data/probfilter/factory"
)

const (
	// estimatedOptionsCount is the estimated number of provider options for capacity pre-allocation.
	estimatedOptionsCount = 5

	// estimatedValidationOptionsCount is the estimated number of validation options for capacity pre-allocation.
	estimatedValidationOptionsCount = 7
)

// Factory creates OIDC providers from configuration.
type Factory struct {
	corefactory.Base
	scheduler   corescheduler.TaskRegistrar
	redisClient redis.UniversalClient
	tokenCache  oidc.Cacher
}

// New creates a new Factory with the given options.
func New(opts ...Option) *Factory {
	cfg := newOptions(opts...)
	return &Factory{
		Base:        corefactory.NewBase(cfg.logger),
		scheduler:   cfg.scheduler,
		redisClient: cfg.redisClient,
		tokenCache:  cfg.tokenCache,
	}
}

// CreateProviderFromConfig creates an OIDC provider from configuration.
func (f *Factory) CreateProviderFromConfig(
	ctx context.Context,
	cfg *config.OIDC,
	revocationStorage oidc.RevocationStorage,
) (*oidc.Provider, error) {
	if cfg == nil {
		return nil, fmt.Errorf("configuration is required")
	}

	// Build options from configuration
	opts := make([]oidc.Option, 0, estimatedOptionsCount)
	opts = append(opts, oidc.WithLogger(f.Logger()))

	// Configure JWKS HTTP timeout only if JWKS is explicitly configured
	if cfg.IsJWKSConfigured() {
		opts = append(opts, oidc.WithJwksHTTPTimeout(cfg.JWKS.HTTPTimeout))
	}

	if cfg.IsValidationConfigured() {
		valOpts, err := f.ValidationOptionsFromConfig(cfg.Validation, cfg.ClockSkew)
		if err != nil {
			return nil, f.WrapError(err, "failed to build validation options")
		}
		opts = append(opts, oidc.WithDefaultValidationOptions(valOpts...))
	}

	// Add presets from configuration
	var presets []*oidc.ValidationPreset
	var presetRules []oidc.PresetRule

	if cfg.IsPresetsConfigured() {
		for i := range cfg.Presets.List {
			preset, err := f.PresetFromConfig(&cfg.Presets.List[i], cfg.ClockSkew)
			if err != nil {
				return nil, f.WrapError(err, "failed to build preset "+cfg.Presets.List[i].Name)
			}
			presets = append(presets, preset)
		}

		for i := range cfg.Presets.Selectors {
			presetRules = append(presetRules, PresetRuleFromConfig(&cfg.Presets.Selectors[i])) //nolint:contextcheck // pure config conversion, no context needed
		}
	}

	opts = append(opts, oidc.WithPresets(presets...))
	opts = append(opts, oidc.WithPresetRules(presetRules...))

	// Configure token cache if enabled
	if cfg.IsCacheConfigured() && cfg.Cache.Enabled && f.tokenCache != nil {
		opts = append(opts, oidc.WithTokenCache(f.tokenCache),
			oidc.WithTokensCacheKeyPrefix(cfg.Cache.TokensKeyPrefix),
			oidc.WithRevokedTokensCacheKeyPrefix(cfg.Cache.RevokedTokensKeyPrefix),
		)
	}

	opts = slices.AppendIfFunc(opts, cfg.IsIntrospectionConfigured(), func() []oidc.Option {
		return []oidc.Option{oidc.WithIntrospection(cfg.Introspection.ClientId, cfg.Introspection.ClientSecret.Expose())}
	})

	// Configure revocation storage if enabled
	if cfg.IsRevocationConfigured() && cfg.Revocation.Enabled && revocationStorage != nil {
		opts = append(opts,
			oidc.WithRevocationStorage(revocationStorage),
			oidc.WithRevocationItemType(cfg.Revocation.ItemType),
		)
	}

	// Configure scheduler and background tasks if scheduler is available
	if f.scheduler != nil {
		opts = append(opts, oidc.WithScheduler(f.scheduler))

		// Configure JWKS refresh task
		if cfg.IsJWKSConfigured() && cfg.JWKS.RefreshEnabled && cfg.JWKS.RefreshSchedule != "" {
			opts = append(opts,
				oidc.WithJWKSRefreshSchedule(cfg.JWKS.RefreshSchedule))
		}

		// Configure revocation sync task
		if cfg.IsRevocationConfigured() && cfg.Revocation.Enabled && cfg.Revocation.SyncEnabled && cfg.Revocation.SyncSchedule != "" && revocationStorage != nil {
			opts = append(opts,
				oidc.WithRevocationSyncSchedule(cfg.Revocation.SyncSchedule))
		}
	}

	return oidc.NewProvider(ctx, cfg.DiscoveryUrl, opts...)
}

// ValidationOptionsFromConfig constructs validation options from configuration.
func (f *Factory) ValidationOptionsFromConfig(cfg *config.OIDCValidation, clockSkew time.Duration) ([]oidc.ValidationOption, error) {
	if cfg == nil {
		return nil, fmt.Errorf("configuration is required")
	}

	opts := make([]oidc.ValidationOption, 0, estimatedValidationOptionsCount)
	opts = append(opts,
		oidc.WithValidationIssuedAt(),
		oidc.WithValidationMaxTokenLifetime(cfg.MaxTokenLifetime),
		oidc.WithValidationLeeway(clockSkew),
		oidc.WithValidationIssuer(cfg.Issuer),
	)
	if cel := f.buildCELValidationRule(cfg.Expression, "custom-validation"); cel != nil {
		opts = append(opts, cel)
	}

	opts = slices.AppendIf(opts, cfg.IsClaimsConfigured(), f.buildClaimsValidationOptions(cfg.Claims)...)

	return opts, nil
}

// PresetFromConfig creates a validation preset from configuration.
func (f *Factory) PresetFromConfig(preset *config.OIDCPreset, clockSkew time.Duration) (*oidc.ValidationPreset, error) {
	if preset == nil {
		return nil, fmt.Errorf("configuration is required")
	}
	opts := make([]oidc.ValidationOption, 0, estimatedValidationOptionsCount)
	opts = append(opts,
		oidc.WithValidationIssuedAt(),
		oidc.WithValidationLeeway(clockSkew),
		oidc.WithValidationMaxTokenLifetime(preset.MaxTokenLifetime),
		oidc.WithValidationIssuer(preset.Issuer),
	)
	opts = slices.AppendIf(opts, preset.IsClaimsConfigured(), f.buildClaimsValidationOptions(preset.Claims)...)
	if cel := f.buildCELValidationRule(preset.Expression, preset.Name+"-validation"); cel != nil {
		opts = append(opts, cel)
	}

	return oidc.NewValidationPreset(preset.Name, opts...), nil
}

// PresetRuleFromConfig creates a preset selection rule from configuration.
func PresetRuleFromConfig(selector *config.OIDCSelector) oidc.PresetRule {
	return oidc.PresetRule{
		Priority:   selector.Priority,
		Matcher:    oidc.CELMatcher(selector.Expression),
		PresetName: selector.PresetName,
	}
}

// buildCELValidationRule creates a CEL validation rule from expression configuration.
func (f *Factory) buildCELValidationRule(expr *config.OIDCExpression, defaultName string) oidc.ValidationOption {
	if expr == nil || expr.Expression == "" {
		return nil
	}

	rule := oidc.CELValidationRule{
		Name:       cmp.Or(expr.Name, defaultName),
		Expression: expr.Expression,
	}

	return oidc.WithValidationCelRules(rule)
}

// buildClaimsValidationOptions creates validation options from claims configuration.
func (f *Factory) buildClaimsValidationOptions(claims *config.OIDCClaims) []oidc.ValidationOption {
	if claims == nil {
		return nil
	}

	opts := []oidc.ValidationOption{
		oidc.WithValidationAudience(claims.Audience...),
		oidc.WithValidationRequiredClaims(claims.Required...),
		oidc.WithValidationExpectedClaims(claims.Expected),
		oidc.WithValidationIgnoredClaims(claims.Ignored...),
		oidc.WithValidationAllowedClientIDs(claims.AllowedClientIds...),
		oidc.WithValidationRequiredScopes(claims.RequiredScopes...),
	}

	opts = slices.AppendIf(opts, claims.AllowMissingSubject, oidc.WithValidationAllowMissingSubject())

	if claims.RequireAuthorizedParty && len(claims.AllowedAuthorizedParties) > 0 {
		opts = append(opts,
			oidc.WithValidationRequireAuthorizedParty(),
			oidc.WithValidationAllowedAuthorizedParties(claims.AllowedAuthorizedParties...))
	}

	return opts
}

// CreateRevocationStorageFromConfig creates a revocation storage from configuration.
func (f *Factory) CreateRevocationStorageFromConfig(cfg *config.OIDCRevocation) (oidc.RevocationStorage, error) {
	if cfg == nil {
		return nil, fmt.Errorf("configuration is required")
	}

	if err := f.RequireDependency(f.redisClient, "redis client"); err != nil {
		return nil, err
	}

	// 1. Create filter
	pfFactory := probfilterfactory.New(
		probfilterfactory.WithLogger(f.Logger()),
		probfilterfactory.WithRedisClient(f.redisClient),
	)

	pfDefaults := config.DefaultProbabilisticFilterDefaults()
	filter, err := pfFactory.CreateFilterFromConfig("oidc-revocation", cfg.Filter, &pfDefaults)
	if err != nil {
		return nil, f.WrapError(err, "failed to create revocation filter")
	}

	// 2. Create loader
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
