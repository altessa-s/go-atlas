// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/auth/opa"
	"github.com/altessa-s/go-atlas/internal/testhelpers"
	"github.com/altessa-s/go-atlas/observability/health"
	"github.com/altessa-s/go-atlas/observability/metrics/metricstest"

	corehash "github.com/altessa-s/go-atlas-core/encoding/hash"
	authconfig "github.com/altessa-s/go-atlas/config/auth"
)

// bundleDir writes a minimal policy bundle and returns its directory.
func bundleDir(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "policy.rego"), []byte(`
package profiles.authz
import rego.v1
allow if input.role == "admin"
`), 0o600))

	return dir
}

func filesystemConfig(t *testing.T) *authconfig.OPA {
	t.Helper()

	cfg := authconfig.DefaultOPA()
	cfg.Source = authconfig.OPASourceFilesystem
	cfg.BundlePath = bundleDir(t)
	cfg.ReleaseManifest = "release.json"
	publishFilesystemRelease(t, &cfg)
	cfg.Query = "data.profiles.authz.allow"

	return &cfg
}

func TestBuild_RequiresConfig(t *testing.T) {
	t.Parallel()

	_, err := New(nil).Build(t.Context())
	require.Error(t, err)
}

func TestBuild_RequiresEnabledSource(t *testing.T) {
	t.Parallel()

	cfg := authconfig.DefaultOPA()
	cfg.Source = authconfig.OPASourceFilesystem
	cfg.BundlePath = "" // IsEnabled is false without a bundle path

	_, err := New(&cfg).Build(t.Context())
	require.Error(t, err)
}

func TestBuild_FilesystemSource(t *testing.T) {
	t.Parallel()

	manager, err := New(filesystemConfig(t)).Build(t.Context())
	require.NoError(t, err)
	t.Cleanup(func() { _ = manager.Close() })

	result, err := manager.Evaluator().Evaluate(t.Context(), map[string]any{"role": "admin"})
	require.NoError(t, err)
	require.True(t, result.Allow)
}

// The regression this mapping exists for: `opa.cache` was described, defaulted
// and validated while nothing read it, so enabling it produced neither caching
// nor an error.
func TestManagerOptions_MapsCache(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		cache     *authconfig.OPACache
		wantExtra int
	}{
		{"absent section", nil, 0},
		{"disabled", &authconfig.OPACache{Enabled: false, TTL: time.Minute, MaxSize: 100}, 0},
		{"enabled", &authconfig.OPACache{Enabled: true, TTL: time.Minute, MaxSize: 100}, 1},
		// Neither a zero TTL nor a zero size is a reason to skip caching; both
		// are passed through and read as "use the package default".
		{"enabled without ttl or size", &authconfig.OPACache{Enabled: true}, 1},
	}

	baseline := len(New(filesystemConfig(t)).managerOptions())

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			cfg := filesystemConfig(t)
			cfg.Cache = tc.cache

			require.Len(t, New(cfg).managerOptions(), baseline+tc.wantExtra)
		})
	}
}

func TestBuild_RegistersUpdateSchedule(t *testing.T) {
	t.Parallel()

	cfg := filesystemConfig(t)
	cfg.UpdateSchedule = "@every 5m"

	registrar := &testhelpers.MockTaskRegistrar{}
	manager, err := New(cfg).UseScheduler(registrar).Build(t.Context())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, manager.Close()) })
	tasks := registrar.Tasks()
	require.Len(t, tasks, 1)
	require.Equal(t, cfg.UpdateSchedule, tasks[0].Schedule)
}

func TestManagerOptions_MapsMeter(t *testing.T) {
	t.Parallel()

	meter := metricstest.NewTestCollector()
	manager, err := New(filesystemConfig(t)).UseMeter(meter).Build(t.Context())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, manager.Close()) })

	require.EqualValues(t, 1, metricstest.GetGaugeValue(t, meter, "test_auth_opa_modules_loaded"))
}

func TestBuild_ScheduleWithoutSchedulerFails(t *testing.T) {
	t.Parallel()

	cfg := filesystemConfig(t)
	cfg.UpdateSchedule = "@every 5m"

	_, err := New(cfg).Build(t.Context())
	require.ErrorIs(t, err, opa.ErrSchedulerRequired)
}

func TestBuild_WatchWithoutScheduler(t *testing.T) {
	t.Parallel()

	cfg := filesystemConfig(t)
	cfg.WatchBundle = true
	cfg.UpdateSchedule = "@every 5m"
	manager, err := New(cfg).Build(t.Context())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, manager.Close()) })
	require.True(t, manager.IsWatching())
}

func TestBuild_MissingReleaseManifest(t *testing.T) {
	t.Parallel()

	fsConfig := filesystemConfig(t)
	fsConfig.ReleaseManifest = ""
	_, err := New(fsConfig).Build(t.Context())
	require.ErrorIs(t, err, opa.ErrReleaseManifestRequired)

	s3Config := authconfig.DefaultOPA()
	s3Config.Source = authconfig.OPASourceS3
	s3Config.S3 = &authconfig.OPAS3{Bucket: "policies"}
	_, err = New(&s3Config).UseS3Client(fakeS3{}).Build(t.Context())
	require.ErrorIs(t, err, opa.ErrReleaseManifestRequired)
}

