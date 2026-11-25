// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package selfjwt

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/auth/denylist"

	jwt "github.com/altessa-s/go-atlas/auth/jwt"
)

func TestOptionsMigrationBaseline(t *testing.T) {
	t.Parallel()
	got := defaultVerifierOptions()
	before := []jwt.Algorithm{jwt.Algorithm(" HS256 "), jwt.Algorithm("HS256"), jwt.Algorithm("HS256")}
	WithVerifierAllowedAlgorithms()(got)
	require.Equal(t, defaultAllowedAlgorithms, got.allowedAlgorithms)
	WithVerifierAllowedAlgorithms(before...)(got)
	before[0] = jwt.AlgRS256
	require.Equal(t, []jwt.Algorithm{" HS256 ", "HS256", "HS256"}, got.allowedAlgorithms)
}

// The pair applies shared settings once and forwards each role's own settings.
func TestPairOptionsReachBothRoles(t *testing.T) {
	t.Parallel()
	clk := &stepClock{}
	rnd := strings.NewReader("randomness")
	metrics := &Metrics{}
	revoked := denylist.New()
	m, v := New(nil, "issuer", "audience", WithPairClock(clk), WithPairRand(rnd),
		WithPairAllowedAlgorithms(jwt.AlgES256), WithPairMetrics(metrics), WithPairRevocation(revoked),
		WithPairMaxTokenLifetime(time.Hour), WithPairLeeway(0), WithPairCacheTTL(time.Minute),
		WithPairCacheMaxEntries(2), WithPairKeyLookupTimeout(time.Second),
		WithPairNegativeCacheTTL(time.Second), WithPairMaxConcurrentLookups(3))

	require.Equal(t, newMinterOptions(WithMinterClock(clk), WithRand(rnd), WithMinterAllowedAlgorithms(jwt.AlgES256),
		WithMinterMetrics(metrics), WithMaxTokenLifetime(time.Hour)), m.opts)
	require.Equal(t, newVerifierOptions(WithVerifierClock(clk), WithVerifierAllowedAlgorithms(jwt.AlgES256),
		WithVerifierMetrics(metrics), WithRevocation(revoked), WithLeeway(0), WithCacheTTL(time.Minute), WithCacheMaxEntries(2),
		WithKeyLookupTimeout(time.Second), WithNegativeCacheTTL(time.Second), WithMaxConcurrentLookups(3)), v.opts)
}
