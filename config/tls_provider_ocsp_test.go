// Copyright 2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package config_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/config"
)

// TestTlsProviderOCSP_Validate_AcceptsDefaults confirms the constants
// from DefaultTlsProviderOCSP pass their own validator — otherwise the
// loader's default-tag round-trip would silently produce a config that
// the Validate() method rejects on the first Load().
func TestTlsProviderOCSP_Validate_AcceptsDefaults(t *testing.T) {
	t.Parallel()

	cfg := config.DefaultTlsProviderOCSP()
	require.NoError(t, cfg.Validate(),
		"the constants returned by DefaultTlsProviderOCSP must validate — a divergence here means the default-tag round-trip would be rejected at Load")
}

// TestTlsProviderOCSP_Validate_RejectsUnknownFailureMode is the
// typo-guard. The TLSSkipVerifyMode and JWKSFailureMode safety
// patterns reject empty/unknown values; OCSP failureMode must follow
// the same rule because the runtime stapler defaults a typo to Soft
// — which is technically safe but hides operator intent.
func TestTlsProviderOCSP_Validate_RejectsUnknownFailureMode(t *testing.T) {
	t.Parallel()

	cfg := config.DefaultTlsProviderOCSP()
	cfg.FailureMode = "strict"
	err := cfg.Validate()
	require.Error(t, err, "an unknown failureMode must be rejected at config-load time")
	require.Contains(t, err.Error(), "FailureMode", "error must name the offending field")
}

// TestTlsProviderOCSP_Validate_AcceptsBothModes confirms the
// validator does not over-rotate — both documented values must pass.
func TestTlsProviderOCSP_Validate_AcceptsBothModes(t *testing.T) {
	t.Parallel()

	for _, mode := range []string{"soft", "hard"} {
		t.Run(mode, func(t *testing.T) {
			t.Parallel()
			cfg := config.DefaultTlsProviderOCSP()
			cfg.FailureMode = mode
			require.NoError(t, cfg.Validate(), "documented failureMode %q must validate", mode)
		})
	}
}

// TestTlsProviderOCSP_Validate_RejectsNegativeMaxCacheEntries pins
// the contract: zero is "unbounded cache" (legacy opt-out) but
// negative values would never make sense for a cap.
func TestTlsProviderOCSP_Validate_RejectsNegativeMaxCacheEntries(t *testing.T) {
	t.Parallel()

	cfg := config.DefaultTlsProviderOCSP()
	cfg.MaxCacheEntries = -1
	require.Error(t, cfg.Validate(), "negative maxCacheEntries must be rejected")
}

// TestTlsProviderOCSP_Validate_AcceptsZeroHTTPTimeout matches the
// rest of the config package's DurationOrZero rule — zero means
// "no timeout" rather than "invalid".
func TestTlsProviderOCSP_Validate_AcceptsZeroHTTPTimeout(t *testing.T) {
	t.Parallel()

	cfg := config.DefaultTlsProviderOCSP()
	cfg.HTTPTimeout = 0
	require.NoError(t, cfg.Validate(), "zero httpTimeout must be accepted as no-timeout")

	cfg.HTTPTimeout = 5 * time.Second
	require.NoError(t, cfg.Validate(), "positive httpTimeout must validate")
}
