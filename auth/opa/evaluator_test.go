// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package opa_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/auth/opa"
	"github.com/altessa-s/go-atlas/auth/opa/sources/filesystem"

	coremaps "github.com/altessa-s/go-atlas/core/collections/maps"
)

func writePolicy(t *testing.T, dir, content string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "policy.rego"), []byte(content), 0644))
}

func TestEvaluator_StructuredResult(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	tmpDir := t.TempDir()

	writePolicy(t, tmpDir, `
package test.authz
import rego.v1

default _authorized := false
default allow := false

deny contains {"code": "RESOURCE_INACTIVE", "message": "resource is inactive"} if {
    input.resource_status == "inactive"
    input.action != "read"
}

deny contains {"code": "PERMISSION_DENIED", "message": "insufficient permissions"} if {
    not _authorized
}

_authorized if input.role == "admin"
_authorized if {
    input.role == "viewer"
    input.action == "read"
}

allow if count(deny) == 0

result := {
    "allow": allow,
    "denials": deny,
}
`)

	source, err := filesystem.New(tmpDir)
	require.NoError(t, err)

	manager, err := opa.NewManager(ctx, source, "data.test.authz.result")
	require.NoError(t, err)
	defer manager.Close()

	evaluator := manager.Evaluator()

	t.Run("Allowed_NoDenials", func(t *testing.T) {
		t.Parallel()
		result, err := evaluator.Evaluate(ctx, map[string]any{
			"role":            "admin",
			"action":          "write",
			"resource_status": "active",
		})
		require.NoError(t, err)
		assert.True(t, result.Allow)
		assert.Empty(t, result.Denials)
	})

	t.Run("Denied_PermissionDenied", func(t *testing.T) {
		t.Parallel()
		result, err := evaluator.Evaluate(ctx, map[string]any{
			"role":            "viewer",
			"action":          "write",
			"resource_status": "active",
		})
		require.NoError(t, err)
		assert.False(t, result.Allow)
		assert.True(t, result.HasDenialCode("PERMISSION_DENIED"))
		assert.False(t, result.HasDenialCode("RESOURCE_INACTIVE"))
	})

	t.Run("Denied_ResourceInactive", func(t *testing.T) {
		t.Parallel()
		result, err := evaluator.Evaluate(ctx, map[string]any{
			"role":            "admin",
			"action":          "write",
			"resource_status": "inactive",
		})
		require.NoError(t, err)
		assert.False(t, result.Allow)
		assert.True(t, result.HasDenialCode("RESOURCE_INACTIVE"))
		assert.False(t, result.HasDenialCode("PERMISSION_DENIED"))
		msg, ok := result.Denials.Get("RESOURCE_INACTIVE")
		require.True(t, ok)
		assert.Equal(t, "resource is inactive", msg)
	})

	t.Run("Denied_MultipleDenials", func(t *testing.T) {
		t.Parallel()
		result, err := evaluator.Evaluate(ctx, map[string]any{
			"role":            "viewer",
			"action":          "write",
			"resource_status": "inactive",
		})
		require.NoError(t, err)
		assert.False(t, result.Allow)
		assert.True(t, result.HasDenialCode("RESOURCE_INACTIVE"))
		assert.True(t, result.HasDenialCode("PERMISSION_DENIED"))
		assert.Equal(t, 2, result.Denials.Len())
	})

	t.Run("Allowed_ReadOnInactive", func(t *testing.T) {
		t.Parallel()
		result, err := evaluator.Evaluate(ctx, map[string]any{
			"role":            "viewer",
			"action":          "read",
			"resource_status": "inactive",
		})
		require.NoError(t, err)
		assert.True(t, result.Allow)
		assert.Empty(t, result.Denials)
	})
}

func TestEvaluator_BooleanBackwardCompat(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	tmpDir := t.TempDir()

	writePolicy(t, tmpDir, `
package test.authz
import rego.v1
allow if {
    input.role == "admin"
}
`)

	source, err := filesystem.New(tmpDir)
	require.NoError(t, err)

	manager, err := opa.NewManager(ctx, source, "data.test.authz.allow")
	require.NoError(t, err)
	defer manager.Close()

	evaluator := manager.Evaluator()

	t.Run("Allowed_Bool", func(t *testing.T) {
		t.Parallel()
		result, err := evaluator.Evaluate(ctx, map[string]any{"role": "admin"})
		require.NoError(t, err)
		assert.True(t, result.Allow)
		assert.Nil(t, result.Denials)
	})

	t.Run("Denied_Bool", func(t *testing.T) {
		t.Parallel()
		result, err := evaluator.Evaluate(ctx, map[string]any{"role": "user"})
		require.NoError(t, err)
		assert.False(t, result.Allow)
		assert.Nil(t, result.Denials)
	})
}

func TestResult_HasDenialCode(t *testing.T) {
	t.Parallel()

	t.Run("NilDenials", func(t *testing.T) {
		t.Parallel()
		r := &opa.Result{Allow: false}
		assert.False(t, r.HasDenialCode("ANY"))
	})

	t.Run("EmptyDenials", func(t *testing.T) {
		t.Parallel()
		r := &opa.Result{Allow: false, Denials: coremaps.NewImmutableMap(map[string]string{})}
		assert.False(t, r.HasDenialCode("ANY"))
	})

	t.Run("Found", func(t *testing.T) {
		t.Parallel()
		r := &opa.Result{
			Allow:   false,
			Denials: coremaps.NewImmutableMap(map[string]string{"CODE_A": "msg a"}),
		}
		assert.True(t, r.HasDenialCode("CODE_A"))
		assert.False(t, r.HasDenialCode("CODE_B"))
	})
}
