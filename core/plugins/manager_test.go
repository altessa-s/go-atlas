// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package plugins

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/observability/health"
)

// newTestPlugin builds a *Plugin for tests with the given name and state.
// It exists because Plugin.state is an atomic field that cannot be set in a
// struct literal.
func newTestPlugin(name string, state State) *Plugin {
	p := &Plugin{descriptor: Descriptor{Name: name}}
	p.setState(state)
	return p
}

func TestManager_NewManager(t *testing.T) {
	mgr := NewManager()
	assert.NotNil(t, mgr)
	assert.NotNil(t, mgr.plugins)
	assert.False(t, mgr.closed.Load())
}

func TestManager_Load_DirNotFound(t *testing.T) {
	mgr := NewManager(WithDir("/nonexistent/path"))
	err := mgr.Load(t.Context())
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrDirNotFound)
}

func TestManager_Load_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	mgr := NewManager(WithDir(dir))
	err := mgr.Load(t.Context())
	require.NoError(t, err)
}

func TestManager_Load_SkipsNonSoFiles(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("hello"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "lib.dylib"), []byte("hello"), 0o644))
	require.NoError(t, os.Mkdir(filepath.Join(dir, "subdir"), 0o755))

	mgr := NewManager(WithDir(dir))
	err := mgr.Load(t.Context())
	require.NoError(t, err)

	count := 0
	for range mgr.Names() {
		count++
	}
	assert.Equal(t, 0, count)
}

func TestManager_resolvePluginFiles_LoadPriority(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"a.so", "b.so", "c.so"} {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte{}, 0o644))
	}

	mgr := NewManager(
		WithDir(dir),
		WithLoad("a.so", "c.so"),
		WithDisabled("a.so"), // should be ignored because Load is set
	)

	files, err := mgr.resolvePluginFiles()
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"a.so", "c.so"}, files)
}

func TestManager_resolvePluginFiles_Disabled(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"a.so", "b.so", "c.so"} {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte{}, 0o644))
	}

	mgr := NewManager(
		WithDir(dir),
		WithDisabled("b.so"),
	)

	files, err := mgr.resolvePluginFiles()
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"a.so", "c.so"}, files)
}

func TestManager_resolvePluginFiles_AllLoaded(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"x.so", "y.so"} {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte{}, 0o644))
	}

	mgr := NewManager(WithDir(dir))
	files, err := mgr.resolvePluginFiles()
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"x.so", "y.so"}, files)
}

func TestManager_Get_NotFound(t *testing.T) {
	mgr := NewManager()
	_, err := mgr.Get("nonexistent")
	assert.ErrorIs(t, err, ErrPluginNotFound)
}

func TestManager_MustGet_Panics(t *testing.T) {
	mgr := NewManager()
	assert.Panics(t, func() {
		mgr.MustGet("nonexistent")
	})
}

func TestManager_Close(t *testing.T) {
	mgr := NewManager()

	// Add a fake plugin directly for testing.
	mgr.plugins["test"] = newTestPlugin("test", StateReady)

	err := mgr.Close()
	require.NoError(t, err)
	assert.True(t, mgr.closed.Load())
	assert.Empty(t, mgr.plugins)
}

func TestManager_Close_Idempotent(t *testing.T) {
	mgr := NewManager()
	require.NoError(t, mgr.Close())
	require.NoError(t, mgr.Close())
}

func TestManager_Load_AfterClose(t *testing.T) {
	mgr := NewManager()
	require.NoError(t, mgr.Close())
	err := mgr.Load(t.Context())
	assert.ErrorIs(t, err, ErrManagerClosed)
}

func TestManager_Unload(t *testing.T) {
	mgr := NewManager()
	mgr.plugins["test"] = newTestPlugin("test", StateReady)

	err := mgr.Unload("test")
	require.NoError(t, err)

	_, err = mgr.Get("test")
	assert.ErrorIs(t, err, ErrPluginNotFound)
}

func TestManager_Unload_NotFound(t *testing.T) {
	mgr := NewManager()
	err := mgr.Unload("nonexistent")
	assert.ErrorIs(t, err, ErrPluginNotFound)
}

func TestManager_CheckHealth_PluginStates(t *testing.T) {
	cases := []struct {
		name    string
		plugins map[string]State
		want    health.ServingStatus
	}{
		{
			name:    "empty registry is serving",
			plugins: map[string]State{},
			want:    health.StatusServing,
		},
		{
			name:    "all ready is serving",
			plugins: map[string]State{"a": StateReady, "b": StateReady},
			want:    health.StatusServing,
		},
		{
			name:    "one failed among healthy is degraded",
			plugins: map[string]State{"a": StateReady, "b": StateFailed},
			want:    health.StatusDegraded,
		},
		{
			name:    "every plugin failed is not serving",
			plugins: map[string]State{"a": StateFailed, "b": StateFailed},
			want:    health.StatusNotServing,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mgr := NewManager()
			t.Cleanup(func() { _ = mgr.Close() })
			for name, state := range tc.plugins {
				mgr.plugins[name] = newTestPlugin(name, state)
			}
			assert.Equal(t, tc.want, mgr.CheckHealth(t.Context()))
		})
	}
}

func TestManager_Iterators(t *testing.T) {
	mgr := NewManager()
	mgr.plugins["ready1"] = newTestPlugin("ready1", StateReady)
	mgr.plugins["ready2"] = newTestPlugin("ready2", StateReady)
	mgr.plugins["failed"] = newTestPlugin("failed", StateFailed)

	// Plugins() should return all 3.
	allCount := 0
	for range mgr.Plugins() {
		allCount++
	}
	assert.Equal(t, 3, allCount)

	// Names() should return all 3.
	names := make([]string, 0, 3)
	for name := range mgr.Names() {
		names = append(names, name)
	}
	assert.Len(t, names, 3)

	// Ready() should return only 2.
	readyCount := 0
	for range mgr.Ready() {
		readyCount++
	}
	assert.Equal(t, 2, readyCount)
}

