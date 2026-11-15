// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package oidc

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/robfig/cron/v3"

	"github.com/altessa-s/go-horai/schedule"

	"github.com/altessa-s/go-atlas/cache/probfilter"

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
	o := p.opts
	var jobs []refreshJob
	if o.jwksRefreshEnabled && o.jwksRefreshSchedule != "" {
		jobs = append(jobs, refreshJob{name: "jwks-refresh", schedule: o.jwksRefreshSchedule, run: p.scheduledJWKSRefresh})
	}
	if p.revocationSyncScheduled() {
		jobs = append(jobs, refreshJob{
			name: "revocation-sync", schedule: o.revocationSyncSchedule, run: p.scheduledRevocationSync,
			staleAfter: o.revocationMaxStaleness,
		})
	}
	return jobs
}

// revocationSyncScheduled reports whether the provider syncs its revocation
// storage on [WithRevocationSyncSchedule].
func (p *Provider) revocationSyncScheduled() bool {
	return p.opts.revocationSyncEnabled && p.opts.revocationSyncSchedule != "" && p.revocationStorage != nil
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
	ctx, cancel := corecontext.ApplyTimeout(ctx, p.opts.jwksHTTPTimeout)
	defer cancel()
	return p.RefreshJWKS(ctx)
}

// scheduledRevocationSync is the revocation sync job. A shared filter that a
// peer replica is rebuilding right now is skipped silently, but not counted
// as a sync of this provider: whether that rebuild publishes is unknown. A
// superseded rebuild (this one stalled past its lease) is reported as a
// failed sync, although the filter stays consistent. Each run is bounded by
// [WithRevocationSyncTimeout].
//
// Whatever the outcome, the run ends by reading the time of the last sync
// committed by any replica ([Provider.refreshSharedRevocationSync]). That is
// what keeps a replica fresh that loses the rebuild lease on every tick, and
// only for as long as some peer does commit.
func (p *Provider) scheduledRevocationSync(ctx context.Context) error {
	defer p.refreshSharedRevocationSync(ctx)

	ctx, cancel := corecontext.ApplyTimeout(ctx, p.opts.revocationSyncTimeout)
	defer cancel()
	err := p.revocationStorage.Sync(ctx)
	if err == nil {
		p.markRevocationSynced()
		return nil
	}
	if errors.Is(err, probfilter.ErrRebuildInProgress) {
		return nil
	}
	p.metrics.revocationCheckErrors.Inc()
	return err
}

// Close releases resources held by the Provider: it cancels the background
// context used by JWKS storage, stops the refresh cron (waiting for a running
// job to return) and, when the HTTP client was handed over with
// [WithOwnedHTTPClient], closes its idle connections. A client the caller
// keeps — and may share with other components — is left alone. Nothing else
// stops the background work — canceling the context passed to [NewProvider]
// does not — so every provider must be closed.
func (p *Provider) Close() {
	p.cancelBackgroundCtx()
	if p.refreshCron != nil {
		<-p.refreshCron.Stop().Done()
	}
	if p.opts.httpClientOwned {
		p.client.CloseIdleConnections()
	}
}

// RefreshJWKS refreshes the JWKS keys from the discovery endpoint. The
// provider's own cron calls it on [WithJWKSRefreshSchedule]; callers may
// also invoke it directly. Concurrent calls collapse into one refresh.
func (p *Provider) RefreshJWKS(ctx context.Context) error {
	return p.jwksRefreshTask.Run(ctx)
}

// refreshJWKSInternal performs the actual JWKS refresh.
// Callers must route through RefreshJWKS so overlapping cycles collapse into
// a single execution.
func (p *Provider) refreshJWKSInternal(ctx context.Context) error {
	// A closed provider does no work, whether RefreshJWKS is called directly
	// or by a cron tick that raced with Close.
	if err := p.backgroundCtx.Err(); err != nil {
		return coreerrs.Wrap(err, "oidc provider closed")
	}
	p.metrics.jwksRefreshes.Inc()
	stop := p.metrics.jwksRefreshDuration.Start()
	defer stop()

	refreshErr := p.doRefreshJWKS(ctx)
	if refreshErr != nil {
		p.metrics.jwksRefreshErrors.Inc()
		return refreshErr
	}
	p.markJWKSRefreshed()
	return nil
}

func (p *Provider) doRefreshJWKS(ctx context.Context) error {
	jwks, err := p.fetchJWKS(ctx)
	if err != nil {
		return err
	}

	// The same policy as the initial load: a malformed verification key fails
	// the refresh as a whole. Skipping it and applying the rest would evict
	// keys that are still signing tokens whenever a glitch corrupts some of
	// the entries. Keys that cannot verify a token are not parsed at all.
	newKeys, err := parseJWKS(jwks.Keys)
	if err != nil {
		return fmt.Errorf("%w: JWK Set holds an invalid key; keeping the current keys: %w", ErrDiscovery, err)
	}

	// A document without a key that can verify a token ({}, an empty set, only
	// unsupported keys, only "enc" keys of a half-published rotation) is a failed
	// refresh, not a rotation: replacing a working key set with it would
	// reject every token until the next successful refresh.
	if len(newKeys) == 0 {
		current, err := p.jwks.Storage().KeyReadAll(ctx)
		if err != nil {
			return coreerrs.WrapOperation(err, "read JWKS storage")
		}
		if len(current) > 0 {
			return coreerrs.Wrap(ErrDiscovery, "JWK Set holds no key usable for verification; keeping the current keys")
		}
	}

	if err := p.jwks.Storage().KeyReplaceAll(ctx, newKeys); err != nil {
		return coreerrs.WrapOperation(err, "update JWKS storage")
	}

	return nil
}
