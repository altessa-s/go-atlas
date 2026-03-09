// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package auth

import (
	"github.com/altessa-s/go-atlas/observability/metrics"
)

// vaultAuthMetrics holds all Prometheus metrics for the Vault authenticator.
// When no [metrics.Collector] is provided, [metrics.Noop] is used and
// all methods become zero-cost no-ops.
type vaultAuthMetrics struct {
	authAttempts       metrics.Counter
	authErrors         metrics.Counter
	tokenRenewals      metrics.Counter
	tokenRenewalErrors metrics.Counter
}

func newVaultAuthMetrics(c metrics.Collector) *vaultAuthMetrics {
	if c == nil {
		c = metrics.Noop()
	}

	scoped := c.WithSubsystem("vault_auth")

	return &vaultAuthMetrics{
		authAttempts: scoped.MustCounter(metrics.MetricOpts{
			Name: "auth_attempts_total",
			Help: "Total number of authentication attempts.",
		}),
		authErrors: scoped.MustCounter(metrics.MetricOpts{
			Name:       "auth_errors_total",
			Help:       "Total number of authentication errors.",
			LabelNames: []string{"type"},
		}),
		tokenRenewals: scoped.MustCounter(metrics.MetricOpts{
			Name: "token_renewals_total",
			Help: "Total number of successful token renewals.",
		}),
		tokenRenewalErrors: scoped.MustCounter(metrics.MetricOpts{
			Name: "token_renewal_errors_total",
			Help: "Total number of token renewal errors.",
		}),
	}
}
