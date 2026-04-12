// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package appinfo

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEnv(t *testing.T) {
	t.Setenv("TEST_APPINFO_KEY", "value123")
	require.Equal(t, "value123", Env("TEST_APPINFO_KEY"))
	require.Equal(t, "", Env("TEST_APPINFO_MISSING"))
}

func TestEnvOr(t *testing.T) {
	t.Setenv("TEST_APPINFO_ENVOR", "set")
	require.Equal(t, "set", EnvOr("TEST_APPINFO_ENVOR", "default"))
	require.Equal(t, "fallback", EnvOr("TEST_APPINFO_MISSING_ENVOR", "fallback"))
}

func TestEnvCached(t *testing.T) {
	ClearEnvCache()
	t.Setenv("TEST_APPINFO_CACHED", "cached_val")

	v1 := EnvCached("TEST_APPINFO_CACHED")
	require.Equal(t, "cached_val", v1)

	// Change env but cache should still return old value.
	t.Setenv("TEST_APPINFO_CACHED", "new_val")
	v2 := EnvCached("TEST_APPINFO_CACHED")
	require.Equal(t, "cached_val", v2)
}

func TestClearEnvCache(t *testing.T) {
	ClearEnvCache()
	t.Setenv("TEST_APPINFO_CLEAR", "original")
	_ = EnvCached("TEST_APPINFO_CLEAR")

	ClearEnvCache()
	t.Setenv("TEST_APPINFO_CLEAR", "updated")
	require.Equal(t, "updated", EnvCached("TEST_APPINFO_CLEAR"))
}

func TestExpandPath(t *testing.T) {
	tests := []struct {
		name string
		path string
		home bool // whether result should start with home dir
	}{
		{"tilde path", "~/Documents", true},
		{"absolute path", "/tmp/file", false},
		{"relative path", "relative/path", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExpandPath(tt.path)
			if tt.home {
				home := HomeDir()
				if home != "" {
					require.Equal(t, home, got[:len(home)], "expected to start with home dir")
				}
			} else {
				require.Equal(t, tt.path, got)
			}
		})
	}
}

func TestHomeDir(t *testing.T) {
	home := HomeDir()
	if home == "" {
		t.Skip("HOME not set")
	}
	require.Equal(t, byte('/'), home[0], "expected absolute path, got %q", home)
}
