// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"context"
	"crypto/tls"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-tlsutils"

	tlsconfig "github.com/altessa-s/go-atlas/config/tls"
	schedtask "github.com/altessa-s/go-horai/task"
	tlsocsp "github.com/altessa-s/go-tlsutils/ocsp"
)

// TestEnsureOcspStaplerFromConfig_DisabledSkipsBuild guards the
// availability fallback: when OCSP is enabled=false (or absent), the
// builder must NOT construct a stapler. A stapler with FailureMode=hard
// silently created on disabled OCSP would convert a benign "OCSP
// section in YAML" into "every new TLS handshake fails if the
// responder is down" — the exact regression the YAML wiring is
// supposed to avoid.
func TestEnsureOcspStaplerFromConfig_DisabledSkipsBuild(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		cfg  *tlsconfig.Provider
	}{
		{name: "ocsp_nil", cfg: &tlsconfig.Provider{}},
		{name: "ocsp_disabled", cfg: &tlsconfig.Provider{OCSP: &tlsconfig.ProviderOCSP{Enabled: false, FailureMode: "hard"}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			b := New(tc.cfg)
			b.ensureOcspStaplerFromConfig()
			require.Nil(t, b.ocspStapler, "disabled OCSP must not produce a stapler")
		})
	}
}

// TestEnsureOcspStaplerFromConfig_EnabledBuildsStapler is the happy
// path: an operator who flips ocsp.enabled to true gets a real stapler
// auto-wired with the YAML-supplied failure mode and cache size. The
// assertion targets FailureMode because that's the only operator-
// visible knob that round-trips through the OCSPStapler interface
// (the rest are internal to the stapler).
func TestEnsureOcspStaplerFromConfig_EnabledBuildsStapler(t *testing.T) {
	t.Parallel()

	cfg := &tlsconfig.Provider{
		OCSP: &tlsconfig.ProviderOCSP{
			Enabled:           true,
			FailureMode:       "hard",
			EnableCompression: true,
			MaxCacheEntries:   2048,
		},
	}
	b := New(cfg)
	b.ensureOcspStaplerFromConfig()
	require.NotNil(t, b.ocspStapler, "enabled OCSP must produce a stapler")

	// The constructed stapler exposes its FailureMode so the operator
	// can confirm the YAML value reached the runtime — otherwise this
	// test would just check "something non-nil was built", which would
	// miss a typo-handling regression in the conversion glue.
	stapler, ok := b.ocspStapler.(*tlsocsp.Stapler)
	require.True(t, ok, "auto-built stapler must be *tlsocsp.Stapler")
	require.Equal(t, tlsocsp.FailureModeHard, stapler.FailureMode(),
		"YAML failureMode=hard must round-trip into the runtime Stapler")
}

// TestEnsureOcspStaplerFromConfig_InjectionWinsOverYAML enforces the
// "programmatic always wins" precedence. An operator staging custom
// retry / HTTP-client plumbing via UseOcspStapler must not have that
// silently replaced just because the YAML file also has an ocsp
// block — that would be a hidden config-overrides-Go regression.
func TestEnsureOcspStaplerFromConfig_InjectionWinsOverYAML(t *testing.T) {
	t.Parallel()

	injected := &injectedStapler{}
	cfg := &tlsconfig.Provider{
		OCSP: &tlsconfig.ProviderOCSP{Enabled: true, FailureMode: "hard"},
	}
	b := New(cfg).UseOcspStapler(injected)
	b.ensureOcspStaplerFromConfig()
	require.Same(t, tlsutils.OCSPStapler(injected), b.ocspStapler,
		"programmatic stapler must survive an enabled YAML OCSP block")
}

// injectedStapler is the bare minimum to satisfy tlsutils.OCSPStapler
// for the injection-precedence test. It panics on call so any code
// that actually tries to staple via the test stapler shows up loudly.
type injectedStapler struct{}

func (*injectedStapler) GetOCSPStaple(_ context.Context, _ *tls.Certificate) ([]byte, error) {
	panic("injectedStapler should not be called")
}
func (*injectedStapler) RunRefreshAll(_ context.Context) error {
	panic("injectedStapler should not be called")
}

type recordingRegistrar struct {
	cfg    schedtask.Config
	err    error
	called bool
}

func (r *recordingRegistrar) Register(_ context.Context, cfg schedtask.Config) error {
	r.called = true
	r.cfg = cfg
	return r.err
}

func TestEnsureOcspStaplerRegistersTrimmedSchedule(t *testing.T) {
	t.Parallel()
	reg := &recordingRegistrar{}
	b := New(&tlsconfig.Provider{OCSP: &tlsconfig.ProviderOCSP{
		Enabled: true, RefreshSchedule: "  @every 1h  ",
	}}).UseScheduler(reg)
	b.ensureOcspStaplerFromConfig()
	require.True(t, reg.called)
	require.Equal(t, "@every 1h", reg.cfg.Schedule)
	require.Equal(t, "ocsp-stapler-refresh-all", reg.cfg.ID)
	require.Equal(t, "Refresh all cached OCSP staples", reg.cfg.Description)
	require.Equal(t, schedtask.PriorityLow, reg.cfg.Priority)
	require.NotNil(t, reg.cfg.Func)
	err := b.ocspStapler.RunRefreshAll(t.Context())
	require.ErrorIs(t, err, tlsocsp.ErrSchedulerManaged)
}

func TestEnsureOcspStaplerSkipsBlankSchedule(t *testing.T) {
	t.Parallel()
	for _, schedule := range []string{"", "   "} {
		reg := &recordingRegistrar{}
		b := New(&tlsconfig.Provider{OCSP: &tlsconfig.ProviderOCSP{
			Enabled: true, RefreshSchedule: schedule,
		}}).UseScheduler(reg)
		b.ensureOcspStaplerFromConfig()
		require.False(t, reg.called)
		require.NoError(t, b.ocspStapler.RunRefreshAll(t.Context()))
	}
}

func TestEnsureOcspStaplerSkipsNilScheduler(t *testing.T) {
	t.Parallel()
	b := New(&tlsconfig.Provider{OCSP: &tlsconfig.ProviderOCSP{
		Enabled: true, RefreshSchedule: "@every 1h",
	}})
	b.ensureOcspStaplerFromConfig()
	require.NoError(t, b.ocspStapler.RunRefreshAll(t.Context()))
}

func TestEnsureOcspStaplerFailedRegisterStaysCallable(t *testing.T) {
	t.Parallel()
	reg := &recordingRegistrar{err: errors.New("register rejected")}
	b := New(&tlsconfig.Provider{OCSP: &tlsconfig.ProviderOCSP{
		Enabled: true, RefreshSchedule: "@every 1h",
	}}).UseScheduler(reg)
	providers, err := b.Build()
	require.NoError(t, err)
	require.NotNil(t, providers)
	require.True(t, reg.called)
	require.NoError(t, b.ocspStapler.RunRefreshAll(t.Context()))
}
