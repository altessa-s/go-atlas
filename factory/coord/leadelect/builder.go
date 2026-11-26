// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"cmp"
	"context"
	"fmt"
	"log/slog"

	"github.com/altessa-s/go-atlas-core/runtime/appinfo"
	"github.com/altessa-s/go-atlas/coord/leadelect"
	"github.com/altessa-s/go-atlas/coord/leadelect/providers"
	"github.com/altessa-s/go-atlas/observability/metrics"

	lockconfig "github.com/altessa-s/go-atlas/config/lock"
	corefactory "github.com/altessa-s/go-atlas/factory/core"
)

// ProviderFunc creates the leader election provider of one provider type
// from the leader elector configuration. ctx bounds provider provisioning,
// such as creating the election bucket; logger and meter are the builder's.
type ProviderFunc func(ctx context.Context, cfg *lockconfig.LeaderElector, logger *slog.Logger, meter metrics.Meter) (providers.Provider, error)

// LeaderBuilder assembles a [leadelect.Leader] step by step using a fluent API.
// Create instances with [New]. Errors are accumulated and reported at [LeaderBuilder.Build] time.
// The builder is not safe for concurrent use.
//
// The builder builds no provider itself. The NATS one registers its
// [ProviderFunc] with [LeaderBuilder.UseProvider] from
// factory/coord/leadelect/providers/nats; a configured provider without one
// fails at Build naming the package to add. An empty provider is nats.
type LeaderBuilder struct {
	corefactory.Base
	cfg  *lockconfig.LeaderElector
	errs []error

	// Dependencies
	collector metrics.Meter
	providers map[lockconfig.LeaderElectorProvider]ProviderFunc

	// Config
	key    string
	nodeId string
}

// New creates a [LeaderBuilder] for the given leader elector config.
// Config can be nil — the error surfaces at [LeaderBuilder.Build] time.
// By default, key is set to [appinfo.Name].
func New(cfg *lockconfig.LeaderElector) *LeaderBuilder {
	return &LeaderBuilder{
		Base: corefactory.NewBase(slog.New(slog.DiscardHandler)),
		cfg:  cfg,
		key:  appinfo.Name,
	}
}

// Build assembles the leader elector. Errors from fluent methods are accumulated
// and reported here via [errors.Join].
func (b *LeaderBuilder) Build(ctx context.Context) (*leadelect.Leader, error) {
	if err := corefactory.JoinErrors(b.errs); err != nil {
		return nil, err
	}

	if b.cfg == nil {
		return nil, corefactory.ErrConfigRequired
	}
	if b.key == "" {
		return nil, corefactory.Errorf(corefactory.ErrConfigRequired, "leader election key is required")
	}
	if b.nodeId == "" {
		return nil, corefactory.Errorf(corefactory.ErrConfigRequired, "leader election node ID is required")
	}

	provider, err := b.createProvider(ctx)
	if err != nil {
		return nil, err
	}

	return leadelect.New(provider, b.key, b.nodeId,
		leadelect.WithTTL(b.cfg.Ttl),
		leadelect.WithMeter(b.collector),
	), nil
}

// createProvider creates the leader election provider registered for the
// configured provider type.
func (b *LeaderBuilder) createProvider(ctx context.Context) (providers.Provider, error) {
	typ := cmp.Or(b.cfg.Provider, lockconfig.LeaderElectorProviderNATS)
	build, ok := b.providers[typ]
	if !ok {
		if typ == lockconfig.LeaderElectorProviderNATS {
			return nil, corefactory.Errorf(corefactory.ErrNotRegistered, "leadelect provider %q is not registered: add factory/coord/leadelect/providers/nats "+
				"with UseProvider(%q, natsfactory.Builder(conn))", typ, typ)
		}
		return nil, corefactory.Errorf(corefactory.ErrUnsupported, "unsupported leader elector provider: %s", typ)
	}

	provider, err := build(ctx, b.cfg, b.Logger(), b.collector)
	if err != nil {
		return nil, b.WrapError(err, fmt.Sprintf("failed to create %s provider", typ))
	}

	return provider, nil
}
