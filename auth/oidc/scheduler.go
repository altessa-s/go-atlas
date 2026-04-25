// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package oidc

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/MicahParks/jwkset"

	"github.com/altessa-s/go-atlas/core/types/nilcheck"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
	corescheduler "github.com/altessa-s/go-atlas/core/scheduler"
)

// registerSchedulerTasks registers background tasks with the scheduler if configured.
func (p *Provider) registerSchedulerTasks(o *options) error {
	if nilcheck.IsNil(p.scheduler) {
		return nil
	}

	ctx := context.Background() // Use background context for registration

	// Register JWKS refresh task if enabled and schedule is configured
	if o.jwksRefreshEnabled && o.jwksRefreshSchedule != "" {
		taskCfg := corescheduler.TaskConfig{
			ID:          "oidc-jwks-refresh",
			Description: "Refresh OIDC JWKS keys from discovery endpoint",
			Func:        p.RegisterJWKSRefreshSchedulerFunc(),
			Schedule:    o.jwksRefreshSchedule,
			Priority:    corescheduler.TaskPriorityNormal,
		}

		if err := p.scheduler.Register(ctx, taskCfg); err != nil {
			return coreerrs.WrapOperation(err, "register JWKS refresh task")
		}
		p.logger.DebugContext(ctx, "registered JWKS refresh task",
			slog.String("schedule", o.jwksRefreshSchedule))
	}

	// Register revocation sync task if enabled and revocation storage is available
	if o.revocationSyncEnabled && o.revocationSyncSchedule != "" && p.revocationStorage != nil {
		taskCfg := corescheduler.TaskConfig{
			ID:          "oidc-revocation-sync",
			Description: "Synchronize OIDC token revocation list",
			Func:        p.revocationStorage.Sync,
			Schedule:    o.revocationSyncSchedule,
			Priority:    corescheduler.TaskPriorityNormal,
		}

		if err := p.scheduler.Register(ctx, taskCfg); err != nil {
			return coreerrs.WrapOperation(err, "register revocation sync task")
		}
		p.logger.DebugContext(ctx, "registered revocation sync task",
			slog.String("schedule", o.revocationSyncSchedule))
	}

	return nil
}

// Close releases resources held by the Provider, including canceling
// the background context used by JWKS storage.
func (p *Provider) Close() {
	p.cancelBackgroundCtx()
}

// RegisterJWKSRefreshSchedulerFunc returns a function for use by a scheduler and marks
// JWKS refresh as scheduler-managed. After calling this method, direct calls to
// RefreshJWKS will return ErrSchedulerManaged.
func (p *Provider) RegisterJWKSRefreshSchedulerFunc() func(context.Context) error {
	p.schedulerJWKSRefreshRegistered.Store(true)
	return p.refreshJWKSInternal
}

// RefreshJWKS manually triggers a refresh of the JWKS keys from the discovery endpoint.
// This method is designed to be called manually for one-time refresh.
// If the function is registered with a scheduler, this method returns ErrSchedulerManaged.
func (p *Provider) RefreshJWKS(ctx context.Context) error {
	if p.schedulerJWKSRefreshRegistered.Load() {
		return ErrSchedulerManaged
	}
	return p.refreshJWKSInternal(ctx)
}

// refreshJWKSInternal performs the actual JWKS refresh.
// It is safe to call concurrently; if already running, returns immediately.
func (p *Provider) refreshJWKSInternal(ctx context.Context) error {
	// Prevent concurrent execution
	if !p.jwksRefreshRunning.CompareAndSwap(false, true) {
		return nil // Already running, skip this cycle
	}
	defer p.jwksRefreshRunning.Store(false)

	p.metrics.jwksRefreshes.Inc()
	stop := p.metrics.jwksRefreshDuration.Start()
	defer stop()

	refreshErr := p.doRefreshJWKS(ctx)
	if refreshErr != nil {
		p.metrics.jwksRefreshErrors.Inc()
	}
	return refreshErr
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
