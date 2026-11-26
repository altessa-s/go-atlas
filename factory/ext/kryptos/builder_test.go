// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"context"
	"errors"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-horai/task"
	"github.com/altessa-s/go-kryptos/manager"

	"github.com/altessa-s/go-atlas-core/types/redacted"
	"github.com/altessa-s/go-atlas/observability/health"

	secretsconfig "github.com/altessa-s/go-atlas/config/secrets"
	kryptos "github.com/altessa-s/go-kryptos"
	memoryprovider "github.com/altessa-s/go-kryptos/providers/memory"
	loadersecrets "github.com/altessa-s/go-nomos/loader/secrets"
)

func TestManagerOptions_CacheOnlyWhenConfigured(t *testing.T) {
	t.Parallel()

	base := managerOptions[any](New[any](&secretsconfig.Config{}))
	sized := managerOptions[string](New[string](&secretsconfig.Config{Cache: secretsconfig.Cache{MaxSize: 100}}))
	sharded := managerOptions[string](New[string](&secretsconfig.Config{Cache: secretsconfig.Cache{MaxSize: 100, ShardCount: 4}}))

	require.Len(t, sized, len(base)+1, "a configured size must add exactly one option")
	require.Len(t, sharded, len(base)+2, "a configured size and shard count must add two options")
}

func TestBuild_RequiresConfig(t *testing.T) {
	t.Parallel()

	_, err := New[any](nil).Build(t.Context())
	require.Error(t, err)
}

func TestBuild_RejectsUnsupportedProvider(t *testing.T) {
	t.Parallel()

	_, err := New[any](&secretsconfig.Config{Provider: "nope"}).Build(t.Context())
	require.Error(t, err)
}

// The memory provider needs no external dependency, so it is the one path that
// exercises config -> provider -> manager end to end.
func TestBuild_MemoryProvider(t *testing.T) {
	t.Parallel()

	cfg := secretsconfig.Default()
	cfg.Cache = secretsconfig.Cache{MaxSize: 64, ShardCount: 4}
	mgr, err := New[any](&cfg).Build(t.Context())

	require.NoError(t, err)
	require.NotNil(t, mgr)
	require.NoError(t, mgr.Close(t.Context()))
}

// memoryConfig returns a memory provider configuration serving values.
func memoryConfig(values map[string]redacted.RedactedString) *secretsconfig.Config {
	cfg := secretsconfig.Default()
	cfg.Provider = secretsconfig.ProviderMemory
	cfg.Memory = &secretsconfig.Memory{Values: values}
	return &cfg
}

// A memory provider serves its configured values, keyed the way a
// $__secret{namespace:key} placeholder looks them up.
func TestBuild_MemoryProviderServesConfiguredValues(t *testing.T) {
	t.Parallel()

	cfg := memoryConfig(map[string]redacted.RedactedString{"app:db_password": "s3cret"})

	anyManager, err := New[any](cfg).Build(t.Context())
	require.NoError(t, err)
	t.Cleanup(func() { _ = anyManager.Close(context.WithoutCancel(t.Context())) })
	anyValue, err := anyManager.Get(t.Context(), "app:db_password")
	require.NoError(t, err)
	t.Cleanup(anyValue.Clear)
	require.Equal(t, any("s3cret"), anyValue.Value)

	stringManager, err := New[string](cfg).Build(t.Context())
	require.NoError(t, err)
	t.Cleanup(func() { _ = stringManager.Close(context.WithoutCancel(t.Context())) })
	value, err := stringManager.Get(t.Context(), "app:db_password")
	require.NoError(t, err)
	t.Cleanup(value.Clear)
	require.Equal(t, "s3cret", value.Value)

	_, err = stringManager.Get(t.Context(), "app:missing")
	require.ErrorIs(t, err, kryptos.ErrNotFound)
}

func TestBuild_MemoryProviderRejectsInvalidKey(t *testing.T) {
	t.Parallel()

	_, err := New[string](memoryConfig(map[string]redacted.RedactedString{"app/db": "x"})).Build(t.Context())
	require.ErrorIs(t, err, kryptos.ErrInvalidKey)
}

// A string-valued builder yields the manager the config loader expands placeholders with:
// a known placeholder expands, an unknown one fails (fail-closed).
func TestBuild_ExpandsLoaderPlaceholders(t *testing.T) {
	t.Parallel()

	mgr, err := New[string](memoryConfig(map[string]redacted.RedactedString{"app:db_password": "s3cret"})).Build(t.Context())
	require.NoError(t, err)
	t.Cleanup(func() { _ = mgr.Close(context.WithoutCancel(t.Context())) })

	got, err := loadersecrets.ExpandString(t.Context(), "password=$__secret{app:db_password}", manager.NewStringResolver(mgr))
	require.NoError(t, err)
	require.Equal(t, "password=s3cret", got)

	_, err = loadersecrets.ExpandString(t.Context(), "$__secret{app:missing}", manager.NewStringResolver(mgr))
	require.ErrorIs(t, err, kryptos.ErrNotFound)
}

