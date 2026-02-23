// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package opa_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/auth/opa"
	"github.com/altessa-s/go-atlas/auth/opa/sources/filesystem"
	"github.com/altessa-s/go-atlas/observability/health"
)

func TestManager_Evaluate(t *testing.T) {
	ctx := t.Context()

	// Setup temporary policy
	tmpDir, err := os.MkdirTemp("", "opa-test")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	policyPath := filepath.Join(tmpDir, "policy.rego")
	policyContent := `
package test.authz
import rego.v1
allow if {
    input.role == "admin"
}
`
	err = os.WriteFile(policyPath, []byte(policyContent), 0644)
	require.NoError(t, err)

	source, err := filesystem.New(tmpDir)
	require.NoError(t, err)

	manager, err := opa.NewManager(ctx, source, "data.test.authz.allow")
	require.NoError(t, err)
	defer manager.Close()

	evaluator := manager.Evaluator()

	t.Run("Allowed", func(t *testing.T) {
		result, err := evaluator.Evaluate(ctx, map[string]any{"role": "admin"})
		require.NoError(t, err)
		assert.True(t, result.Allow)
	})

	t.Run("Denied", func(t *testing.T) {
		result, err := evaluator.Evaluate(ctx, map[string]any{"role": "user"})
		require.NoError(t, err)
		assert.False(t, result.Allow)
	})
}

func TestManager_HotReload(t *testing.T) {
	ctx := t.Context()

	tmpDir, err := os.MkdirTemp("", "opa-test-hotreload")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	policyPath := filepath.Join(tmpDir, "policy.rego")
	policyContent := `
package test.authz
import rego.v1
allow if {
    input.role == "admin"
}
`
	err = os.WriteFile(policyPath, []byte(policyContent), 0644)
	require.NoError(t, err)

	source, err := filesystem.New(tmpDir)
	require.NoError(t, err)

	manager, err := opa.NewManager(ctx, source, "data.test.authz.allow")
	require.NoError(t, err)
	defer manager.Close()

	// Start watching for changes
	err = manager.StartWatching(ctx)
	require.NoError(t, err)

	evaluator := manager.Evaluator()

	// Verify initial policy works
	result, err := evaluator.Evaluate(ctx, map[string]any{"role": "admin"})
	require.NoError(t, err)
	assert.True(t, result.Allow)

	// Update the policy
	newPolicyContent := `
package test.authz
import rego.v1
allow if {
    input.role == "user"
}
`
	err = os.WriteFile(policyPath, []byte(newPolicyContent), 0644)
	require.NoError(t, err)

	// Give fsnotify time to detect the change and reload
	assert.Eventually(t, func() bool {
		result, err := evaluator.Evaluate(ctx, map[string]any{"role": "user"})
		return err == nil && result.Allow
	}, 5*time.Second, 100*time.Millisecond, "Policy should reload and allow user role")
}

func TestManager_Watch(t *testing.T) {
	ctx := t.Context()

	tmpDir, err := os.MkdirTemp("", "opa-test-watch")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	policyPath := filepath.Join(tmpDir, "policy.rego")
	policyContent := `
package test.authz
import rego.v1
allow if {
    input.role == "admin"
}
`
	err = os.WriteFile(policyPath, []byte(policyContent), 0644)
	require.NoError(t, err)

	source, err := filesystem.New(tmpDir)
	require.NoError(t, err)

	manager, err := opa.NewManager(ctx, source, "data.test.authz.allow")
	require.NoError(t, err)
	defer manager.Close()

	// Subscribe to events
	watchResult, err := manager.Watch(ctx, opa.DefaultWatchOptions())
	require.NoError(t, err)
	defer watchResult.Stop()

	// Start watching
	err = manager.StartWatching(ctx)
	require.NoError(t, err)

	// Give watcher time to initialize - filesystem watchers need sufficient time
	// to register with the kernel, especially in CI environments
	time.Sleep(200 * time.Millisecond)

	// Update the policy
	newPolicyContent := `
package test.authz
import rego.v1
allow if {
    input.role == "superadmin"
}
`
	err = os.WriteFile(policyPath, []byte(newPolicyContent), 0644)
	require.NoError(t, err)

	// Wait for event
	select {
	case event := <-watchResult.Events:
		assert.Equal(t, opa.EventTypePolicyUpdated, event.Type)
		assert.NotEmpty(t, event.Revision)
	case <-time.After(5 * time.Second):
		t.Fatal("Timeout waiting for policy update event")
	}
}

func TestManager_HealthCheck(t *testing.T) {
	ctx := t.Context()

	tmpDir, err := os.MkdirTemp("", "opa-test-health")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	policyPath := filepath.Join(tmpDir, "policy.rego")
	policyContent := `
package test.authz
import rego.v1
allow if {
    input.role == "admin"
}
`
	err = os.WriteFile(policyPath, []byte(policyContent), 0644)
	require.NoError(t, err)

	source, err := filesystem.New(tmpDir)
	require.NoError(t, err)

	manager, err := opa.NewManager(ctx, source, "data.test.authz.allow")
	require.NoError(t, err)
	defer manager.Close()

	assert.Equal(t, health.StatusServing, manager.CheckHealth(ctx))
}

func TestManager_RunUpdateCycle(t *testing.T) {
	ctx := t.Context()

	tmpDir, err := os.MkdirTemp("", "opa-test-cycle")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	policyPath := filepath.Join(tmpDir, "policy.rego")
	policyContent := `
package test.authz
import rego.v1
allow if {
    input.role == "admin"
}
`
	err = os.WriteFile(policyPath, []byte(policyContent), 0644)
	require.NoError(t, err)

	source, err := filesystem.New(tmpDir)
	require.NoError(t, err)

	manager, err := opa.NewManager(ctx, source, "data.test.authz.allow")
	require.NoError(t, err)
	defer manager.Close()

	initialRevision := manager.Revision()

	// Update policy
	newPolicyContent := `
package test.authz
import rego.v1
allow if {
    input.role == "updated"
}
`
	err = os.WriteFile(policyPath, []byte(newPolicyContent), 0644)
	require.NoError(t, err)

	// Manually trigger update cycle
	err = manager.RunUpdateCycle(ctx)
	require.NoError(t, err)

	// Verify revision changed
	assert.NotEqual(t, initialRevision, manager.Revision())

	// Verify new policy works
	evaluator := manager.Evaluator()
	result, err := evaluator.Evaluate(ctx, map[string]any{"role": "updated"})
	require.NoError(t, err)
	assert.True(t, result.Allow)
}
