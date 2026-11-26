// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"cmp"
	"context"
	"errors"
	"log/slog"

	"github.com/altessa-s/go-atlas-core/collections/slices"
	"github.com/altessa-s/go-atlas-core/types/ptr"
	"github.com/altessa-s/go-atlas/auth/oidc"
	"github.com/altessa-s/go-atlas/internal/diagnostics"
	"github.com/altessa-s/go-atlas/observability/metrics"

	corehash "github.com/altessa-s/go-atlas-core/encoding/hash"
	httpclient "github.com/altessa-s/go-atlas-net/http/client"
	netproxy "github.com/altessa-s/go-atlas-net/proxyconfig"
	probfilter "github.com/altessa-s/go-atlas/cache/probfilter"
	authconfig "github.com/altessa-s/go-atlas/config/auth"
	probfilterconfig "github.com/altessa-s/go-atlas/config/probfilter"
	probfilterfactory "github.com/altessa-s/go-atlas/factory/cache/probfilter"
	corefactory "github.com/altessa-s/go-atlas/factory/core"
	httpclientfactory "github.com/altessa-s/go-atlas/factory/ext/net/httpclient"
)

// ProviderBuilder assembles an [oidc.Provider] from configuration and
// injected dependencies using a fluent API with deferred error accumulation.
//
// The revocation filter is built through the probfilter factory, which
// builds only in-memory storages; a Redis filter storage registers with
// [ProviderBuilder.UseRevocationBloomStorage] or
// [ProviderBuilder.UseRevocationCuckooStorage], passed through to it.
type ProviderBuilder struct {
	corefactory.Base
	cfg  *authconfig.OIDC
	errs []error

	// Dependencies (set via Use*).
	meter                   metrics.Meter
	bloomStorages           map[probfilterconfig.StorageType]probfilterfactory.BloomStorageFunc
	cuckooStorages          map[probfilterconfig.StorageType]probfilterfactory.CuckooStorageFunc
	tokenCache              oidc.Cacher
	revocationStorage       oidc.RevocationStorage
	revocationAuthoritative oidc.Authoritative
}

// New creates a new [ProviderBuilder] for the given OIDC config.
// A nil cfg is accepted; the error surfaces at [ProviderBuilder.Build] time.
func New(cfg *authconfig.OIDC) *ProviderBuilder {
	return &ProviderBuilder{
		Base: corefactory.NewBase(slog.New(slog.DiscardHandler)),
		cfg:  cfg,
	}
}

// Build assembles and returns the OIDC provider. All errors accumulated
// during the fluent chain are returned here.
func (b *ProviderBuilder) Build(ctx context.Context) (*oidc.Provider, error) {
	if err := corefactory.JoinErrors(b.errs); err != nil {
		return nil, err
	}

	if b.cfg == nil {
		return nil, corefactory.ErrConfigRequired
	}

	opts, err := b.buildProviderOptions(ctx)
	if err != nil {
		return nil, err
	}

	return oidc.NewProvider(ctx, b.cfg.DiscoveryUrl, opts...)
}

