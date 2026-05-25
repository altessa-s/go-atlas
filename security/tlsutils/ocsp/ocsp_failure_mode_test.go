// Copyright 2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package ocsp

import (
	"context"
	"crypto/tls"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

// failingStapler is a fake OCSPStapler that always reports a failure
// from GetOCSPStaple — the precondition we need to exercise the
// Hard/Soft branch in stapleOrEnforce.
type failingStapler struct {
	err         error
	failureMode FailureMode
}

func (f *failingStapler) GetOCSPStaple(_ context.Context, _ *tls.Certificate) ([]byte, error) {
	return nil, f.err
}
func (f *failingStapler) RunRefreshAll(_ context.Context) error { return nil }
func (f *failingStapler) FailureMode() FailureMode              { return f.failureMode }

// TestStapleOrEnforce_HardModeRejectsOnFetchError is the regression
// guard for the audit finding: in Hard mode, a stapler that cannot
// produce a valid OCSP response MUST abort the handshake (caller
// receives a typed error) rather than serve the certificate unstapled.
func TestStapleOrEnforce_HardModeRejectsOnFetchError(t *testing.T) {
	t.Parallel()

	stapler := &failingStapler{
		err:         errors.New("responder unreachable"),
		failureMode: FailureModeHard,
	}
	cert := &tls.Certificate{Certificate: [][]byte{{0x01}}}

	got, err := stapleOrEnforce(t.Context(), stapler, cert)
	require.Error(t, err, "Hard mode must fail the handshake when no staple can be produced")
	require.Nil(t, got, "no certificate must be returned in Hard mode failure")
	require.Contains(t, err.Error(), "hard-fail",
		"error must mark the hard-fail path so operators can grep")
}

// TestStapleOrEnforce_SoftModeReturnsCertOnFetchError pins the
// production-safe-for-availability default: a fetch failure surrenders
// the staple but still serves the certificate.
func TestStapleOrEnforce_SoftModeReturnsCertOnFetchError(t *testing.T) {
	t.Parallel()

	stapler := &failingStapler{
		err:         errors.New("responder unreachable"),
		failureMode: FailureModeSoft,
	}
	cert := &tls.Certificate{Certificate: [][]byte{{0x01}}}

	got, err := stapleOrEnforce(t.Context(), stapler, cert)
	require.NoError(t, err, "Soft mode must continue the handshake on fetch failure")
	require.Same(t, cert, got, "Soft mode must return the ORIGINAL certificate unmodified — no staple, no clone")
}

// TestStapler_FailureMode_DefaultsToSoft confirms a freshly-constructed
// stapler without WithFailureMode reports Soft — preserves legacy
// behavior for callers that don't opt in.
func TestStapler_FailureMode_DefaultsToSoft(t *testing.T) {
	t.Parallel()

	s := NewOCSPStapler()
	require.Equal(t, FailureModeSoft, s.FailureMode(),
		"default failure mode must be Soft (availability over strict revocation)")
}

// TestWithFailureMode_AcceptsKnownModes proves the option installs the
// requested mode and ignores unknown values (fail-safe to the default).
func TestWithFailureMode_AcceptsKnownModes(t *testing.T) {
	t.Parallel()

	s := NewOCSPStapler(WithFailureMode(FailureModeHard))
	require.Equal(t, FailureModeHard, s.FailureMode())

	s2 := NewOCSPStapler(WithFailureMode("bogus"))
	require.Equal(t, FailureModeSoft, s2.FailureMode(),
		"unknown failure modes must leave the default in place — mirrors the project's safety-mode pattern")
}
