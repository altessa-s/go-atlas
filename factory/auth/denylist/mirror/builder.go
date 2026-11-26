// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"cmp"
	"context"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/altessa-s/go-atlas-core/collections/slices"
	"github.com/altessa-s/go-atlas-core/types/nilcheck"
	"github.com/altessa-s/go-atlas/auth/denylist/mirror"
	"github.com/altessa-s/go-atlas/observability/health"
	"github.com/altessa-s/go-atlas/observability/metrics"

	coreerrs "github.com/altessa-s/go-atlas-core/errors"
	corefactory "github.com/altessa-s/go-atlas/factory/core"
	schedtask "github.com/altessa-s/go-horai/task"
)

const (
	// DefaultRefreshSchedule is the refresh cadence used when none is set:
	// the revocation-propagation window of the mirror.
	DefaultRefreshSchedule = "@every 30s"

	// DefaultRefreshTimeout bounds one scheduled refresh and the priming
	// refresh of Build.
	DefaultRefreshTimeout = 20 * time.Second

	// RefreshTaskID is the prefix of the scheduler task that refreshes a mirror;
	// the task ID is this, a dash and the mirror's name or build number.
	RefreshTaskID = "denylist-mirror-refresh"

	// DefaultHealthServiceName is the prefix of the health service a mirror
	// reports under; a dot and the mirror's name or build number follow.
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
	name             string
	primeTimeout     time.Duration
	schedule         string
	maxStaleness     time.Duration
	serveUnprimed    bool
	metricsCollector metrics.Meter
	metricsSubsystem string
}

// NewBuilder creates a [Builder] for a mirror reading src. src may be nil here;
// the error surfaces at [Builder.Build] time.
func NewBuilder(src mirror.Source) *Builder {
	return &Builder{Base: corefactory.NewBase(slog.New(slog.DiscardHandler)), src: src,
		maxStaleness: mirror.DefaultMaxStaleness}
}

// Managed is a [mirror.Cache] together with the scheduler task and health
// registration the [Builder] created for it. The embedded cache serves
// lookups; [Managed.Close] releases the registrations.
type Managed struct {
	*mirror.Cache
	closed           *atomic.Bool
	unregisterHealth func()
}

// Close makes the refresh task a no-op and releases the health registration.
// The task stays registered but inert: a named mirror shares its task ID across
// replicas, so removing it here would stop refreshes everywhere. The cache
// itself holds no resources and keeps answering. It is safe to call repeatedly.
func (m *Managed) Close() {
	m.closed.Store(true)
	if m.unregisterHealth != nil {
		m.unregisterHealth()
	}
}

// Build validates the dependencies, creates the cache, primes it with ctx,
// registers the refresh task and the health checker, and returns the cache
// with its registrations, to be released with [Managed.Close]. A
// priming failure fails Build unless [Builder.UseInsecureServeUnprimed] was
// set, in which case it is logged and the cache serves "not revoked" until a
// scheduled refresh succeeds.
func (b *Builder) Build(ctx context.Context) (*Managed, error) {
	if err := b.RequireDependency(b.src, "revocation source"); err != nil {
		return nil, err
	}
	if err := b.RequireDependency(b.scheduler, "scheduler"); err != nil {
		return nil, err
	}
	schedule := cmp.Or(b.schedule, DefaultRefreshSchedule)
	if err := b.checkStaleness(schedule); err != nil {
		return nil, err
	}
	taskID, healthName := b.identity()

	var opts []mirror.Option
	opts = append(opts, mirror.WithLogger(b.Logger()))
	opts = append(opts, mirror.WithMaxStaleness(b.maxStaleness))
	opts = slices.AppendIf(opts, b.serveUnprimed, mirror.WithInsecureServeUnprimed())
	opts = slices.AppendIf(opts, b.metricsCollector != nil, mirror.WithMetrics(mirror.NewMetrics(b.metricsCollector, b.metricsSubsystem)))
	cache := mirror.New(b.src, opts...)

	primeCtx, cancel := context.WithTimeout(ctx, cmp.Or(b.primeTimeout, DefaultRefreshTimeout))
	defer cancel()
	if err := cache.Prime(primeCtx); err != nil {
		if !b.serveUnprimed {
			return nil, b.WrapError(err, "prime denylist mirror")
		}
		b.Logger().Error("denylist mirror: priming failed, serving without revocation checks until a refresh succeeds",
			slog.Any("error", err)) // a refresh error names the source error's type, never its text
	}

	var closed atomic.Bool
	if err := b.scheduler.Register(ctx, schedtask.Config{
		ID:          taskID,
		Description: "Refresh the local snapshot of the distributed denylist",
		Schedule:    schedule,
		Func: func(ctx context.Context) error {
			if closed.Load() {
				return nil
			}
			return cache.Refresh(ctx)
		},
		Priority: schedtask.PriorityHigh,
		Timeout:  DefaultRefreshTimeout,
	}); err != nil {
		return nil, b.WrapError(err, "register denylist mirror refresh task")
	}

	managed := &Managed{Cache: cache, closed: &closed}
	if !nilcheck.IsNil(b.health) {
		managed.unregisterHealth = sync.OnceFunc(health.RegisterOwnedChecker(b.health, healthName,
			health.Func(func(context.Context) health.ServingStatus {
				if cache.Ready() {
					return health.StatusServing
				}
				return health.StatusNotServing
			})))
	}
	return managed, nil
}

// mirrorSeq numbers the unnamed mirrors built in this process.
var mirrorSeq atomic.Int64

// identity returns the scheduler task ID and health service name of this
// mirror. A scheduler replaces a task registered under an existing ID, so every
// mirror needs its own: a named one derives them from its name, an unnamed one
// from the order it is built in.
func (b *Builder) identity() (taskID, healthName string) {
	taskID, healthName = RefreshTaskID, DefaultHealthServiceName
	switch {
	case b.name != "":
		taskID, healthName = taskID+"-"+b.name, healthName+"."+b.name
	default:
		suffix := strconv.FormatInt(mirrorSeq.Add(1), 10)
		taskID, healthName = taskID+"-"+suffix, healthName+"."+suffix
	}
	return taskID, cmp.Or(b.healthName, healthName)
}

// checkStaleness rejects a maximum staleness shorter than the refresh interval
// of an "@every" schedule: every gap between two refreshes would then refuse
// all tokens. Other cron expressions cannot be measured and are not checked.
func (b *Builder) checkStaleness(schedule string) error {
	spec, ok := strings.CutPrefix(schedule, "@every ")
	if !ok || b.maxStaleness <= 0 {
		return nil
	}
	interval, err := time.ParseDuration(strings.TrimSpace(spec))
	if err != nil {
		return coreerrs.Wrapf(coreerrs.ErrInvalidArgument, "refresh schedule %q", schedule)
	}
	if b.maxStaleness < interval {
		return coreerrs.Wrapf(coreerrs.ErrInvalidArgument,
			"max staleness %s is shorter than the refresh interval %s", b.maxStaleness, interval)
	}
	return nil
}
