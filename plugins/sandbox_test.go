// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package plugins

import (
	"errors"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/config"
	"github.com/altessa-s/go-atlas/observability/health"
)

// newStubbedManager constructs a Manager with the sandbox-apply function
// replaced by stub. The override is per-Manager (not a package global) so
// parallel tests can each use their own stub without races.
func newStubbedManager(stub func(SandboxOptions) error, opts ...Option) *Manager {
	m := NewManager(opts...)
	m.sandboxApply = stub
	return m
}

func TestSandboxOptionsFromConfig(t *testing.T) {
	cfg := config.PluginsSandbox{
		Enabled:          true,
		NoNewPrivs:       true,
		MemoryLimitBytes: 1 << 28,
		MaxOpenFiles:     1024,
		MaxProcesses:     32,
		MaxFileSizeBytes: 1 << 30,
		DisableCoreDumps: true,
		Landlock: config.PluginsLandlock{
			Enabled:        true,
			ReadPaths:      []string{"/lib64", "/usr/lib64"},
			ReadWritePaths: []string{"/var/lib/myservice"},
		},
	}
	got := SandboxOptionsFromConfig(cfg)
	want := SandboxOptions{
		Enabled:          true,
		NoNewPrivs:       true,
		MemoryLimitBytes: 1 << 28,
		MaxOpenFiles:     1024,
		MaxProcesses:     32,
		MaxFileSizeBytes: 1 << 30,
		DisableCoreDumps: true,
		Landlock: LandlockOptions{
			Enabled:        true,
			ReadPaths:      []string{"/lib64", "/usr/lib64"},
			ReadWritePaths: []string{"/var/lib/myservice"},
		},
	}
	assert.Equal(t, want, got)
}

func TestApplySandbox_Disabled_NoOp(t *testing.T) {
	// Disabled config must be a no-op on every platform — even non-Linux —
	// so cross-platform builds work without behavior changes.
	err := applySandbox(SandboxOptions{Enabled: false})
	assert.NoError(t, err)
}

func TestEnsureSandbox_DisabledIsNoop(t *testing.T) {
	mgr := NewManager() // sandbox unset → Enabled=false
	t.Cleanup(func() { _ = mgr.Close() })

	require.NoError(t, mgr.ensureSandbox())
	// Calling twice is also fine.
	require.NoError(t, mgr.ensureSandbox())
}

func TestEnsureSandbox_AppliesOnceCachesResult(t *testing.T) {
	var calls int
	mgr := newStubbedManager(
		func(o SandboxOptions) error {
			calls++
			assert.True(t, o.Enabled)
			return nil
		},
		WithSandbox(SandboxOptions{Enabled: true, NoNewPrivs: true}),
	)
	t.Cleanup(func() { _ = mgr.Close() })

	require.NoError(t, mgr.ensureSandbox())
	require.NoError(t, mgr.ensureSandbox())
	require.NoError(t, mgr.ensureSandbox())

	assert.Equal(t, 1, calls, "applySandbox must run exactly once")
}

func TestEnsureSandbox_PropagatesError(t *testing.T) {
	stubErr := errors.New("simulated sandbox failure")
	mgr := newStubbedManager(
		func(SandboxOptions) error { return stubErr },
		WithSandbox(SandboxOptions{Enabled: true}),
	)
	t.Cleanup(func() { _ = mgr.Close() })

	err := mgr.ensureSandbox()
	require.Error(t, err)
	assert.ErrorIs(t, err, stubErr)

	// The cached error must be returned on every subsequent call.
	err2 := mgr.ensureSandbox()
	assert.Same(t, err, err2)
}

func TestManager_Load_PropagatesSandboxFailure(t *testing.T) {
	stubErr := errors.New("simulated sandbox failure")
	mgr := newStubbedManager(
		func(SandboxOptions) error { return stubErr },
		WithDir(t.TempDir()),
		WithSandbox(SandboxOptions{Enabled: true}),
	)
	t.Cleanup(func() { _ = mgr.Close() })

	err := mgr.Load(t.Context())
	require.Error(t, err)
	assert.ErrorIs(t, err, stubErr)
}

func TestManager_CheckHealth_FailsOnSandboxError(t *testing.T) {
	stubErr := errors.New("simulated sandbox failure")
	mgr := newStubbedManager(
		func(SandboxOptions) error { return stubErr },
		WithSandbox(SandboxOptions{Enabled: true}),
	)
	t.Cleanup(func() { _ = mgr.Close() })

	// Healthy before any Load attempt — sandbox has not been applied yet.
	assert.Equal(t, health.StatusServing, mgr.CheckHealth(t.Context()))

	// First Load triggers ensureSandbox, which fails and caches the error.
	err := mgr.Load(t.Context())
	require.ErrorIs(t, err, stubErr)

	// Health must now report unhealthy even though m.plugins is empty (no
	// failed plugin would otherwise trigger a NotServing response).
	assert.Equal(t, health.StatusNotServing, mgr.CheckHealth(t.Context()))
}

