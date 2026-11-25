// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package oidc

import (
	"context"
	"log/slog"
	"time"

	"github.com/robfig/cron/v3"

	"github.com/altessa-s/go-horai/schedule"

	"github.com/altessa-s/go-atlas/internal/diagnostics"

	corecontext "github.com/altessa-s/go-atlas-core/context"
	coreerrs "github.com/altessa-s/go-atlas-core/errors"
)

// refreshJob is one periodic background job of the provider.
type refreshJob struct {
	name     string
	schedule string
	run      func(ctx context.Context) error
	// staleAfter is the freshness bound of what the job refreshes; zero: none.
	staleAfter time.Duration
}

// refreshJobs lists the periodic jobs configured by
// [WithJWKSRefreshSchedule] and [WithRevocationSyncSchedule].
func (p *Provider) refreshJobs() []refreshJob {
	var jobs []refreshJob
	if p.keys.schedule != "" {
		jobs = append(jobs, refreshJob{name: "jwks-refresh", schedule: p.keys.schedule, run: p.scheduledJWKSRefresh})
	}
	if p.revocation.syncScheduled() {
		jobs = append(jobs, refreshJob{
			name: "revocation-sync", schedule: p.revocation.syncSchedule, run: p.revocation.scheduledRevocationSync,
			staleAfter: p.revocation.maxStaleness,
		})
	}
	return jobs
}

// newRefreshCron builds the provider-owned, process-local cron that runs
// jobs. Every provider instance (and so every replica) refreshes its own keys
// and revocation state; nothing is shared through a distributed scheduler.
// Overlapping ticks of one job are skipped. An invalid schedule is an error.
// The cron is started by [NewProvider] only after construction succeeded and
// stopped by [Provider.Close].
func (p *Provider) newRefreshCron(jobs []refreshJob) (*cron.Cron, error) {
	c := cron.New(cron.WithChain(cron.SkipIfStillRunning(cron.DiscardLogger)))
	for _, job := range jobs {
		sched, err := schedule.Parse(job.schedule)
		if err != nil {
			return nil, coreerrs.Wrapf(err, "invalid %s schedule", job.name)
		}
		// A bound shorter than the gap between two runs rejects tokens
		// between them even when every run succeeds.
		if next := sched.Next(time.Now()); job.staleAfter > 0 && sched.Next(next).Sub(next) > job.staleAfter {
			p.logger.Warn("oidc background job runs less often than its staleness bound allows",
				slog.String("job", job.name), slog.String("schedule", job.schedule), slog.Duration("max_staleness", job.staleAfter))
		}
		p.scheduleRefreshJob(c, sched, job)
	}
	return c, nil
}

// scheduleRefreshJob makes c run job on sched through [Provider.runRefreshJob].
func (p *Provider) scheduleRefreshJob(c *cron.Cron, sched cron.Schedule, job refreshJob) {
	c.Schedule(sched, cron.FuncJob(func() { p.runRefreshJob(job) }))
}

// runRefreshJob runs job under the provider's background context; a closed
// provider does no work. Failures are logged (and counted by the job).
func (p *Provider) runRefreshJob(job refreshJob) {
	ctx := p.backgroundCtx
	if ctx.Err() != nil {
		return
	}
	if err := job.run(ctx); err != nil && ctx.Err() == nil {
		p.logger.WarnContext(ctx, "oidc background job failed",
			slog.String("job", job.name), slog.Any("error", err))
	}
}

// scheduledJWKSRefresh is the JWKS refresh job, bounded by the JWKS HTTP timeout.
func (p *Provider) scheduledJWKSRefresh(ctx context.Context) error {
	ctx, cancel := corecontext.ApplyTimeout(ctx, p.keys.jwksHTTPTimeout)
	defer cancel()
	return p.RefreshJWKS(ctx)
}

// Close releases resources held by the Provider: it cancels the background
// context used by JWKS storage, stops the refresh cron (waiting for a running
// job to return) and, when the HTTP client was handed over with
// [WithOwnedHTTPClient], closes its idle connections. A client the caller
// keeps — and may share with other components — is left alone. Nothing else
// stops the background work — canceling the context passed to [NewProvider]
// does not — so every provider must be closed.
func (p *Provider) Close() {
	p.closeOnce.Do(p.close)
}

func (p *Provider) close() {
	p.cancelBackgroundCtx()
	if p.refreshCron != nil {
		<-p.refreshCron.Stop().Done()
	}
	// Join any active recovery before releasing its filter.
	p.revocation.syncMu.Lock()
	defer p.revocation.syncMu.Unlock()
	p.healthMu.Lock()
	unregister := p.unregisterHealth
	p.unregisterHealth = nil
	p.healthMu.Unlock()
	if unregister != nil {
		unregister()
	}
	if cleanup := p.revocation.filterCleanup; cleanup != nil {
		if err := cleanup(context.WithoutCancel(p.backgroundCtx)); err != nil {
			p.logger.Error("closing revocation filter failed", "cause", diagnostics.ErrorClass(err))
		}
	}
	if p.ownedHTTPClient != nil {
		p.ownedHTTPClient.CloseIdleConnections()
	}
}

// RefreshJWKS refreshes the JWKS keys from the discovery endpoint. The
// provider's own cron calls it on [WithJWKSRefreshSchedule]; callers may
// also invoke it directly. Concurrent calls collapse into one refresh.
func (p *Provider) RefreshJWKS(ctx context.Context) error {
	return p.keys.jwksRefreshTask.Run(ctx)
}
