// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package scope_test

import (
	"maps"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/auth/scope"
)

func TestRegistryRequired(t *testing.T) {
	t.Parallel()

	r := scope.NewRegistry()
	r.Register("/files.v1.Files/Read", "files:read")
	r.Register("/health.v1.Health/Check", "") // public

	got, ok := r.Required("/files.v1.Files/Read")
	require.True(t, ok)
	require.Equal(t, "files:read", got)

	got, ok = r.Required("/health.v1.Health/Check")
	require.True(t, ok)
	require.Empty(t, got)

	_, ok = r.Required("/files.v1.Files/Unknown")
	require.False(t, ok, "unregistered key must report ok=false")
}

func TestRegistryRegisterMany(t *testing.T) {
	t.Parallel()

	r := scope.NewRegistry()
	r.RegisterMany("files:read", "/a/Read", "/b/List")

	for _, key := range []string{"/a/Read", "/b/List"} {
		got, ok := r.Required(key)
		require.True(t, ok)
		require.Equal(t, "files:read", got)
	}
	require.Equal(t, 2, r.Len())
}

func TestRegistryOverwrite(t *testing.T) {
	t.Parallel()

	r := scope.NewRegistry()
	r.Register("/a/Do", "files:read")
	r.Register("/a/Do", "files:write")

	got, _ := r.Required("/a/Do")
	require.Equal(t, "files:write", got, "last registration wins")
}

func TestRegistryFreezeReads(t *testing.T) {
	t.Parallel()

	r := scope.NewRegistry()
	r.Register("/a/Do", "files:read")
	r.Freeze()

	got, ok := r.Required("/a/Do")
	require.True(t, ok)
	require.Equal(t, "files:read", got)
	require.Equal(t, 1, r.Len())
}

func TestRegistryFreezeIdempotent(t *testing.T) {
	t.Parallel()

	r := scope.NewRegistry()
	r.Register("/a/Do", "files:read")
	r.Freeze()
	require.NotPanics(t, r.Freeze)
}

func TestRegistryRegisterAfterFreezePanics(t *testing.T) {
	t.Parallel()

	r := scope.NewRegistry()
	r.Freeze()
	require.Panics(t, func() { r.Register("/a/Do", "files:read") })
	require.Panics(t, func() { r.RegisterMany("files:read", "/a/Do") })
}

func TestRegistryAll(t *testing.T) {
	t.Parallel()

	r := scope.NewRegistry()
	r.Register("/a/Read", "files:read")
	r.Register("/b/Write", "files:write")
	r.Freeze()

	got := maps.Collect(r.All())
	require.Equal(t, map[string]string{
		"/a/Read":  "files:read",
		"/b/Write": "files:write",
	}, got)
}