func TestManager_CheckHealth_RaceFreeWithSandboxFailure(t *testing.T) {
	// Regression for the race between CheckHealth reading m.sandboxErr and
	// ensureSandbox writing it inside sync.Once.Do. Without the atomic
	// pointer, -race would flag this scenario.
	stubErr := errors.New("simulated sandbox failure")
	mgr := newStubbedManager(
		func(SandboxOptions) error { return stubErr },
		WithDir(t.TempDir()),
		WithSandbox(SandboxOptions{Enabled: true}),
	)
	t.Cleanup(func() { _ = mgr.Close() })

	const goroutines = 8
	const iterations = 100

	ctx := t.Context()
	var wg sync.WaitGroup
	for range goroutines {
		wg.Go(func() {
			for range iterations {
				_ = mgr.Load(ctx)
			}
		})
	}
	for range goroutines {
		wg.Go(func() {
			for range iterations {
				_ = mgr.CheckHealth(ctx)
			}
		})
	}
	wg.Wait()

	// Eventual state: sandbox failed, health is NotServing.
	assert.Equal(t, health.StatusNotServing, mgr.CheckHealth(ctx))
}

// TestEnsureSandbox_LandlockReachedViaStub verifies that LandlockOptions
// flow all the way through ensureSandbox to the apply hook. The actual
// Landlock syscalls are stubbed because they would restrict the test process
// for every other test in the same binary.
func TestEnsureSandbox_LandlockReachedViaStub(t *testing.T) {
	var captured SandboxOptions
	mgr := newStubbedManager(
		func(o SandboxOptions) error {
			captured = o
			return nil
		},
		WithSandbox(SandboxOptions{
			Enabled: true,
			Landlock: LandlockOptions{
				Enabled:        true,
				ReadPaths:      []string{"/lib64", "/etc/myservice"},
				ReadWritePaths: []string{"/var/lib/myservice"},
			},
		}),
	)
	t.Cleanup(func() { _ = mgr.Close() })

	require.NoError(t, mgr.ensureSandbox())

	assert.True(t, captured.Landlock.Enabled)
	assert.Equal(t, []string{"/lib64", "/etc/myservice"}, captured.Landlock.ReadPaths)
	assert.Equal(t, []string{"/var/lib/myservice"}, captured.Landlock.ReadWritePaths)
}

// TestExpandSandboxOptions_StrictModeUnchanged verifies that with both
// auto-add flags off, expandSandboxOptions returns the input unmodified —
// the strict-allowlist contract from v2 is preserved as the default.
func TestExpandSandboxOptions_StrictModeUnchanged(t *testing.T) {
	mgr := NewManager(WithDir("/opt/myservice/plugins"))
	t.Cleanup(func() { _ = mgr.Close() })

	in := SandboxOptions{
		Enabled: true,
		Landlock: LandlockOptions{
			Enabled:   true,
			ReadPaths: []string{"/etc/myservice"},
		},
	}
	out := mgr.expandSandboxOptions(in)

	assert.Equal(t, []string{"/etc/myservice"}, out.Landlock.ReadPaths,
		"strict mode must not auto-add any path")
}

func TestExpandSandboxOptions_AllowPluginDirAppends(t *testing.T) {
	mgr := NewManager(WithDir("/opt/myservice/plugins"))
	t.Cleanup(func() { _ = mgr.Close() })

	in := SandboxOptions{
		Enabled: true,
		Landlock: LandlockOptions{
			Enabled:        true,
			AllowPluginDir: true,
			ReadPaths:      []string{"/etc/myservice"},
		},
	}
	out := mgr.expandSandboxOptions(in)

	assert.Equal(t,
		[]string{"/etc/myservice", "/opt/myservice/plugins"},
		out.Landlock.ReadPaths)
}

func TestExpandSandboxOptions_AllowSystemLibsAppends(t *testing.T) {
	// Override sandboxSystemLibPaths with two real temp directories so
	// the DirExists filter is satisfied regardless of the host's actual
	// filesystem layout (some CI runners do not have /lib64).
	libA := t.TempDir()
	libB := t.TempDir()

	mgr := NewManager(WithDir("/opt/myservice/plugins"))
	mgr.sandboxSystemLibPaths = []string{libA, libB}
	t.Cleanup(func() { _ = mgr.Close() })

	in := SandboxOptions{
		Enabled: true,
		Landlock: LandlockOptions{
			Enabled:         true,
			AllowSystemLibs: true,
			ReadPaths:       []string{"/etc/myservice"},
		},
	}
	out := mgr.expandSandboxOptions(in)

	assert.Equal(t,
		[]string{"/etc/myservice", libA, libB},
		out.Landlock.ReadPaths)
}