// buildProviderOptions translates config + builder deps into oidc.Option slice.
func (b *ProviderBuilder) buildProviderOptions(ctx context.Context) (_ []oidc.Option, buildErr error) {
	cfg := b.cfg

	opts := make([]oidc.Option, 0, estimatedOptionsCount)
	opts = append(opts, oidc.WithLogger(b.Logger()))
	if b.meter != nil {
		opts = append(opts, oidc.WithMeter(b.meter))
	}

	opts = slices.AppendIfFunc(opts, cfg.IsJWKSConfigured(), func() []oidc.Option {
		jwksOpts := []oidc.Option{oidc.WithJwksHTTPTimeout(cfg.JWKS.HTTPTimeout)}
		// An omitted bound keeps the provider default; an explicit zero disables it.
		return slices.AppendIf(jwksOpts, cfg.JWKS.MaxStaleness != nil, oidc.WithJWKSMaxStaleness(ptr.Unwrap(cfg.JWKS.MaxStaleness)))
	})

	if cfg.IsValidationConfigured() {
		valOpts, err := validationOptionsFromConfig(cfg.Validation, cfg.ClockSkew)
		if err != nil {
			return nil, b.WrapError(err, "failed to build validation options")
		}
		opts = append(opts, oidc.WithDefaultValidationOptions(valOpts...))
	}

	presetOpts, err := b.buildPresetOptions(ctx)
	if err != nil {
		return nil, err
	}
	opts = append(opts, presetOpts...)

	opts = slices.AppendIfFunc(opts, cfg.IsCacheConfigured() && cfg.Cache.Enabled && b.tokenCache != nil, func() []oidc.Option {
		return []oidc.Option{
			oidc.WithTokenCache(b.tokenCache),
			oidc.WithTokensCacheKeyPrefix(cfg.Cache.TokensKeyPrefix),
			oidc.WithRevokedTokensCacheKeyPrefix(cfg.Cache.RevokedTokensKeyPrefix),
		}
	})

	opts = slices.AppendIfFunc(opts, cfg.IsIntrospectionEnabled(), func() []oidc.Option {
		introspectionOpts := []oidc.Option{
			oidc.WithIntrospection(cfg.ClientCredentials.ClientId, cfg.ClientCredentials.ClientSecret.Expose()),
		}
		return slices.AppendIf(introspectionOpts, cfg.Introspection.FailOpen, oidc.WithIntrospectionFailOpen())
	})

	revOpts, cleanup, err := b.buildRevocationOptions(ctx)
	if err != nil {
		return nil, err
	}
	if cleanup != nil {
		defer func() {
			if buildErr != nil {
				if closeErr := cleanup(context.WithoutCancel(ctx)); closeErr != nil {
					b.Logger().Error("closing revocation filter failed", "cause", diagnostics.ErrorClass(closeErr))
				}
			}
		}()
		opts = append(opts, oidc.WithRevocationFilterCleanup(cleanup))
	}
	opts = append(opts, revOpts...)

	if cfg.Proxy != nil && cfg.Proxy.Mode != "" && cfg.Proxy.Mode != netproxy.ModeNone {
		return nil, b.WrapError(httpclient.ErrSSRFUnsafeTransport, "proxy requires a separately constructed client with SSRF protection disabled")
	}
	clientOpts, err := httpclientfactory.ProxyOptions(cfg.Proxy)
	if err != nil {
		return nil, b.WrapError(err, "failed to materialize oidc proxy options")
	}
	ssrfOpts, err := httpclientfactory.SSRFOptions(cfg.SSRF)
	if err != nil {
		return nil, b.WrapError(err, "failed to materialize oidc SSRF options")
	}
	clientOpts = append(clientOpts, ssrfOpts...)
	// The client is built for this provider alone, so the provider owns it
	// and closes its idle connections on Close.
	opts = append(opts, oidc.WithHTTPClient(httpclient.New(clientOpts...)), oidc.WithOwnedHTTPClient())

	opts = append(opts, b.buildRefreshOptions()...)

	return opts, nil
}

// buildPresetOptions builds preset and preset-rule options from config.
func (b *ProviderBuilder) buildPresetOptions(ctx context.Context) ([]oidc.Option, error) {
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
		presetRules = append(presetRules, presetRuleFromConfig(ctx, &cfg.Presets.Selectors[i]))
	}

	return []oidc.Option{
		oidc.WithPresets(presets...),
		oidc.WithPresetRules(presetRules...),
	}, nil
}

// buildRevocationOptions builds revocation storage options. If revocation is
// configured and no custom storage was provided via [ProviderBuilder.UseRevocationStorage],
// a filter-based storage is created automatically.
func (b *ProviderBuilder) buildRevocationOptions(ctx context.Context) ([]oidc.Option, func(context.Context) error, error) {
	cfg := b.cfg
	if !cfg.IsRevocationConfigured() || !cfg.Revocation.Enabled {
		return nil, nil, nil
	}

	var cleanup func(context.Context) error
	storage := b.revocationStorage
	if storage == nil {
		var err error
		storage, cleanup, err = b.buildRevocationStorage(ctx, cfg.Revocation)
		if err != nil {
			return nil, nil, err
		}
	}

	if storage == nil {
		return nil, nil, nil
	}

	opts := []oidc.Option{
		oidc.WithRevocationStorage(storage),
		oidc.WithRevocationItemType(cfg.Revocation.ItemType),
		// Passed as is: zero is the documented opt-out of the bound.
		oidc.WithRevocationMaxStaleness(cfg.Revocation.MaxStaleness),
		// Passed as is: zero is the documented opt-out of the timeout.
		oidc.WithRevocationSyncTimeout(cfg.Revocation.SyncTimeout),
	}
	opts = slices.AppendIf(opts, cfg.Revocation.AllowEmpty, oidc.WithRevocationAllowEmpty())
	return slices.AppendIf(opts, cfg.Revocation.FailOpen, oidc.WithRevocationFailOpen()), cleanup, nil
}

