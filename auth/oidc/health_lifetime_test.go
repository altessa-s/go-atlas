// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package oidc

import (
	"context"
	"slices"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/observability/health"
)

type legacyRegistrar struct{ checker health.Checker }

func (r *legacyRegistrar) RegisterService(_ string, c health.Checker) { r.checker = c }

func TestProviderHealthLifetime(t *testing.T) {
	t.Parallel()
	idp := newTestIdP(t)
	t.Run("scoped", func(t *testing.T) {
		t.Parallel()
		reg := health.New()
		defer reg.Close()
		first := idp.newProvider(t, WithHealthCoordinator(reg))
		second := idp.newProvider(t, WithHealthCoordinator(reg))
		first.Close()
		require.Equal(t, health.StatusServing, reg.CheckServiceHealth(t.Context(), "oidc"))
		second.Close()
		require.NotContains(t, slices.Collect(reg.ListServices()), "oidc")
	})
	t.Run("legacy", func(t *testing.T) {
		t.Parallel()
		reg := &legacyRegistrar{}
		p := idp.newProvider(t, WithHealthCoordinator(reg))
		require.Equal(t, health.StatusServing, reg.checker.CheckHealth(t.Context()))
		p.Close()
		require.Equal(t, health.StatusServiceUnknown, reg.checker.CheckHealth(t.Context()))
	})
}

type borrowedFilter struct {
	*rebuildableFilter
	closes int
}

func (f *borrowedFilter) Close(context.Context) error { f.closes++; return nil }

func TestSharedBorrowedFilterLifetime(t *testing.T) {
	t.Parallel()
	idp := newTestIdP(t)
	f := &borrowedFilter{rebuildableFilter: newRebuildableFilter()}
	a := idp.newProvider(t, WithRevocationFilter(f))
	b := idp.newProvider(t, WithRevocationStorage(NewFilterRevocationStorage(f, nil, nil)))
	a.Close()
	_, err := b.ValidateToken(t.Context(), idp.sign(t, idp.claims(nil)))
	require.NoError(t, err)
	b.Close()
	require.Zero(t, f.closes)
}
