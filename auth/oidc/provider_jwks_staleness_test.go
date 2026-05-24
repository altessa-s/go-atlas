// Copyright 2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package oidc

import (
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// newStalenessTestProvider builds a minimal Provider wired for direct
// checkJWKSStaleness calls. No JWKS, no introspection, no revocation —
// only the staleness plumbing is exercised so the test does not depend
// on an IdP fixture.
func newStalenessTestProvider(t *testing.T, maxStaleness time.Duration, mode JWKSFailureMode) *Provider {
	t.Helper()
	return &Provider{
		opts: &options{
			jwksMaxStaleness: maxStaleness,
			jwksFailureMode:  mode,
		},
		logger:  slog.New(slog.DiscardHandler),
		metrics: newOIDCMetrics(nil),
	}
}

// setStaleness pretends the last successful JWKS refresh happened `age`
// ago, in a way that does not race with concurrent staleness reads.
func setStaleness(t *testing.T, p *Provider, age time.Duration) {
	t.Helper()
	p.lastJWKSRefreshUnixNanos.Store(time.Now().Add(-age).UnixNano())
}

func TestProvider_checkJWKSStaleness_DisabledByDefault(t *testing.T) {
	t.Parallel()

	// Default options ⇒ jwksMaxStaleness == 0 ⇒ the check is a no-op
	// regardless of how stale the cache is. Preserves the legacy
	// (opt-in) behavior so callers without a refresh path are not
	// surprised by sudden rejections.
	p := newStalenessTestProvider(t, 0, JWKSFailureModeEnforce)
	setStaleness(t, p, 24*time.Hour)

	require.NoError(t, p.checkJWKSStaleness(t.Context()),
		"max staleness == 0 must short-circuit before mode evaluation")
}

func TestProvider_checkJWKSStaleness_EnforceRejectsWhenStale(t *testing.T) {
	t.Parallel()

	p := newStalenessTestProvider(t, 5*time.Minute, JWKSFailureModeEnforce)
	setStaleness(t, p, 10*time.Minute)

	err := p.checkJWKSStaleness(t.Context())
	require.ErrorIs(t, err, ErrJWKSStale,
		"enforce mode must reject validation when the staleness budget is exceeded")
}

func TestProvider_checkJWKSStaleness_EnforceAllowsWhenFresh(t *testing.T) {
	t.Parallel()

	p := newStalenessTestProvider(t, 5*time.Minute, JWKSFailureModeEnforce)
	setStaleness(t, p, 1*time.Minute)

	require.NoError(t, p.checkJWKSStaleness(t.Context()),
		"enforce mode must permit validation while the cache is within budget")
}

func TestProvider_checkJWKSStaleness_WarnAllowsButLogs(t *testing.T) {
	t.Parallel()

	// Warn mode trades safety for availability: validation proceeds even
	// when the cache is stale, but checkJWKSStaleness still touches the
	// metrics counter so operators can dashboard the silent risk.
	p := newStalenessTestProvider(t, 5*time.Minute, JWKSFailureModeWarn)
	setStaleness(t, p, 10*time.Minute)

	require.NoError(t, p.checkJWKSStaleness(t.Context()),
		"warn mode must allow validation through even when stale")
}

func TestProvider_checkJWKSStaleness_DisabledIsNoop(t *testing.T) {
	t.Parallel()

	// Disabled mode is the operator-side break-glass: when staleness is
	// configured (>0) but mode is Disabled, the check is a no-op so a
	// runtime toggle (env, config reload) can bypass it without changing
	// the duration.
	p := newStalenessTestProvider(t, 5*time.Minute, JWKSFailureModeDisabled)
	setStaleness(t, p, 10*time.Minute)

	require.NoError(t, p.checkJWKSStaleness(t.Context()),
		"disabled mode must skip the check entirely")
}

func TestProvider_checkJWKSStaleness_UnknownModeFallsBackToEnforce(t *testing.T) {
	t.Parallel()

	// An empty or unrecognized mode must default to the safe (Enforce)
	// behavior — never silently bypass the check. WithJWKSFailureMode
	// rejects unknown values at construction, but a hand-built options
	// struct or an unmigrated config could still set one; the helper
	// must stay fail-safe.
	p := newStalenessTestProvider(t, 5*time.Minute, JWKSFailureMode(""))
	setStaleness(t, p, 10*time.Minute)

	err := p.checkJWKSStaleness(t.Context())
	require.ErrorIs(t, err, ErrJWKSStale,
		"unknown / empty failure mode must default to enforce, not bypass the check")
}

func TestProvider_markJWKSRefreshed_ClearsStaleness(t *testing.T) {
	t.Parallel()

	// Simulate a long outage followed by a successful refresh:
	// markJWKSRefreshed must reset the staleness clock so subsequent
	// validations are accepted under the configured budget.
	p := newStalenessTestProvider(t, 5*time.Minute, JWKSFailureModeEnforce)
	setStaleness(t, p, 1*time.Hour)
	require.ErrorIs(t, p.checkJWKSStaleness(t.Context()), ErrJWKSStale,
		"precondition: stale cache must currently fail")

	p.markJWKSRefreshed()
	require.NoError(t, p.checkJWKSStaleness(t.Context()),
		"a successful refresh must reset the staleness anchor and unblock validation")
}

func TestWithJWKSFailureMode_RejectsUnknownValues(t *testing.T) {
	t.Parallel()

	// Verifies the public option follows the project's convention of
	// leaving the default in place when given an unrecognized value
	// (mirrors WithRevocationItemType / SignatureMode handling).
	o := &options{jwksFailureMode: DefaultJWKSFailureMode}
	WithJWKSFailureMode(JWKSFailureMode("bogus"))(o)
	require.Equal(t, DefaultJWKSFailureMode, o.jwksFailureMode,
		"WithJWKSFailureMode must silently keep the default for unknown modes")

	WithJWKSFailureMode(JWKSFailureModeWarn)(o)
	require.Equal(t, JWKSFailureModeWarn, o.jwksFailureMode,
		"WithJWKSFailureMode must accept recognized modes")
}