// buildRevocationStorage creates a filter-based revocation storage from config.
//
// The provider owns the filter's rebuilds: it syncs the storage (a filter
// rebuild from cfg.Source) once in oidc.NewProvider and then on
// cfg.SyncSchedule on its own cron. The probfilter factory therefore gets no
// data loader and defaults with Bloom rebuilds disabled, and a per-filter
// bloom.rebuildCron or bloom.rebuildOnStart: true — a second rebuild schedule
// for the same filter — is rejected.
func (b *ProviderBuilder) buildRevocationStorage(
	ctx context.Context, cfg *authconfig.OIDCRevocation,
) (oidc.RevocationStorage, func(context.Context) error, error) {
	if err := rejectFilterRebuildSettings(cfg.Filter); err != nil {
		return nil, nil, b.WrapError(err, "invalid revocation filter configuration")
	}

	pfDefaults := probfilterconfig.NewDefaults()
	pfDefaults.Bloom.RebuildOnStart = false
	pfDefaults.Bloom.RebuildCron = ""
	fb := probfilterfactory.NewFilter(revocationFilterName(b.cfg), cfg.Filter, &pfDefaults).
		UseLogger(b.Logger())
	for typ, build := range b.bloomStorages {
		fb.UseBloomStorage(typ, build)
	}
	for typ, build := range b.cuckooStorages {
		fb.UseCuckooStorage(typ, build)
	}
	filter, err := fb.Build(ctx)
	if err != nil {
		return nil, nil, b.WrapError(err, "failed to create revocation filter")
	}

	var loader probfilter.DataLoader
	if cfg.Source != nil {
		if cfg.Source.File != "" {
			loader = &oidc.FileRevocationLoader{Path: cfg.Source.File}
		} else if cfg.Source.URL != "" {
			// Client is intentionally left nil — oidc.NewProvider injects
			// the shared HTTP client (built here via httpclient.New and WithHTTPClient)
			// through the storage's SetHTTPClient, so revocation refresh reuses
			// the same pool, retry policy and proxy resolver as discovery/JWKS/userinfo.
			loader = &oidc.URLRevocationLoader{URL: cfg.Source.URL, MaxResponseBytes: cfg.MaxResponseBytes}
		}
	}

	// The confirmer is a builder dependency rather than a config field: the
	// filter is populated from cfg.Source (a file or URL list), so only the
	// assembling code knows whether an exact store holds that same set.
	// Confirming against an unrelated store would silently stop revoking.
	// Without one the storage runs in lossy mode — see
	// [oidc.NewFilterRevocationStorage].
	var cleanup func(context.Context) error
	if closer, ok := filter.(interface{ Close(context.Context) error }); ok {
		cleanup = closer.Close
	}
	return oidc.NewFilterRevocationStorage(filter, loader, b.revocationAuthoritative), cleanup, nil
}

// errFilterRebuildSettings rejects probfilter rebuild settings on the
// revocation filter, whose rebuilds the provider owns.
var errFilterRebuildSettings = errors.New("revocation.filter.bloom.rebuildCron and rebuildOnStart are not supported: " +
	"the provider rebuilds the revocation filter from revocation.source at startup and on revocation.syncSchedule")

// rejectFilterRebuildSettings fails when filter asks for rebuilds of its own:
// a non-empty bloom.rebuildCron or bloom.rebuildOnStart: true. Settings that
// disable them agree with the provider's ownership and are accepted.
func rejectFilterRebuildSettings(filter *probfilterconfig.Filter) error {
	if filter == nil || filter.Bloom == nil {
		return nil
	}
	bloom := filter.Bloom
	if (bloom.RebuildCron != nil && *bloom.RebuildCron != "") || (bloom.RebuildOnStart != nil && *bloom.RebuildOnStart) {
		return errFilterRebuildSettings
	}
	return nil
}

// revocationFilterName names the revocation filter (and so its Redis key)
// after the provider's revocation domain — discovery URL, item type and
// source — so independent providers sharing Redis never sync over each
// other's revocation sets.
func revocationFilterName(cfg *authconfig.OIDC) string {
	var source string
	if cfg.Revocation.Source != nil {
		source = cfg.Revocation.Source.URL + "\x00" + cfg.Revocation.Source.File
	}
	domain := cfg.DiscoveryUrl + "\x00" + cfg.Revocation.ItemType + "\x00" + source
	return corehash.SHA256HexWithPrefix("oidc-revocation-", domain)[:len("oidc-revocation-")+16]
}

// buildRefreshOptions maps the JWKS refresh and revocation sync schedules
// onto the provider's own, process-local refresh cron.
func (b *ProviderBuilder) buildRefreshOptions() []oidc.Option {
	cfg := b.cfg
	schedule := authconfig.DefaultOIDCJwks().RefreshSchedule
	if cfg.IsJWKSConfigured() {
		schedule = cmp.Or(cfg.JWKS.RefreshSchedule, schedule)
	}
	opts := []oidc.Option{oidc.WithJWKSRefreshSchedule(schedule)}

	opts = slices.AppendIfFunc(opts,
		cfg.IsRevocationConfigured() && cfg.Revocation.Enabled &&
			cfg.Revocation.SyncEnabled && cfg.Revocation.SyncSchedule != "",
		func() []oidc.Option {
			return []oidc.Option{oidc.WithRevocationSyncSchedule(cfg.Revocation.SyncSchedule)}
		})

	return opts
}
