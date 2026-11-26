// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"cmp"
	"context"
	"log/slog"
	"time"

	"github.com/altessa-s/go-atlas-core/collections/slices"
	"github.com/altessa-s/go-atlas-core/types/nilcheck"
	"github.com/altessa-s/go-atlas/auth/denylist/mirror"
	"github.com/altessa-s/go-atlas/observability/health"
	"github.com/altessa-s/go-atlas/observability/metrics"

	corefactory "github.com/altessa-s/go-atlas/factory/core"
	schedtask "github.com/altessa-s/go-horai/task"
)

const (
	// DefaultRefreshSchedule is the refresh cadence used when none is set:
	// the revocation-propagation window of the mirror.
	DefaultRefreshSchedule = "@every 30s"

	// DefaultRefreshTimeout bounds one scheduled refresh.
	DefaultRefreshTimeout = 20 * time.Second

	// RefreshTaskID is the scheduler task that refreshes the mirror.
	RefreshTaskID = "denylist-mirror-refresh"

	// DefaultHealthServiceName is the health service the mirror reports under.
	DefaultHealthServiceName = "denylist.mirror"
)

// Builder assembles a [mirror.Cache] around a revocation source: it primes the
// cache with the build context, registers the refresh task with the scheduler
// and, when a health registrar is set, a checker that reports NotServing while
// the cache refuses to answer. The builder is not safe for concurrent use.
//
// A mirror that is never primed or never refreshed would make every token with
// a jti fail verification, so [Builder.Build] returns the priming error and a
// scheduler is mandatory.
type Builder struct {
	corefactory.Base
	src mirror.Source

	scheduler        schedtask.Registrar
	health           health.Registrar
	healthName       string
	schedule         string
	maxStaleness     time.Duration
	serveUnprimed    bool
	metricsCollector metrics.Meter
	metricsSubsystem string
}

// NewBuilder creates a [Builder] for a mirror reading src. src may be nil here;
// the error surfaces at [Builder.Build] time.
func NewBuilder(src mirror.Source) *Builder {
	return &Builder{Base: corefactory.NewBase(slog.New(slog.DiscardHandler)), src: src}
}

// Build validates the dependencies, creates the cache, primes it with ctx,
// registers the refresh task and the health checker, and returns the cache. A
// priming failure fails Build unless [Builder.UseInsecureServeUnprimed] was
// set, in which case it is logged and the cache serves "not revoked" until a
// scheduled refresh succeeds.
func (b *Builder) Build(ctx context.Context) (*mirror.Cache, error) {
	if err := b.RequireDependency(b.src, "revocation source"); err != nil {
		return nil, err
	}
	if err := b.RequireDependency(b.scheduler, "scheduler"); err != nil {
		return nil, err
	}

	var opts []mirror.Option
	opts = append(opts, mirror.WithLogger(b.Logger()))
	opts = slices.AppendIf(opts, b.maxStaleness > 0, mirror.WithMaxStaleness(b.maxStaleness))
	opts = slices.AppendIf(opts, b.serveUnprimed, mirror.WithInsecureServeUnprimed())
	opts = slices.AppendIf(opts, b.metricsCollector != nil, mirror.WithMetrics(mirror.NewMetrics(b.metricsCollector, b.metricsSubsystem)))
	cache := mirror.New(b.src, opts...)

	if err := cache.Prime(ctx); err != nil {
		if !b.serveUnprimed {
			return nil, b.WrapError(err, "prime denylist mirror")
		}
		b.Logger().Error("denylist mirror: priming failed, serving without revocation checks until a refresh succeeds",
			slog.Any("error", err))
	}

	if err := b.scheduler.Register(ctx, schedtask.Config{
		ID:          RefreshTaskID,
		Description: "Refresh the local snapshot of the distributed denylist",
		Schedule:    cmp.Or(b.schedule, DefaultRefreshSchedule),
		Func:        cache.Refresh,
		Priority:    schedtask.PriorityHigh,
		Timeout:     DefaultRefreshTimeout,
	}); err != nil {
		return nil, b.WrapError(err, "register denylist mirror refresh task")
	}

	if !nilcheck.IsNil(b.health) {
		b.health.RegisterService(cmp.Or(b.healthName, DefaultHealthServiceName), health.Func(func(context.Context) health.ServingStatus {
			if cache.Ready() {
				return health.StatusServing
			}
			return health.StatusNotServing
		}))
	}
	return cache, nil
}
