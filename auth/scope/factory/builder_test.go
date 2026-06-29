// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/auth/scope"
	"github.com/altessa-s/go-atlas/auth/scope/factory"
	"github.com/altessa-s/go-atlas/config"
)

func TestBuildRegistersRules(t *testing.T) {
	t.Parallel()
	reg, err := factory.New(&config.ScopeRegistry{Rules: []config.ScopeRule{
		{Scope: "files:read", Keys: []string{"/files.v1.Files/Read", "/files.v1.Files/List"}},
		{Scope: "files:write", Keys: []string{"/files.v1.Files/Write"}},
		{Scope: "", Keys: []string{"/health.v1.Health/Check"}},
	}}).Build()
	require.NoError(t, err)

	got, ok := reg.Required("/files.v1.Files/Read")
	require.True(t, ok)
	require.Equal(t, "files:read", got)

	pub, ok := reg.Required("/health.v1.Health/Check")
	require.True(t, ok)
	require.Equal(t, "", pub) // public

	_, ok = reg.Required("/unregistered")
	require.False(t, ok)
}

func TestBuildFreezesRegistry(t *testing.T) {
	t.Parallel()
	reg, err := factory.New(&config.ScopeRegistry{Rules: []config.ScopeRule{
		{Scope: "files:read", Keys: []string{"/files.v1.Files/Read"}},
	}}).Build()
	require.NoError(t, err)
	require.Panics(t, func() { reg.Register("/x", "y") }) // frozen
}

func TestBuildNilConfig(t *testing.T) {
	t.Parallel()
	_, err := factory.New(nil).Build()
	require.Error(t, err)
}

func TestBuildEmptyKeys(t *testing.T) {
	t.Parallel()
	_, err := factory.New(&config.ScopeRegistry{Rules: []config.ScopeRule{
		{Scope: "files:read"},
	}}).Build()
	require.Error(t, err)
}

func TestBuildDuplicateKey(t *testing.T) {
	t.Parallel()
	_, err := factory.New(&config.ScopeRegistry{Rules: []config.ScopeRule{
		{Scope: "files:read", Keys: []string{"/files.v1.Files/Read"}},
		{Scope: "files:write", Keys: []string{"/files.v1.Files/Read"}}, // duplicate
	}}).Build()
	require.Error(t, err)
}

// The built registry drives an Enforcer exactly like a hand-built one.
func TestBuiltRegistryWithEnforcer(t *testing.T) {
	t.Parallel()
	reg, err := factory.New(&config.ScopeRegistry{Rules: []config.ScopeRule{
		{Scope: "files:write", Keys: []string{"/files.v1.Files/Write"}},
	}}).Build()
	require.NoError(t, err)

	type principal struct{ scopes []string }
	enf := scope.NewEnforcer(reg, scope.ScopeAuthorizer(
		func(p *principal) []scope.Scope { return p.scopes }, scope.Exact()))

	require.NoError(t, enf.Enforce(&principal{scopes: []string{"files:write"}}, "/files.v1.Files/Write"))
	require.ErrorIs(t, enf.Enforce(&principal{scopes: []string{"files:read"}}, "/files.v1.Files/Write"), scope.ErrAccessDenied)
}
