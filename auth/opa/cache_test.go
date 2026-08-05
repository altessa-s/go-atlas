// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package opa_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/auth/opa"
	"github.com/altessa-s/go-atlas/auth/opa/sources/filesystem"
	"github.com/altessa-s/go-atlas/internal/testhelpers"
)

// adminPolicy allows only input.role == "admin".
const adminPolicy = `
package test.authz
import rego.v1
allow if {
    input.role == "admin"
}
`

// denyAllPolicy allows nothing, so a reload from adminPolicy flips the decision
// for the same input — which is what makes stale cache entries observable.
const denyAllPolicy = `
package test.authz
import rego.v1
allow if {
    input.role == "nobody-has-this-role"
}
`

func newCachingManager(t *testing.T, dir string, opts ...opa.Option) *opa.Manager {
	t.Helper()

	source, err := filesystem.New(dir)
	require.NoError(t, err)

	manager, err := opa.NewManager(t.Context(), source, "data.test.authz.allow", opts...)
	require.NoError(t, err)
	t.Cleanup(func() { _ = manager.Close() })

	return manager
}

func TestDecisionCache_ServesRepeatedInput(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writePolicy(t, dir, adminPolicy)

	collector := testhelpers.NewTestCollector()
	manager := newCachingManager(t, dir,
		opa.WithDecisionCache(100, time.Minute),
		opa.WithCollector(collector),
	)

	evaluator := manager.Evaluator()
	input := map[string]any{"role": "admin"}

	first, err := evaluator.Evaluate(t.Context(), input)
	require.NoError(t, err)
	require.True(t, first.Allow)

	second, err := evaluator.Evaluate(t.Context(), input)
	require.NoError(t, err)
	require.True(t, second.Allow)

	require.Equal(t, float64(1),
		testhelpers.GetCounterValue(t, collector, "test_auth_opa_decision_cache_lookups_total", "result", "hit"))
	require.Equal(t, float64(1),
		testhelpers.GetCounterValue(t, collector, "test_auth_opa_decision_cache_lookups_total", "result", "miss"))
}

// A cached decision must still be counted: caching is an evaluation
// short-circuit, not a reason for the decision to go unobserved.
func TestDecisionCache_HitStillCountsEvaluation(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writePolicy(t, dir, adminPolicy)

	collector := testhelpers.NewTestCollector()
	manager := newCachingManager(t, dir,
		opa.WithDecisionCache(100, time.Minute),
		opa.WithCollector(collector),
	)

	evaluator := manager.Evaluator()
	input := map[string]any{"role": "admin"}

	for range 3 {
		_, err := evaluator.Evaluate(t.Context(), input)
		require.NoError(t, err)
	}

	require.Equal(t, float64(3),
		testhelpers.GetCounterValue(t, collector, "test_auth_opa_evaluations_total", "result", "allow"),
		"every decision must be counted, cached or not")
}

// The security-critical property: a policy reload must not leave the previous
// bundle's answers reachable.
func TestDecisionCache_ReloadInvalidatesDecisions(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writePolicy(t, dir, adminPolicy)

	manager := newCachingManager(t, dir, opa.WithDecisionCache(100, time.Hour))
	evaluator := manager.Evaluator()
	input := map[string]any{"role": "admin"}

	allowed, err := evaluator.Evaluate(t.Context(), input)
	require.NoError(t, err)
	require.True(t, allowed.Allow)

	writePolicy(t, dir, denyAllPolicy)
	require.NoError(t, manager.RunUpdateCycle(t.Context()))

	denied, err := evaluator.Evaluate(t.Context(), input)
	require.NoError(t, err)
	require.False(t, denied.Allow, "the pre-reload decision must not survive a policy change")
}

func TestDecisionCache_DistinctInputsDoNotCollide(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writePolicy(t, dir, adminPolicy)

	manager := newCachingManager(t, dir, opa.WithDecisionCache(100, time.Minute))
	evaluator := manager.Evaluator()

	admin, err := evaluator.Evaluate(t.Context(), map[string]any{"role": "admin"})
	require.NoError(t, err)
	require.True(t, admin.Allow)

	user, err := evaluator.Evaluate(t.Context(), map[string]any{"role": "user"})
	require.NoError(t, err)
	require.False(t, user.Allow, "a different input must not read the previous decision")
}

// Result has exported mutable fields, so a caller enriching one hit must not
// affect the next.
func TestDecisionCache_HitsAreIndependentCopies(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writePolicy(t, dir, adminPolicy)

	manager := newCachingManager(t, dir, opa.WithDecisionCache(100, time.Minute))
	evaluator := manager.Evaluator()
	input := map[string]any{"role": "admin"}

	first, err := evaluator.Evaluate(t.Context(), input)
	require.NoError(t, err)

	first.Allow = false
	first.DecisionID = "tampered"

	second, err := evaluator.Evaluate(t.Context(), input)
	require.NoError(t, err)
	require.True(t, second.Allow, "mutating one result must not corrupt the cached decision")
	require.NotEqual(t, "tampered", second.DecisionID)
}