// observedProvider is a provider whose Close is counted.
type observedProvider struct {
	kryptos.Provider[string]
	closes atomic.Int32
}

func (p *observedProvider) Close(ctx context.Context) error {
	p.closes.Add(1)
	return p.Provider.Close(ctx)
}

// recordingRegistrar records registered tasks, failing them with err.
type recordingRegistrar struct {
	err     error
	tasks   []task.Config
	removed []string
}

func (r *recordingRegistrar) Register(_ context.Context, cfg task.Config) error {
	r.tasks = append(r.tasks, cfg)
	return r.err
}

func newObservedProvider(t *testing.T) *observedProvider {
	t.Helper()
	mem, err := memoryprovider.New[string](nil)
	require.NoError(t, err)
	return &observedProvider{Provider: mem}
}

// The builder owns the provider until the Manager is built: when building the
// Manager fails (here the scheduler rejects the update task), the provider is
// closed; once the Manager is built, only Manager.Close closes it.
func TestNewManager_ClosesProviderOnError(t *testing.T) {
	t.Parallel()

	cfg := secretsconfig.Default()
	cfg.Provider = secretsconfig.ProviderVault

	failed := newObservedProvider(t)
	_, err := newManager[string](t.Context(), New[string](&cfg).UseScheduler(&recordingRegistrar{err: errors.New("rejected")}), failed)
	require.Error(t, err)
	require.Eventually(t, func() bool { return failed.closes.Load() == 1 }, timeout, tick)

	built := newObservedProvider(t)
	mgr, err := newManager[string](t.Context(), New[string](&cfg), built)
	require.NoError(t, err)
	require.Zero(t, built.closes.Load())
	require.NoError(t, mgr.Close(t.Context()))
	require.Equal(t, int32(1), built.closes.Load())
}

// A remote provider's update cycle is registered with the configured schedule;
// the memory provider is never reconciled, so it registers nothing.
func TestNewManager_RegistersUpdateTask(t *testing.T) {
	t.Parallel()

	cfg := secretsconfig.Default()
	cfg.Provider = secretsconfig.ProviderVault
	reg := &recordingRegistrar{}
	mgr, err := newManager[string](t.Context(), New[string](&cfg).UseScheduler(reg), newObservedProvider(t))
	require.NoError(t, err)
	t.Cleanup(func() { _ = mgr.Close(context.WithoutCancel(t.Context())) })

	require.Len(t, reg.tasks, 1)
	require.Equal(t, UpdateTaskID, reg.tasks[0].ID)
	require.Equal(t, cfg.UpdateSchedule, reg.tasks[0].Schedule)
	require.Equal(t, cfg.RunOnStart, reg.tasks[0].RunOnStart)
	require.NoError(t, reg.tasks[0].Func(t.Context()))

	memory := secretsconfig.Default()
	memReg := &recordingRegistrar{}
	memMgr, err := New[any](&memory).UseScheduler(memReg).Build(t.Context())
	require.NoError(t, err)
	t.Cleanup(func() { _ = memMgr.Close(context.WithoutCancel(t.Context())) })
	require.Empty(t, memReg.tasks)
}

const (
	timeout = 5 * time.Second
	tick    = 10 * time.Millisecond
)

func (r *recordingRegistrar) Unregister(_ context.Context, id string) error {
	r.removed = append(r.removed, id)
	return nil
}

func TestManagerCloseRemovesHealthCheckAndUpdateTask(t *testing.T) {
	t.Parallel()
	cfg := secretsconfig.Default()
	cfg.Provider = secretsconfig.ProviderVault
	coord := health.New()
	defer coord.Close()
	scheduler := &recordingRegistrar{}
	b := New[string](&cfg).UseScheduler(scheduler).UseHealthCoordinator(coord)
	mgr, err := newManager(t.Context(), b, newObservedProvider(t))
	require.NoError(t, err)
	require.Contains(t, slices.Collect(coord.ListServices()), HealthServiceName)
	require.Len(t, scheduler.tasks, 1)

	require.NoError(t, mgr.Close(t.Context()))
	require.NoError(t, mgr.Close(t.Context()))
	require.Empty(t, slices.Collect(coord.ListServices()))
	require.Equal(t, []string{UpdateTaskID}, scheduler.removed)
}
