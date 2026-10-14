// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package oidc

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/testhelpers"
)

func TestOIDCMetrics_ForIssuer(t *testing.T) {
	t.Parallel()

	collector := testhelpers.NewTestCollector()
	m := newOIDCMetrics(collector)

	first := m.forIssuer("https://a.example")
	require.Same(t, first, m.forIssuer("https://a.example"), "same issuer must reuse the bound counters")

	first.validations.Inc()
	first.errors.Inc()

	// An issuer change (e.g. rediscovery) rebinds to the new label.
	second := m.forIssuer("https://b.example")
	require.NotSame(t, first, second)
	second.validations.Inc()

	require.InDelta(t, 1.0, testhelpers.GetCounterValue(t, collector,
		"test_auth_oidc_token_validations_total", "issuer", "https://a.example"), 0)
	require.InDelta(t, 1.0, testhelpers.GetCounterValue(t, collector,
		"test_auth_oidc_validation_errors_total", "issuer", "https://a.example"), 0)
	require.InDelta(t, 1.0, testhelpers.GetCounterValue(t, collector,
		"test_auth_oidc_token_validations_total", "issuer", "https://b.example"), 0)
}

func TestOIDCMetrics_ForIssuerConcurrent(t *testing.T) {
	t.Parallel()

	collector := testhelpers.NewTestCollector()
	m := newOIDCMetrics(collector)

	const goroutines, perGoroutine = 8, 100

	var wg sync.WaitGroup
	for g := range goroutines {
		issuer := "https://a.example"
		if g%2 == 1 {
			issuer = "https://b.example"
		}
		wg.Go(func() {
			for range perGoroutine {
				m.forIssuer(issuer).validations.Inc()
			}
		})
	}
	wg.Wait()

	const perIssuer = goroutines / 2 * perGoroutine
	require.InDelta(t, perIssuer, testhelpers.GetCounterValue(t, collector,
		"test_auth_oidc_token_validations_total", "issuer", "https://a.example"), 0)
	require.InDelta(t, perIssuer, testhelpers.GetCounterValue(t, collector,
		"test_auth_oidc_token_validations_total", "issuer", "https://b.example"), 0)
}