func publishFilesystemRelease(t testing.TB, cfg *authconfig.OPA) {
	t.Helper()

	content, err := os.ReadFile(filepath.Join(cfg.BundlePath, "policy.rego"))
	require.NoError(t, err)
	manifest, err := json.Marshal(map[string]string{"policy.rego": corehash.SHA256HexBytes(content)})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(cfg.BundlePath, cfg.ReleaseManifest), manifest, 0o600))
}

// End-to-end proof that the mapped option reaches the manager: with caching on,
// a repeated input is answered without re-entering Rego, which is observable
// because a bundle swapped behind the manager's back changes nothing until
// reload — while the cache also survives it.
func TestBuild_CacheEnabledStillAnswersCorrectly(t *testing.T) {
	t.Parallel()

	cfg := filesystemConfig(t)
	cfg.Cache = &authconfig.OPACache{Enabled: true, TTL: time.Minute}

	manager, err := New(cfg).Build(t.Context())
	require.NoError(t, err)
	t.Cleanup(func() { _ = manager.Close() })

	evaluator := manager.Evaluator()

	allowed, err := evaluator.Evaluate(t.Context(), map[string]any{"role": "admin"})
	require.NoError(t, err)
	require.True(t, allowed.Allow)

	// Served from the cache; must agree with the uncached answer.
	again, err := evaluator.Evaluate(t.Context(), map[string]any{"role": "admin"})
	require.NoError(t, err)
	require.True(t, again.Allow)

	denied, err := evaluator.Evaluate(t.Context(), map[string]any{"role": "guest"})
	require.NoError(t, err)
	require.False(t, denied.Allow)
}

func TestWatchSurvivesBuildContext(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		cfg := filesystemConfig(t)
		cfg.WatchBundle = true
		cfg.PollInterval = time.Minute
		ctx, cancel := context.WithCancel(t.Context())
		m, err := New(cfg).Build(ctx)
		require.NoError(t, err)
		defer m.Close()
		sub, err := m.Watch(t.Context(), opa.WatchOptions{})
		require.NoError(t, err)
		cancel()
		synctest.Wait()
		require.NoError(t, os.WriteFile(filepath.Join(cfg.BundlePath, "policy.rego"), []byte("package profiles.authz\nallow := false"), 0o600))
		publishFilesystemRelease(t, cfg)
		select {
		case <-sub.Events:
		case <-time.After(2 * time.Minute):
			require.FailNow(t, "watcher did not reload after Build context cancellation")
		}
		r, err := m.Evaluator().Evaluate(t.Context(), map[string]any{"role": "admin"})
		require.NoError(t, err)
		require.False(t, r.Allow)
		require.NoError(t, m.Close())
		<-sub.Done
	})
}

type countedSource struct {
	fetchErr error
	closed   int
}

func (s *countedSource) Name() string { return "counted" }
func (s *countedSource) Fetch(context.Context) (*opa.PolicyBundle, error) {
	if s.fetchErr != nil {
		return nil, s.fetchErr
	}
	return opa.NewPolicyBundle(map[string][]byte{"p.rego": []byte("package t")}), nil
}
func (s *countedSource) Close() error {
	s.closed++
	return nil
}

func TestBuildManagerSourceOwnership(t *testing.T) {
	t.Parallel()
	for _, failure := range []string{"fetch", "register", "success"} {
		t.Run(failure, func(t *testing.T) {
			t.Parallel()
			boom := errors.New("construction failed")
			src := &countedSource{}
			cfg := filesystemConfig(t)
			cfg.UpdateSchedule = "@every 1m"
			reg := &testhelpers.MockTaskRegistrar{}
			switch failure {
			case "fetch":
				src.fetchErr = boom
			case "register":
				reg.Err = boom
			}
			builder := New(cfg).UseScheduler(reg)
			m, err := builder.buildManager(t.Context(), src, builder.managerOptions()...)
			if failure == "success" {
				require.NoError(t, err)
				require.Zero(t, src.closed)
				require.NoError(t, m.Close())
				require.ErrorIs(t, m.StartWatchingUntilClose(), opa.ErrManagerClosed)
				require.NoError(t, m.Close())
			} else {
				require.ErrorIs(t, err, boom)
			}
			require.Equal(t, 1, src.closed)
		})
	}
}

// Close the actual manager in the window between registration and the factory
// starting its watcher. StartWatching's only failure is an already closed manager.
type closeOnRegister struct{ *health.Coordinator }

func (r closeOnRegister) RegisterScopedService(name string, c health.Checker) (func(health.ServingStatus), func()) {
	notify, remove := r.Coordinator.RegisterScopedService(name, c)
	_ = c.(*opa.Manager).Close()
	return notify, remove
}
func TestWatchStartFailureClosesSourceOnce(t *testing.T) {
	t.Parallel()
	cfg := filesystemConfig(t)
	cfg.WatchBundle = true
	reg := closeOnRegister{health.New()}
	defer reg.Coordinator.Close()
	src := &countedSource{}
	b := New(cfg)
	_, err := b.buildManager(t.Context(), src, opa.WithHealthCoordinator(reg))
	require.ErrorIs(t, err, opa.ErrManagerClosed)
	require.Equal(t, 1, src.closed)
	require.False(t, slices.Contains(slices.Collect(reg.ListServices()), "opa"))
}
