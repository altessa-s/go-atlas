// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package appinfo_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/core/runtime/appinfo"
)

func TestDependencies_Contains_Found(t *testing.T) {
	deps := appinfo.Deps()
	// At least one dependency should exist in a real Go module
	if len(deps) == 0 {
		t.Skip("no dependencies found")
	}
	first := deps[0]
	require.True(t, deps.Contains(first.Path), "Contains(%q) = false, want true", first.Path)
}

func TestDependencies_Contains_NotFound(t *testing.T) {
	deps := appinfo.Deps()
	require.False(t, deps.Contains("nonexistent/module/path"), "Contains(nonexistent) = true, want false")
}

func TestDependencies_Get_Found(t *testing.T) {
	deps := appinfo.Deps()
	if len(deps) == 0 {
		t.Skip("no dependencies found")
	}
	first := deps[0]
	got := deps.Get(first.Path)
	require.NotNil(t, got, "Get(%q) = nil, want non-nil", first.Path)
}

func TestDependencies_Get_NotFound(t *testing.T) {
	deps := appinfo.Deps()
	require.Nil(t, deps.Get("nonexistent/module/path"), "Get(nonexistent) should return nil")
}

func TestHomeDir(t *testing.T) {
	home := appinfo.HomeDir()
	require.NotEmpty(t, home, "HomeDir() should not be empty")
}
