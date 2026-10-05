// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package oidc

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/MicahParks/jwkset"
	"github.com/robfig/cron/v3"

	"github.com/altessa-s/go-atlas/data/probfilter"

	corecontext "github.com/altessa-s/go-atlas/core/context"
	coreerrs "github.com/altessa-s/go-atlas/core/errors"
)

// cronParser accepts six-field cron expressions (with seconds) and
// descriptors such as "@every 5m", matching data/probfilter/factory.
var cronParser = cron.NewParser(
	cron.Second | cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor,
)

// refreshJob is one periodic background job of the provider.
type refreshJob struct {
	name     string
	schedule string
	run      func(ctx context.Context) error
}

// refreshJobs lists the periodic jobs configured by
// [WithJWKSRefreshSchedule] and [WithRevocationSyncSchedule].
func (p *Provider) refreshJobs(o *options) []refreshJob {
	var jobs []refreshJob
	if o.jwksRefreshEnabled && o.jwksRefreshSchedule != "" {
		jobs = append(jobs, refreshJob{name: "jwks-refresh", schedule: o.jwksRefreshSchedule, run: p.scheduledJWKSRefresh})
	}
	if o.revocationSyncEnabled && o.revocationSyncSchedule != "" && p.revocationStorage != nil {
		jobs = append(jobs, refreshJob{name: "revocation-sync", schedule: o.revocationSyncSchedule, run: p.scheduledRevocationSync})
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
	c := cron.New(cron.WithParser(cronParser), cron.WithChain(cron.SkipIfStillRunning(cron.DiscardLogger)))
	for _, job := range jobs {
		schedule, err := cronParser.Parse(job.schedule)
		if err != nil {
			return nil, coreerrs.Wrapf(err, "invalid %s schedule %q", job.name, job.schedule)
		}
		c.Schedule(schedule, cron.FuncJob(func() { p.runRefreshJob(job) }))
	}
	return c, nil
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
// peer replica is rebuilding right now is skipped silently: that rebuild
// publishes a fresh snapshot. A superseded rebuild (this one stalled past its
// lease) is reported as a failed sync, although the filter stays consistent.
func (p *Provider) scheduledRevocationSync(ctx context.Context) error {
	err := p.revocationStorage.Sync(ctx)
	if err == nil || errors.Is(err, probfilter.ErrRebuildInProgress) {
		return nil
	}
	p.metrics.revocationCheckErrors.Inc()
	return err
}

// Close releases resources held by the Provider: it cancels the background
// context used by JWKS storage, stops the refresh cron (waiting for a running
// job to return) and closes the HTTP client's idle connections.
func (p *Provider) Close() {
	p.cancelBackgroundCtx()
	if p.refreshCron != nil {
		<-p.refreshCron.Stop().Done()
	}
	p.client.CloseIdleConnections()
}

// RefreshJWKS refreshes the JWKS keys from the discovery endpoint. The
// provider's own cron calls it on [WithJWKSRefreshSchedule]; callers may
// also invoke it directly. Concurrent calls collapse into one refresh.
func (p *Provider) RefreshJWKS(ctx context.Context) error {
	return p.jwksRefreshTask.Run(ctx, p.refreshJWKSInternal)
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
	if p.discoveryInfo == nil || p.discoveryInfo.JwksURL == "" {
		return coreerrs.Wrap(ErrDiscovery, "JWKS URL not available")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.discoveryInfo.JwksURL, nil)
	if err != nil {
		return err
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return err
	}
	defer drainAndClose(resp)

	if resp.StatusCode != http.StatusOK {
		return coreerrs.Wrapf(ErrDiscovery, "unexpected status %d", resp.StatusCode)
	}

	var jwks jwkset.JWKSMarshal
	if err := json.NewDecoder(resp.Body).Decode(&jwks); err != nil {
		return coreerrs.WrapOperation(err, "decode JWK Set")
	}

	newKeys := make([]jwkset.JWK, 0, len(jwks.Keys))
	for _, marshal := range jwks.Keys {
		jwk, err := jwkset.NewJWKFromMarshal(marshal, jwkset.JWKMarshalOptions{Private: false}, jwkset.JWKValidateOptions{})
		if err != nil {
			p.logger.WarnContext(ctx, "skipping invalid JWK", slog.String("kid", marshal.KID), slog.Any("error", err))
			continue
		}
		newKeys = append(newKeys, jwk)
	}

	if err := p.jwks.Storage().KeyReplaceAll(ctx, newKeys); err != nil {
		return coreerrs.WrapOperation(err, "update JWKS storage")
	}

	return nil
}
