// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package uniq_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/uniq"
	"github.com/altessa-s/go-atlas/data/uniq/providers"
	"github.com/altessa-s/go-atlas/observability/health"
)

// minimalProvider implements providers.Provider but NOT
// providers.Prober — used to verify the type-assertion fallback path
// in (*Uniq).CheckHealth treats third-party providers as Serving.
type minimalProvider struct{}

func (minimalProvider) Add(_ context.Context, _ string) error { return nil }
func (minimalProvider) AddWithValue(_ context.Context, _ string, _ []byte) error {
	return nil
}
func (minimalProvider) TryAdd(_ context.Context, _ string, _ time.Duration) (bool, error) {
	return true, nil
}
func (minimalProvider) TryAddWithValue(_ context.Context, _ string, _ []byte, _ time.Duration) (bool, error) {
	return true, nil
}
func (minimalProvider) Exist(_ context.Context, _ string) (bool, error)      { return false, nil }
func (minimalProvider) GetValue(_ context.Context, _ string) ([]byte, error) { return nil, nil }
func (minimalProvider) Remove(_ context.Context, _ string) error             { return nil }
func (minimalProvider) Clear(_ context.Context) error                        { return nil }

// fakeProvider implements both Provider and Prober so we can drive
// CheckHealth into both Serving and NotServing branches.
type fakeProvider struct {
	minimalProvider
	probeErr error
}

func (f *fakeProvider) Probe(_ context.Context) error { return f.probeErr }

func TestCheckHealth_NoopProvider(t *testing.T) {
	t.Parallel()

	u := uniq.NewWithNoop()
	require.Equal(t, health.StatusServing, u.CheckHealth(t.Context()))
}

func TestCheckHealth_NilProvider(t *testing.T) {
	t.Parallel()

	u := uniq.New(nil)
	require.Equal(t, health.StatusNotServing, u.CheckHealth(t.Context()))
}

func TestCheckHealth_ProviderWithoutProber(t *testing.T) {
	t.Parallel()

	u := uniq.New(minimalProvider{})
	require.Equal(t, health.StatusServing, u.CheckHealth(t.Context()))
}

func TestCheckHealth_ProviderError(t *testing.T) {
	t.Parallel()

	u := uniq.New(&fakeProvider{probeErr: errors.New("upstream down")})
	require.Equal(t, health.StatusNotServing, u.CheckHealth(t.Context()))
}

func TestCheckHealth_ProviderOK(t *testing.T) {
	t.Parallel()

	u := uniq.New(&fakeProvider{})
	require.Equal(t, health.StatusServing, u.CheckHealth(t.Context()))
}

func TestNew_RegistersWithHealthCoordinator(t *testing.T) {
	t.Parallel()

	coord := health.New()
	t.Cleanup(coord.Close)

	uniq.NewWithNoop(uniq.WithHealthCoordinator(coord))
	require.Equal(t, health.StatusServing, coord.CheckServiceHealth(t.Context(), "uniq"))
}

func TestNew_RegistersWithCustomServiceName(t *testing.T) {
	t.Parallel()

	coord := health.New()
	t.Cleanup(coord.Close)

	uniq.NewWithNoop(
		uniq.WithHealthCoordinator(coord),
		uniq.WithHealthServiceName("uniq-tokens"),
	)
	require.Equal(t, health.StatusServing, coord.CheckServiceHealth(t.Context(), "uniq-tokens"))
	require.Equal(t, health.StatusServiceUnknown, coord.CheckServiceHealth(t.Context(), "uniq"))
}

// Compile-time guard: providers.Provider is satisfied by minimalProvider.
var _ providers.Provider = minimalProvider{}
