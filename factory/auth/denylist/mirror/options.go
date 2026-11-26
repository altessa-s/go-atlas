// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"log/slog"
	"time"

	"github.com/altessa-s/go-atlas/observability/health"
	"github.com/altessa-s/go-atlas/observability/metrics"

	schedtask "github.com/altessa-s/go-horai/task"
)

// UseLogger sets the logger for the builder and the cache it constructs.
func (b *Builder) UseLogger(v *slog.Logger) *Builder {
	b.SetLogger(v)
	return b
}

// UseDefaultLogger sets the logger to [slog.Default].
func (b *Builder) UseDefaultLogger() *Builder {
	return b.UseLogger(slog.Default())
}

// UseScheduler sets the scheduler that runs the refresh task. It is required.
func (b *Builder) UseScheduler(v schedtask.Registrar) *Builder {
	b.scheduler = v
	return b
}

// UseHealthCoordinator sets the registrar that receives the mirror's health
// checker, which reports NotServing while the cache is unprimed or stale.
func (b *Builder) UseHealthCoordinator(v health.Registrar) *Builder {
	b.health = v
	return b
}

// UseHealthServiceName overrides [DefaultHealthServiceName].
func (b *Builder) UseHealthServiceName(v string) *Builder {
	b.healthName = v
	return b
}

// UseRefreshSchedule sets the cron expression of the refresh task, replacing
// [DefaultRefreshSchedule]. It is the revocation-propagation window.
func (b *Builder) UseRefreshSchedule(v string) *Builder {
	b.schedule = v
	return b
}

// UseMaxStaleness bounds the age of the snapshot the cache still trusts; older
// snapshots make verification fail with an unavailable-revocation error. Zero
// leaves the bound off. Set it to a few refresh intervals.
func (b *Builder) UseMaxStaleness(v time.Duration) *Builder {
	b.maxStaleness = v
	return b
}

// UseInsecureServeUnprimed lets the cache answer "not revoked" while it has no
// snapshot, and keeps [Builder.Build] from failing when priming fails. Use it
// only where an outage of the revocation source at startup must not block
// authentication.
func (b *Builder) UseInsecureServeUnprimed() *Builder {
	b.serveUnprimed = true
	return b
}

// UseMetrics enables the cache's telemetry using the given collector and
// subsystem. A nil collector leaves metrics disabled; an empty subsystem falls
// back to [mirror.DefaultMetricsSubsystem].
func (b *Builder) UseMetrics(collector metrics.Meter, subsystem string) *Builder {
	b.metricsCollector = collector
	b.metricsSubsystem = subsystem
	return b
}
