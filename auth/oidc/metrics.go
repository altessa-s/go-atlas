// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package oidc

import (
	"github.com/altessa-s/go-atlas/observability/metrics"
)

// DefaultMetricsSubsystem is the Prometheus subsystem prefix applied to every
// metric this package emits. It follows the auth_<package> scheme shared with
// the sibling auth/* packages (auth_static, auth_selfjwt, auth_opa).
const DefaultMetricsSubsystem = "auth_oidc"

// oidcMetrics holds all Prometheus metrics for the OIDC provider.
// When no [metrics.Collector] is provided, [metrics.Noop] is used and
// all methods become zero-cost no-ops.
type oidcMetrics struct {
	tokenValidations      metrics.Counter
	validationErrors      metrics.Counter
	validationDuration    metrics.Timer
	cacheHits             metrics.Counter
	cacheMisses           metrics.Counter
	jwksRefreshes         metrics.Counter
	jwksRefreshErrors     metrics.Counter
	jwksRefreshDuration   metrics.Timer
	jwksStaleRejections   metrics.Counter
	revocationCheckErrors metrics.Counter
}

func newOIDCMetrics(c metrics.Collector) *oidcMetrics {
	if c == nil {
		c = metrics.Noop()
	}

	scoped := c.WithSubsystem(DefaultMetricsSubsystem)

	return &oidcMetrics{
		tokenValidations: scoped.MustCounter(metrics.MetricOpts{
			Name:       "token_validations_total",
			Help:       "Total number of token validation attempts.",
			LabelNames: []string{"issuer"},
		}),
		validationErrors: scoped.MustCounter(metrics.MetricOpts{
			Name:       "validation_errors_total",
			Help:       "Total number of token validation errors.",
			LabelNames: []string{"issuer"},
		}),
		validationDuration: scoped.MustTimer(metrics.HistogramOpts{
			MetricOpts: metrics.MetricOpts{
				Name: "validation_duration_seconds",
				Help: "Duration of token validation operations in seconds.",
			},
		}),
		cacheHits: scoped.MustCounter(metrics.MetricOpts{
			Name: "cache_hits_total",
			Help: "Total number of token cache hits.",
		}),
		cacheMisses: scoped.MustCounter(metrics.MetricOpts{
			Name: "cache_misses_total",
			Help: "Total number of token cache misses.",
		}),
		jwksRefreshes: scoped.MustCounter(metrics.MetricOpts{
			Name: "jwks_refreshes_total",
			Help: "Total number of JWKS refresh operations.",
		}),
		jwksRefreshErrors: scoped.MustCounter(metrics.MetricOpts{
			Name: "jwks_refresh_errors_total",
			Help: "Total number of failed JWKS refresh operations.",
		}),
		jwksRefreshDuration: scoped.MustTimer(metrics.HistogramOpts{
			MetricOpts: metrics.MetricOpts{
				Name: "jwks_refresh_duration_seconds",
				Help: "Duration of JWKS refresh operations in seconds.",
			},
		}),
		jwksStaleRejections: scoped.MustCounter(metrics.MetricOpts{
			Name: "jwks_stale_rejections_total",
			Help: "Total number of validations affected by an over-stale JWKS cache (enforced or warned).",
		}),
		revocationCheckErrors: scoped.MustCounter(metrics.MetricOpts{
			Name: "revocation_check_errors_total",
			Help: "Total number of token revocation check failures.",
		}),
	}
}