func TestManager_LookupAll(t *testing.T) {
	mgr := NewManager()

	// Simulate ready plugins with cached symbols.
	p1 := newTestPlugin("p1", StateReady)
	p1.symbols.Store("Provider", symbolEntry{value: "provider1", ok: true})

	p2 := newTestPlugin("p2", StateReady)
	p2.symbols.Store("Provider", symbolEntry{value: "provider2", ok: true})

	p3 := newTestPlugin("p3", StateFailed)
	p3.symbols.Store("Provider", symbolEntry{value: "provider3", ok: true})

	mgr.plugins["p1"] = p1
	mgr.plugins["p2"] = p2
	mgr.plugins["p3"] = p3

	var matches []string
	for _, sym := range mgr.LookupAll("Provider") {
		s, ok := sym.(string)
		require.True(t, ok)
		matches = append(matches, s)
	}

	assert.Len(t, matches, 2)
	assert.NotContains(t, matches, "provider3")
}

func TestDescriptor_State_String(t *testing.T) {
	tests := []struct {
		state State
		want  string
	}{
		{StateLoaded, "loaded"},
		{StateReady, "ready"},
		{StateFailed, "failed"},
		{StateUnloaded, "unloaded"},
		{State(99), "unknown"},
	}

	for _, tt := range tests {
		assert.Equal(t, tt.want, tt.state.String())
	}
}

func TestPlugin_Accessors(t *testing.T) {
	now := time.Now()
	p := &Plugin{
		descriptor: Descriptor{
			Name:        "test",
			Version:     "1.0.0",
			Description: "test plugin",
		},
		path:     "/path/to/test.so",
		loadedAt: now,
	}
	p.setState(StateReady)

	assert.Equal(t, "test", p.Name())
	assert.Equal(t, "1.0.0", p.Version())
	assert.Equal(t, "test plugin", p.Description())
	assert.Equal(t, StateReady, p.State())
	assert.Equal(t, "/path/to/test.so", p.Path())
	assert.Equal(t, now, p.LoadedAt())
	assert.NoError(t, p.Err())
}

func TestManager_Len(t *testing.T) {
	mgr := NewManager()
	assert.Equal(t, 0, mgr.Len())

	mgr.plugins["a"] = newTestPlugin("a", StateReady)
	mgr.plugins["b"] = newTestPlugin("b", StateFailed)
	assert.Equal(t, 2, mgr.Len())
}

func TestPlugin_Lookup_NilRaw(t *testing.T) {
	p := newTestPlugin("test", StateReady)

	// Lookup on nil raw should return false, not panic.
	sym, ok := p.Lookup("SomeSymbol")
	assert.Nil(t, sym)
	assert.False(t, ok)
}

func TestPlugin_Err_RoundTrip(t *testing.T) {
	p := newTestPlugin("test", StateLoaded)
	if got := p.Err(); got != nil {
		t.Errorf("Err on fresh plugin: got %v, want nil", got)
	}

	want := ErrPluginFailed
	p.setErr(want)
	if got := p.Err(); got != want {
		t.Errorf("Err after setErr: got %v, want %v", got, want)
	}
}

func TestPlugin_Lookup_Cached(t *testing.T) {
	p := newTestPlugin("test", StateReady)

	// Pre-cache a symbol.
	p.symbols.Store("Cached", symbolEntry{value: 42, ok: true})

	sym, ok := p.Lookup("Cached")
	assert.True(t, ok)
	assert.Equal(t, 42, sym)
}

func TestManager_Reload_SkipsExisting(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"a.so", "b.so"} {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte{}, 0o644))
	}

	mgr := NewManager(WithDir(dir))

	// Simulate "a.so" as already loaded.
	existing := &Plugin{
		descriptor: Descriptor{Name: "existing"},
		path:       filepath.Join(dir, "a.so"),
	}
	existing.setState(StateReady)
	mgr.plugins["existing"] = existing

	// Reload will try to load b.so (which will fail since it's not a valid .so),
	// but should skip a.so.
	_ = mgr.Reload(t.Context())

	// a.so should still be there (not re-loaded).
	p, err := mgr.Get("existing")
	require.NoError(t, err)
	assert.Equal(t, StateReady, p.State())
}

func TestManager_Reload_AfterClose(t *testing.T) {
	mgr := NewManager()
	require.NoError(t, mgr.Close())
	err := mgr.Reload(t.Context())
	assert.ErrorIs(t, err, ErrManagerClosed)
}

// TestManager_ConcurrentStateAccess exercises concurrent reads and writes of
// Plugin.state to verify the atomic-field fix is race-free under -race.
func TestManager_ConcurrentStateAccess(t *testing.T) {
	ctx := t.Context()
	mgr := NewManager()
	mgr.plugins["a"] = newTestPlugin("a", StateLoaded)
	mgr.plugins["b"] = newTestPlugin("b", StateLoaded)

	const goroutines = 8
	const iterations = 200

	var wg sync.WaitGroup

	// Writers cycle plugin state.
	for range goroutines {
		wg.Go(func() {
			for range iterations {
				p, err := mgr.Get("a")
				if err == nil {
					p.setState(StateReady)
					p.setState(StateFailed)
				}
			}
		})
	}

	// Readers iterate via the public API.
	for range goroutines {
		wg.Go(func() {
			for range iterations {
				for _, p := range mgr.Plugins() {
					_ = p.State()
				}
				_ = mgr.CheckHealth(ctx)
			}
		})
	}

	wg.Wait()
}
