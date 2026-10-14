// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package oidc

import (
	"sync/atomic"

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

	// issuerBound caches the issuer-labeled validation counters so the hot
	// path does not rebind them on every validation; see forIssuer.
	issuerBound atomic.Pointer[issuerCounters]
}

// issuerCounters holds the validation counters bound to one issuer label.
type issuerCounters struct {
	issuer      string
	validations metrics.Counter
	errors      metrics.Counter
}

// forIssuer returns the validation counters bound to issuer, binding and
// caching them on first use or when the issuer changes. Concurrent callers may
// bind the same label set twice; both handles address the same series.
func (m *oidcMetrics) forIssuer(issuer string) *issuerCounters {
	if c := m.issuerBound.Load(); c != nil && c.issuer == issuer {
		return c
	}

	labels := metrics.Labels{"issuer": issuer}
	c := &issuerCounters{
		issuer:      issuer,
		validations: m.tokenValidations.WithLabels(labels),
		errors:      m.validationErrors.WithLabels(labels),
	}
	m.issuerBound.Store(c)

	return c
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
