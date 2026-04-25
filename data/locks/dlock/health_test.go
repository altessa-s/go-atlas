// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package dlock_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/locks/dlock"
	"github.com/altessa-s/go-atlas/data/locks/dlock/providers"
	"github.com/altessa-s/go-atlas/observability/health"
)

// minimalProvider implements providers.Provider but NOT
// providers.Prober — used to verify the type-assertion fallback path
// in (*DLock).CheckHealth treats third-party providers as Serving.
type minimalProvider struct{}

func (minimalProvider) Lock(_ context.Context, _ string) (providers.Lock, error) {
	return nil, errors.New("not implemented")
}

func (minimalProvider) GetLockInfo(_ context.Context, _ string) (*providers.LockInfo, error) {
	return nil, errors.New("not implemented")
}

func (minimalProvider) Close(_ context.Context) error { return nil }

// fakeProvider implements both Provider and Prober so we can drive
// CheckHealth into both Serving and NotServing branches.
type fakeProvider struct {
	minimalProvider
	probeErr error
}

func (f *fakeProvider) Probe(_ context.Context) error { return f.probeErr }

func TestCheckHealth_NoopProvider(t *testing.T) {
	t.Parallel()

	dl := dlock.NewWithNoop()
	require.Equal(t, health.StatusServing, dl.CheckHealth(t.Context()))
}

func TestCheckHealth_NoopAfterClose(t *testing.T) {
	t.Parallel()

	dl := dlock.NewWithNoop()
	require.NoError(t, dl.Close(t.Context()))
	require.Equal(t, health.StatusNotServing, dl.CheckHealth(t.Context()))
}

func TestCheckHealth_NilProvider(t *testing.T) {
	t.Parallel()

	dl := dlock.New(nil)
	require.Equal(t, health.StatusNotServing, dl.CheckHealth(t.Context()))
}

func TestCheckHealth_ProviderWithoutProber(t *testing.T) {
	t.Parallel()

	dl := dlock.New(minimalProvider{})
	require.Equal(t, health.StatusServing, dl.CheckHealth(t.Context()))
}

func TestCheckHealth_ProviderError(t *testing.T) {
	t.Parallel()

	dl := dlock.New(&fakeProvider{probeErr: errors.New("upstream down")})
	require.Equal(t, health.StatusNotServing, dl.CheckHealth(t.Context()))
}

func TestCheckHealth_ProviderOK(t *testing.T) {
	t.Parallel()

	dl := dlock.New(&fakeProvider{})
	require.Equal(t, health.StatusServing, dl.CheckHealth(t.Context()))
}

func TestNew_RegistersWithHealthCoordinator(t *testing.T) {
	t.Parallel()

	coord := health.New()
	t.Cleanup(coord.Close)

	dlock.NewWithNoop(dlock.WithHealthCoordinator(coord))
	require.Equal(t, health.StatusServing, coord.CheckServiceHealth(t.Context(), "dlock"))
}

func TestNew_RegistersWithCustomServiceName(t *testing.T) {
	t.Parallel()

	coord := health.New()
	t.Cleanup(coord.Close)

	dlock.NewWithNoop(
		dlock.WithHealthCoordinator(coord),
		dlock.WithHealthServiceName("dlock-payments"),
	)
	require.Equal(t, health.StatusServing, coord.CheckServiceHealth(t.Context(), "dlock-payments"))
	require.Equal(t, health.StatusServiceUnknown, coord.CheckServiceHealth(t.Context(), "dlock"))
}

func TestNew_NoHealthCoordinator_DoesNotPanic(t *testing.T) {
	t.Parallel()

	// Sanity: omitting the coordinator must not crash, and the DLock
	// must still answer CheckHealth on its own.
	dl := dlock.NewWithNoop()
	require.Equal(t, health.StatusServing, dl.CheckHealth(t.Context()))
}