// TestExpandSandboxOptions_AllowSystemLibsFiltersMissingPaths verifies the
// DirExists filter: missing directories must be silently dropped from the
// merged ReadPaths so a host without /lib64 (Alpine, NixOS, slim Debian)
// does not fail Manager.Load with ENOENT from landlock_add_rule.
func TestExpandSandboxOptions_AllowSystemLibsFiltersMissingPaths(t *testing.T) {
	existing := t.TempDir()
	missing := filepath.Join(t.TempDir(), "does-not-exist")

	mgr := NewManager(WithDir("/opt/myservice/plugins"))
	mgr.sandboxSystemLibPaths = []string{existing, missing}
	t.Cleanup(func() { _ = mgr.Close() })

	in := SandboxOptions{
		Enabled: true,
		Landlock: LandlockOptions{
			Enabled:         true,
			AllowSystemLibs: true,
			ReadPaths:       []string{"/etc/myservice"},
		},
	}
	out := mgr.expandSandboxOptions(in)

	assert.Equal(t,
		[]string{"/etc/myservice", existing},
		out.Landlock.ReadPaths,
		"missing system lib paths must be silently dropped")
}

func TestExpandSandboxOptions_BothAllowsAppends(t *testing.T) {
	libA := t.TempDir()
	libB := t.TempDir()

	mgr := NewManager(WithDir("/opt/myservice/plugins"))
	mgr.sandboxSystemLibPaths = []string{libA, libB}
	t.Cleanup(func() { _ = mgr.Close() })

	in := SandboxOptions{
		Enabled: true,
		Landlock: LandlockOptions{
			Enabled:         true,
			AllowPluginDir:  true,
			AllowSystemLibs: true,
			ReadPaths:       []string{"/etc/myservice"},
		},
	}
	out := mgr.expandSandboxOptions(in)

	assert.Equal(t,
		[]string{"/etc/myservice", "/opt/myservice/plugins", libA, libB},
		out.Landlock.ReadPaths)
}

// TestExpandSandboxOptions_DoesNotMutateInputSlice verifies the slice-clone
// contract: expanding the options must NOT mutate the caller's ReadPaths,
// even when the append would fit within the original slice's capacity.
func TestExpandSandboxOptions_DoesNotMutateInputSlice(t *testing.T) {
	mgr := NewManager(WithDir("/opt/myservice/plugins"))
	t.Cleanup(func() { _ = mgr.Close() })

	// Allocate a slice with extra capacity so append would otherwise
	// mutate it in place.
	original := make([]string, 1, 16)
	original[0] = "/etc/myservice"

	in := SandboxOptions{
		Enabled: true,
		Landlock: LandlockOptions{
			Enabled:         true,
			AllowPluginDir:  true,
			AllowSystemLibs: true,
			ReadPaths:       original,
		},
	}
	_ = mgr.expandSandboxOptions(in)

	assert.Equal(t, 1, len(original), "caller's slice length must be unchanged")
	assert.Equal(t, "/etc/myservice", original[0], "caller's first element must be unchanged")
}

func TestExpandSandboxOptions_DisabledLandlockIsPassthrough(t *testing.T) {
	mgr := NewManager(WithDir("/opt/myservice/plugins"))
	t.Cleanup(func() { _ = mgr.Close() })

	in := SandboxOptions{
		Enabled: true,
		Landlock: LandlockOptions{
			Enabled:         false,
			AllowPluginDir:  true,
			AllowSystemLibs: true,
			ReadPaths:       []string{"/etc/myservice"},
		},
	}
	out := mgr.expandSandboxOptions(in)

	// When Landlock is disabled, the auto-add flags are ignored entirely.
	assert.Equal(t, []string{"/etc/myservice"}, out.Landlock.ReadPaths)
}

func TestSandboxOptionsFromConfig_MirrorsAllowFlags(t *testing.T) {
	cfg := config.PluginsSandbox{
		Enabled: true,
		Landlock: config.PluginsLandlock{
			Enabled:         true,
			AllowPluginDir:  true,
			AllowSystemLibs: true,
			ReadPaths:       []string{"/etc/myservice"},
		},
	}
	got := SandboxOptionsFromConfig(cfg)
	assert.True(t, got.Landlock.AllowPluginDir)
	assert.True(t, got.Landlock.AllowSystemLibs)
}
