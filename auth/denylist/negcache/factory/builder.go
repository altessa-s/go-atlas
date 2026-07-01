// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"log/slog"

	"github.com/redis/go-redis/v9"

	"github.com/altessa-s/go-atlas/auth/denylist/negcache"
	"github.com/altessa-s/go-atlas/config"
	"github.com/altessa-s/go-atlas/observability/metrics"

	corefactory "github.com/altessa-s/go-atlas/core/factory"
	probfilterfactory "github.com/altessa-s/go-atlas/data/probfilter/factory"
)

// Builder assembles a [negcache.Cache] from a probabilistic-filter config and
// injected dependencies using a fluent API. Missing-dependency and
// filter-construction errors are reported at [Builder.Build] time. The builder
// is not safe for concurrent use.
//
// The negative filter is constructed via the shared probfilter factory and
// wired to the caller-supplied [negcache.Authoritative] revocation store. The
// factory never builds the authoritative store or a Redis client — those are
// external dependencies the caller injects.
type Builder struct {
	corefactory.Base
	name          string
	filterCfg     *config.ProbabilisticFilterConfig
	defaults      *config.ProbabilisticFilterDefaults
	authoritative negcache.Authoritative

	// Optional dependencies (set via Use*).
	redisClient      redis.UniversalClient
	metricsCollector metrics.Collector
	metricsSubsystem string
}

// NewBuilder creates a [Builder] for a negative cache named name (the name is
// passed to the probfilter factory for metric and storage-key scoping), built
// from filterCfg and defaults and fronting authoritative.
//
// filterCfg, defaults, and authoritative may be nil here — the errors surface
// at [Builder.Build] time.
func NewBuilder(
	name string,
	filterCfg *config.ProbabilisticFilterConfig,
	defaults *config.ProbabilisticFilterDefaults,
	authoritative negcache.Authoritative,
) *Builder {
	return &Builder{
		Base:          corefactory.NewBase(slog.New(slog.DiscardHandler)),
		name:          name,
		filterCfg:     filterCfg,
		defaults:      defaults,
		authoritative: authoritative,
	}
}

// Build validates the injected dependencies, builds the negative filter via the
// probfilter factory, and returns a [negcache.Cache] fronting the authoritative
// store. Lookup metrics are wired only when a collector was injected via
// [Builder.UseMetrics]. Accumulated fluent-step errors are reported here.
func (b *Builder) Build() (*negcache.Cache, error) {
	if err := b.RequireDependency(b.authoritative, "authoritative store"); err != nil {
		return nil, err
	}
	if err := b.RequireDependency(b.filterCfg, "filter configuration"); err != nil {
		return nil, err
	}

	filter, err := probfilterfactory.NewFilter(b.name, b.filterCfg, b.defaults).
		UseLogger(b.Logger()).
		UseRedisClient(b.redisClient).
		Build()
	if err != nil {
		return nil, b.WrapError(err, "failed to build negative filter")
	}

	var opts []negcache.Option
	if b.metricsCollector != nil {
		opts = append(opts, negcache.WithMetrics(negcache.NewMetrics(b.metricsCollector, b.metricsSubsystem)))
	}

	return negcache.New(filter, b.authoritative, opts...), nil
}
