// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/config"
	"github.com/altessa-s/go-atlas/plugins/factory"
)

func TestManagerBuilder_Build_NilConfig(t *testing.T) {
	_, err := factory.NewManager(nil).Build(t.Context())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "plugins configuration")
}

func TestManagerBuilder_Build_Disabled(t *testing.T) {
	cfg := &config.Plugins{Enabled: false}
	_, err := factory.NewManager(cfg).Build(t.Context())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not enabled")
}

func TestManagerBuilder_Build_EnabledEmptyDir(t *testing.T) {
	cfg := &config.Plugins{
		Enabled: true,
		Dir:     t.TempDir(),
	}
	mgr, err := factory.NewManager(cfg).Build(t.Context())
	require.NoError(t, err)
	t.Cleanup(func() { _ = mgr.Close() })

	assert.NotNil(t, mgr)
	assert.Equal(t, 0, mgr.Len())
	assert.False(t, mgr.IsWatching(), "watcher must stay off when cfg.Watch is false")
}

func TestManagerBuilder_Build_WatchEnabled(t *testing.T) {
	cfg := &config.Plugins{
		Enabled: true,
		Dir:     t.TempDir(),
		Watch:   true,
	}
	mgr, err := factory.NewManager(cfg).Build(t.Context())
	require.NoError(t, err)
	t.Cleanup(func() { _ = mgr.Close() })

	assert.True(t, mgr.IsWatching(), "watcher must auto-start when cfg.Watch is true")
}

func TestManagerBuilder_Build_WatchFailsOnMissingDir(t *testing.T) {
	cfg := &config.Plugins{
		Enabled: true,
		Dir:     "/definitely/not/a/real/path/for/plugins",
		Watch:   true,
	}
	_, err := factory.NewManager(cfg).Build(t.Context())
	// Load fails on the missing directory before watch gets a chance to start.
	require.Error(t, err)
}
