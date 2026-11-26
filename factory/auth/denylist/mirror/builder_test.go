// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory_test

import (
	"context"
	"errors"
	"iter"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/auth/jwt"
	"github.com/altessa-s/go-atlas/observability/health"

	factory "github.com/altessa-s/go-atlas/factory/auth/denylist/mirror"
	corefactory "github.com/altessa-s/go-atlas/factory/core"
	schedtask "github.com/altessa-s/go-horai/task"
)

type source struct {
	mu   sync.Mutex
	keys []string
	err  error
}

func (s *source) StreamValues(context.Context) iter.Seq2[string, error] {
	s.mu.Lock()
	keys, err := s.keys, s.err
	s.mu.Unlock()
	return func(yield func(string, error) bool) {
		if err != nil {
			yield("", err)
			return
		}
		for _, k := range keys {
			if !yield(k, nil) {
				return
			}
		}
	}
}

type registrar struct {
	mu    sync.Mutex
	tasks []schedtask.Config
	err   error
}

func (r *registrar) Register(_ context.Context, cfg schedtask.Config) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tasks = append(r.tasks, cfg)
	return r.err
}

type healthRegistry struct {
	mu       sync.Mutex
	services map[string]health.Checker
}

func (h *healthRegistry) RegisterService(name string, c health.Checker) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.services == nil {
		h.services = map[string]health.Checker{}
	}
	h.services[name] = c
}

func TestBuildPrimesRegistersAndReportsHealth(t *testing.T) {
	t.Parallel()

	src := &source{keys: []string{"revoked-jti"}}
	reg := &registrar{}
	hr := &healthRegistry{}
	cache, err := factory.NewBuilder(src).UseScheduler(reg).UseHealthCoordinator(hr).
		UseRefreshSchedule("@every 5s").UseMaxStaleness(time.Hour).Build(t.Context())
	require.NoError(t, err)
	require.True(t, cache.Ready())

	got, err := cache.IsRevoked("revoked-jti")
	require.NoError(t, err)
	require.True(t, got)

	require.Len(t, reg.tasks, 1)
	require.Equal(t, factory.RefreshTaskID, reg.tasks[0].ID)
	require.Equal(t, "@every 5s", reg.tasks[0].Schedule)
	require.NoError(t, reg.tasks[0].Func(t.Context()))

	checker := hr.services[factory.DefaultHealthServiceName]
	require.NotNil(t, checker)
	require.Equal(t, health.StatusServing, checker.CheckHealth(t.Context()))
}

func TestBuildFailsWhenSourceUnreachable(t *testing.T) {
	t.Parallel()

	sentinel := errors.New("redis down")
	_, err := factory.NewBuilder(&source{err: sentinel}).UseScheduler(&registrar{}).Build(t.Context())
	require.ErrorIs(t, err, sentinel)
}

func TestBuildInsecureServeUnprimedSurvivesAndReportsNotServing(t *testing.T) {
	t.Parallel()

	src := &source{err: errors.New("redis down")}
	reg := &registrar{}
	hr := &healthRegistry{}
	cache, err := factory.NewBuilder(src).UseScheduler(reg).UseHealthCoordinator(hr).
		UseInsecureServeUnprimed().Build(t.Context())
	require.NoError(t, err)
	require.False(t, cache.Ready())
	require.Equal(t, health.StatusNotServing, hr.services[factory.DefaultHealthServiceName].CheckHealth(t.Context()))

	_, err = cache.IsRevoked("any")
	require.NoError(t, err)

	src.mu.Lock()
	src.err = nil
	src.mu.Unlock()
	require.NoError(t, reg.tasks[0].Func(t.Context()))
	require.Equal(t, health.StatusServing, hr.services[factory.DefaultHealthServiceName].CheckHealth(t.Context()))
}

func TestBuildStaleCacheRefuses(t *testing.T) {
	t.Parallel()

	src := &source{}
	cache, err := factory.NewBuilder(src).UseScheduler(&registrar{}).UseMaxStaleness(time.Nanosecond).Build(t.Context())
	require.NoError(t, err)
	time.Sleep(5 * time.Millisecond)
	_, err = cache.IsRevoked("x")
	require.ErrorIs(t, err, jwt.ErrRevocationUnavailable)
}

func TestBuildRequiresDependencies(t *testing.T) {
	t.Parallel()

	_, err := factory.NewBuilder(nil).UseScheduler(&registrar{}).Build(t.Context())
	require.ErrorIs(t, err, corefactory.ErrDependencyRequired)

	_, err = factory.NewBuilder(&source{}).Build(t.Context())
	require.ErrorIs(t, err, corefactory.ErrDependencyRequired)
}

func TestBuildPropagatesRegistrationError(t *testing.T) {
	t.Parallel()

	sentinel := errors.New("scheduler closed")
	_, err := factory.NewBuilder(&source{}).UseScheduler(&registrar{err: sentinel}).Build(t.Context())
	require.ErrorIs(t, err, sentinel)
}
