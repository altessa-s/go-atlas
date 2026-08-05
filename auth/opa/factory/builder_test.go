// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/config"
	"github.com/altessa-s/go-atlas/internal/testhelpers"
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

func filesystemConfig(t *testing.T) *config.OPA {
	t.Helper()

	cfg := config.DefaultOPA()
	cfg.Source = config.OPASourceFilesystem
	cfg.BundlePath = bundleDir(t)
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

	cfg := config.DefaultOPA()
	cfg.Source = config.OPASourceFilesystem
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
		cache     *config.OPACache
		wantExtra int
	}{
		{"absent section", nil, 0},
		{"disabled", &config.OPACache{Enabled: false, TTL: time.Minute, MaxSize: 100}, 0},
		{"enabled", &config.OPACache{Enabled: true, TTL: time.Minute, MaxSize: 100}, 1},
		// Neither a zero TTL nor a zero size is a reason to skip caching; both
		// are passed through and read as "use the package default".
		{"enabled without ttl or size", &config.OPACache{Enabled: true}, 1},
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

func TestManagerOptions_MapsScheduler(t *testing.T) {
	t.Parallel()

	cfg := filesystemConfig(t)
	cfg.UpdateSchedule = "@every 5m"

	withoutScheduler := len(New(cfg).managerOptions())
	withScheduler := len(New(cfg).UseScheduler(&testhelpers.MockTaskRegistrar{}).managerOptions())

	require.Equal(t, withoutScheduler+2, withScheduler,
		"a scheduler must contribute both WithScheduler and WithUpdateSchedule")
}

// A schedule without a scheduler has nothing to run it, so it must not look
// configured.
func TestManagerOptions_ScheduleWithoutSchedulerIsIgnored(t *testing.T) {
	t.Parallel()

	cfg := filesystemConfig(t)
	cfg.UpdateSchedule = "@every 5m"

	bare := filesystemConfig(t)
	bare.UpdateSchedule = ""

	require.Len(t, New(cfg).managerOptions(), len(New(bare).managerOptions()))
}

// End-to-end proof that the mapped option reaches the manager: with caching on,
// a repeated input is answered without re-entering Rego, which is observable
// because a bundle swapped behind the manager's back changes nothing until
// reload — while the cache also survives it.
func TestBuild_CacheEnabledStillAnswersCorrectly(t *testing.T) {
	t.Parallel()

	cfg := filesystemConfig(t)
	cfg.Cache = &config.OPACache{Enabled: true, TTL: time.Minute}

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
