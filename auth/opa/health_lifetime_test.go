// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package opa_test

import (
	"context"
	"slices"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/auth/opa"
	"github.com/altessa-s/go-atlas/observability/health"
)

type registrationOnly struct{ checker health.Checker }

func (r *registrationOnly) RegisterService(_ string, c health.Checker) { r.checker = c }

func TestCloseHealthRegistration(t *testing.T) {
	t.Parallel()
	for _, replace := range []bool{false, true} {
		c := health.New()
		m, err := opa.NewManager(t.Context(), &mutableSource{bundle: adminsBundle("x")}, "data.t.allow", opa.WithHealthCoordinator(c))
		require.NoError(t, err)
		require.True(t, slices.Contains(slices.Collect(c.ListServices()), "opa"))
		if replace {
			c.RegisterService("opa", health.Func(func(context.Context) health.ServingStatus { return health.StatusServing }))
		}
		require.NoError(t, m.Close())
		require.Equal(t, replace, slices.Contains(slices.Collect(c.ListServices()), "opa"))
		c.Close()
	}
}
func TestCloseRegistrationOnly(t *testing.T) {
	t.Parallel()
	reg := &registrationOnly{}
	m, err := opa.NewManager(t.Context(), &mutableSource{bundle: adminsBundle("x")}, "data.t.allow", opa.WithHealthCoordinator(reg))
	require.NoError(t, err)
	require.Equal(t, health.StatusServing, reg.checker.CheckHealth(t.Context()))
	require.NoError(t, m.Close())
	require.Equal(t, health.StatusServiceUnknown, reg.checker.CheckHealth(t.Context()))
}

type pausedScoped struct {
	*health.Coordinator
	entered, release chan struct{}
}

func (p *pausedScoped) RegisterScopedService(name string, c health.Checker) (func(health.ServingStatus), func()) {
	notify, remove := p.Coordinator.RegisterScopedService(name, c)
	return notify, func() {
		close(p.entered)
		<-p.release
		remove()
	}
}
func TestCloseHealthReplacement(t *testing.T) {
	t.Parallel()
	p := &pausedScoped{health.New(), make(chan struct{}), make(chan struct{})}
	defer p.Coordinator.Close()
	m, err := opa.NewManager(t.Context(), &mutableSource{bundle: adminsBundle("x")}, "data.t.allow", opa.WithHealthCoordinator(p))
	require.NoError(t, err)
	done := make(chan error, 1)
	go func() { done <- m.Close() }()
	select {
	case <-p.entered:
	case <-done:
		require.FailNow(t, "scoped registration was not released")
	}
	p.RegisterService("opa", health.Func(func(context.Context) health.ServingStatus { return health.StatusServing }))
	close(p.release)
	require.NoError(t, <-done)
	require.Equal(t, health.StatusServing, p.CheckServiceHealth(t.Context(), "opa"))
}

// A decorator may expose name-based removal without an owner token.
type dynamicHealth struct {
	registrationOnly
	removed bool
}

func (*dynamicHealth) NotifyStatusChange(string, health.ServingStatus) {}
func (d *dynamicHealth) UnregisterService(string)                      { d.removed = true; d.checker = nil }

func TestClosePreservesLegacyReplacement(t *testing.T) {
	t.Parallel()
	reg := &dynamicHealth{}
	m, err := opa.NewManager(t.Context(), &mutableSource{bundle: adminsBundle("x")}, "data.t.allow", opa.WithHealthCoordinator(reg))
	require.NoError(t, err)
	old := reg.checker
	reg.RegisterService("opa", health.Func(func(context.Context) health.ServingStatus { return health.StatusServing }))
	require.NoError(t, m.Close())
	require.False(t, reg.removed)
	require.Equal(t, health.StatusServiceUnknown, old.CheckHealth(t.Context()))
	require.Equal(t, health.StatusServing, reg.checker.CheckHealth(t.Context()))
}