// A DecisionID identifies one decision; replaying a cached one under its
// original ID would make distinct decisions indistinguishable in the audit log.
func TestDecisionCache_HitGetsFreshDecisionID(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writePolicy(t, dir, adminPolicy)

	manager := newCachingManager(t, dir,
		opa.WithDecisionCache(100, time.Minute),
		opa.WithDecisionLogging(),
	)
	evaluator := manager.Evaluator()
	input := map[string]any{"role": "admin"}

	first, err := evaluator.Evaluate(t.Context(), input)
	require.NoError(t, err)
	require.NotEmpty(t, first.DecisionID)

	second, err := evaluator.Evaluate(t.Context(), input)
	require.NoError(t, err)
	require.NotEmpty(t, second.DecisionID)
	require.NotEqual(t, first.DecisionID, second.DecisionID)
}

func TestDecisionCache_EntriesExpire(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writePolicy(t, dir, adminPolicy)

	collector := testhelpers.NewTestCollector()
	manager := newCachingManager(t, dir,
		opa.WithDecisionCache(100, 20*time.Millisecond),
		opa.WithCollector(collector),
	)

	evaluator := manager.Evaluator()
	input := map[string]any{"role": "admin"}

	_, err := evaluator.Evaluate(t.Context(), input)
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		_, err := evaluator.Evaluate(t.Context(), input)
		require.NoError(t, err)

		return testhelpers.GetCounterValue(t, collector,
			"test_auth_opa_decision_cache_lookups_total", "result", "miss") >= 2
	}, time.Second, 10*time.Millisecond, "entries must stop being served after the TTL")
}

// The cache keys on the JSON encoding of the input, which sounds like a new
// restriction but is not: OPA marshals the input to JSON to evaluate it, so an
// input the key cannot encode was never evaluable in the first place. Enabling
// the cache must not change that outcome, and must not report a hit for it.
func TestDecisionCache_UnmarshalableInputIsRejectedAsBefore(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writePolicy(t, dir, adminPolicy)

	collector := testhelpers.NewTestCollector()
	cached := newCachingManager(t, dir,
		opa.WithDecisionCache(100, time.Minute),
		opa.WithCollector(collector),
	)
	uncached := newCachingManager(t, dir)

	input := map[string]any{"role": "admin", "ch": make(chan int)}

	_, uncachedErr := uncached.Evaluator().Evaluate(t.Context(), input)
	require.Error(t, uncachedErr)

	_, cachedErr := cached.Evaluator().Evaluate(t.Context(), input)
	require.Error(t, cachedErr, "the cache must not turn an unevaluable input into a decision")

	require.Zero(t,
		testhelpers.GetCounterValue(t, collector, "test_auth_opa_decision_cache_lookups_total", "result", "hit"),
		"an unkeyable input must never report a hit")
}

// Caching is opt-in; without it every evaluation goes to Rego.
func TestDecisionCache_DisabledByDefault(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writePolicy(t, dir, adminPolicy)

	collector := testhelpers.NewTestCollector()
	manager := newCachingManager(t, dir, opa.WithCollector(collector))

	evaluator := manager.Evaluator()
	input := map[string]any{"role": "admin"}

	for range 2 {
		_, err := evaluator.Evaluate(t.Context(), input)
		require.NoError(t, err)
	}

	require.Zero(t,
		testhelpers.GetCounterValue(t, collector, "test_auth_opa_decision_cache_lookups_total", "result", "hit"))
	require.Zero(t,
		testhelpers.GetCounterValue(t, collector, "test_auth_opa_decision_cache_lookups_total", "result", "miss"))
}

// The size cap must be honored: with room for a single entry, alternating
// between two inputs evicts each before it can be read back.
func TestDecisionCache_HonorsSizeCap(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writePolicy(t, dir, adminPolicy)

	collector := testhelpers.NewTestCollector()
	manager := newCachingManager(t, dir,
		opa.WithDecisionCache(1, time.Minute),
		opa.WithCollector(collector),
	)

	evaluator := manager.Evaluator()
	admin := map[string]any{"role": "admin"}
	guest := map[string]any{"role": "guest"}

	for range 4 {
		_, err := evaluator.Evaluate(t.Context(), admin)
		require.NoError(t, err)
		_, err = evaluator.Evaluate(t.Context(), guest)
		require.NoError(t, err)
	}

	require.Zero(t,
		testhelpers.GetCounterValue(t, collector, "test_auth_opa_decision_cache_lookups_total", "result", "hit"),
		"a one-entry cache cannot serve two alternating inputs")
	require.Equal(t, float64(8),
		testhelpers.GetCounterValue(t, collector, "test_auth_opa_decision_cache_lookups_total", "result", "miss"))
}

// A larger cap holds both, which is what makes the previous test a statement
// about the cap rather than about caching being broken.
func TestDecisionCache_LargerCapHoldsBoth(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writePolicy(t, dir, adminPolicy)

	collector := testhelpers.NewTestCollector()
	manager := newCachingManager(t, dir,
		opa.WithDecisionCache(8, time.Minute),
		opa.WithCollector(collector),
	)

	evaluator := manager.Evaluator()
	admin := map[string]any{"role": "admin"}
	guest := map[string]any{"role": "guest"}

	for range 4 {
		_, err := evaluator.Evaluate(t.Context(), admin)
		require.NoError(t, err)
		_, err = evaluator.Evaluate(t.Context(), guest)
		require.NoError(t, err)
	}

	require.Equal(t, float64(6),
		testhelpers.GetCounterValue(t, collector, "test_auth_opa_decision_cache_lookups_total", "result", "hit"))
	require.Equal(t, float64(2),
		testhelpers.GetCounterValue(t, collector, "test_auth_opa_decision_cache_lookups_total", "result", "miss"))
}
