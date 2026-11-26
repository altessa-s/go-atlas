// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory_test

import (
	"bytes"
	"context"
	"errors"
	"iter"
	"log/slog"
	"slices"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/auth/jwt"
	"github.com/altessa-s/go-atlas/observability/health"

	coreerrs "github.com/altessa-s/go-atlas-core/errors"
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
	tasks   []schedtask.Config
	removed []string
	err     error
}

func (r *registrar) Unregister(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.removed = append(r.removed, id)
	return nil
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
	cache, err := factory.NewBuilder(src).UseName("t").UseScheduler(reg).UseHealthCoordinator(hr).
		UseRefreshSchedule("@every 5s").UseMaxStaleness(time.Hour).Build(t.Context())
	require.NoError(t, err)
	require.True(t, cache.Ready())

	got, err := cache.IsRevoked("revoked-jti")
	require.NoError(t, err)
	require.True(t, got)

	require.Len(t, reg.tasks, 1)
	require.Equal(t, factory.RefreshTaskID+"-t", reg.tasks[0].ID)
	require.Equal(t, "@every 5s", reg.tasks[0].Schedule)
	require.NoError(t, reg.tasks[0].Func(t.Context()))

	checker := hr.services[factory.DefaultHealthServiceName+".t"]
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
	cache, err := factory.NewBuilder(src).UseName("t").UseScheduler(reg).UseHealthCoordinator(hr).
		UseInsecureServeUnprimed().Build(t.Context())
	require.NoError(t, err)
	require.False(t, cache.Ready())
	require.Equal(t, health.StatusNotServing, hr.services[factory.DefaultHealthServiceName+".t"].CheckHealth(t.Context()))

	_, err = cache.IsRevoked("any")
	require.NoError(t, err)

	src.mu.Lock()
	src.err = nil
	src.mu.Unlock()
	require.NoError(t, reg.tasks[0].Func(t.Context()))
	require.Equal(t, health.StatusServing, hr.services[factory.DefaultHealthServiceName+".t"].CheckHealth(t.Context()))
}

func TestBuildStaleCacheRefuses(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		cache, err := factory.NewBuilder(&source{}).UseScheduler(&registrar{}).
			UseRefreshSchedule("@every 1m").UseMaxStaleness(time.Minute).Build(t.Context())
		require.NoError(t, err)
		time.Sleep(2 * time.Minute)
		_, err = cache.IsRevoked("x")
		require.ErrorIs(t, err, jwt.ErrRevocationUnavailable)
	})
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

// Horai replaces a task registered under an existing ID, so two mirrors on one
// scheduler must register under different IDs or the first stops refreshing.
func TestBuildTwoMirrorsRegisterDistinctTasks(t *testing.T) {
	t.Parallel()

	reg := &registrar{}
	hr := &healthRegistry{}
	first := &source{keys: []string{"a"}}
	second := &source{keys: []string{"b"}}
	_, err := factory.NewBuilder(first).UseScheduler(reg).UseHealthCoordinator(hr).Build(t.Context())
	require.NoError(t, err)
	_, err = factory.NewBuilder(second).UseScheduler(reg).UseHealthCoordinator(hr).Build(t.Context())
	require.NoError(t, err)
	_, err = factory.NewBuilder(&source{}).UseName("tenants").UseScheduler(reg).UseHealthCoordinator(hr).Build(t.Context())
	require.NoError(t, err)

	ids := map[string]bool{}
	for _, task := range reg.tasks {
		ids[task.ID] = true
	}
	require.Len(t, ids, 3, "task IDs: %v", ids)
	require.Contains(t, ids, factory.RefreshTaskID+"-tenants")
	require.Len(t, hr.services, 3)
}

// A priming error is reported by type only, in the returned error and in the
// log of the insecure-startup mode, while errors.Is still reaches the cause.
func TestBuildHidesSourceErrorText(t *testing.T) {
	t.Parallel()

	sentinel := errors.New("MARKER-secret-dsn")
	_, err := factory.NewBuilder(&source{err: sentinel}).UseScheduler(&registrar{}).Build(t.Context())
	require.ErrorIs(t, err, sentinel)
	require.NotContains(t, err.Error(), "MARKER")

	var logs bytes.Buffer
	_, err = factory.NewBuilder(&source{err: sentinel}).UseScheduler(&registrar{}).
		UseLogger(slog.New(slog.NewTextHandler(&logs, nil))).UseInsecureServeUnprimed().Build(t.Context())
	require.NoError(t, err)
	require.NotContains(t, logs.String(), "MARKER")
	require.Contains(t, logs.String(), "priming failed")
}

func TestBuildPrimeIsBounded(t *testing.T) {
	t.Parallel()

	_, err := factory.NewBuilder(hangingSource{}).UseScheduler(&registrar{}).UsePrimeTimeout(10 * time.Millisecond).Build(t.Context())
	require.ErrorIs(t, err, context.DeadlineExceeded)
}

type hangingSource struct{}

func (hangingSource) StreamValues(ctx context.Context) iter.Seq2[string, error] {
	return func(yield func(string, error) bool) {
		<-ctx.Done()
		yield("", ctx.Err())
	}
}

func TestBuildRejectsStalenessShorterThanRefresh(t *testing.T) {
	t.Parallel()

	_, err := factory.NewBuilder(&source{}).UseScheduler(&registrar{}).
		UseRefreshSchedule("@every 1m").UseMaxStaleness(30 * time.Second).Build(t.Context())
	require.ErrorIs(t, err, coreerrs.ErrInvalidArgument)

	_, err = factory.NewBuilder(&source{}).UseScheduler(&registrar{}).UseMaxStaleness(time.Second).Build(t.Context())
	require.ErrorIs(t, err, coreerrs.ErrInvalidArgument, "the default 30s schedule applies")

	_, err = factory.NewBuilder(&source{}).UseScheduler(&registrar{}).
		UseRefreshSchedule("@every 1m").UseMaxStaleness(3 * time.Minute).Build(t.Context())
	require.NoError(t, err)
}

func TestManagedCloseReleasesHealthAndTask(t *testing.T) {
	t.Parallel()

	reg := &registrar{}
	coordinator := health.New()
	other, err := factory.NewBuilder(&source{}).UseName("other").UseScheduler(reg).UseHealthCoordinator(coordinator).Build(t.Context())
	require.NoError(t, err)
	t.Cleanup(other.Close)

	managed, err := factory.NewBuilder(&source{keys: []string{"a"}}).UseName("t").UseScheduler(reg).UseHealthCoordinator(coordinator).
		Build(t.Context())
	require.NoError(t, err)
	name := factory.DefaultHealthServiceName + ".t"
	require.Contains(t, slices.Collect(coordinator.ListServices()), name)

	managed.Close()
	managed.Close()

	services := slices.Collect(coordinator.ListServices())
	require.NotContains(t, services, name)
	require.Contains(t, services, factory.DefaultHealthServiceName+".other")
	require.Equal(t, []string{factory.RefreshTaskID + "-t"}, reg.removed)

	// A refresh still queued in the scheduler does nothing after Close.
	require.NoError(t, reg.tasks[1].Func(t.Context()))
	got, err := managed.IsRevoked("a")
	require.NoError(t, err)
	require.True(t, got)
}
