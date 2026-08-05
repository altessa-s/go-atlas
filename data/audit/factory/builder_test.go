// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/config"
	"github.com/altessa-s/go-atlas/data/audit"
	"github.com/altessa-s/go-atlas/data/audit/factory"

	coreruntime "github.com/altessa-s/go-atlas/core/runtime"
)

// stubDispatcher stands in for an externally-owned dispatcher so a test can
// tell "the builder used what I gave it" from "the builder built its own".
type stubDispatcher struct{ submitted int }

func (s *stubDispatcher) Submit(*audit.Event) bool { s.submitted++; return true }
func (s *stubDispatcher) Dropped() int64           { return 0 }

func memoryConfig() *config.Audit {
	return &config.Audit{
		Enabled: true,
		Storage: config.AuditStorage{Type: config.AuditStorageTypeMemory},
		Dispatch: config.Dispatch{
			BufferSize:    16,
			BatchSize:     4,
			FlushInterval: 10 * time.Millisecond,
			Workers:       1,
			RetryAttempts: 1,
		},
		ShutdownTimeout: 5 * time.Second,
	}
}

func TestBuild_RequiresConfig(t *testing.T) {
	t.Parallel()

	_, err := factory.New(nil).Build()
	require.Error(t, err)
}

func TestBuild_DisabledReturnsSentinel(t *testing.T) {
	t.Parallel()

	cfg := memoryConfig()
	cfg.Enabled = false

	_, err := factory.New(cfg).Build()
	require.ErrorIs(t, err, factory.ErrDisabled)
}

// The regression this whole mapping exists for: before it, `storage` was
// described, validated and documented while the builder demanded an injected
// dispatcher, so a configured backend produced neither an effect nor an error.
func TestBuild_CreatesDispatcherFromConfig(t *testing.T) {
	t.Parallel()

	var hooks coreruntime.HookGroup

	auditor, err := factory.New(memoryConfig()).UseShutdownHooks(&hooks).Build()
	require.NoError(t, err)
	require.NotNil(t, auditor)

	require.True(t, auditor.Emit(&audit.Event{
		Type:   audit.EventTypeBusinessEvent,
		Action: audit.ActionCreate,
		Actor:  audit.Actor{Type: audit.ActorTypeUser, ID: "user-1"},
		Result: audit.Result{Status: audit.ResultStatusSuccess},
	}))

	require.NoError(t, hooks.Shutdown(t.Context()))
}

func TestBuild_InjectedDispatcherWins(t *testing.T) {
	t.Parallel()

	stub := &stubDispatcher{}

	auditor, err := factory.New(memoryConfig()).UseDispatcher(stub).Build()
	require.NoError(t, err)

	require.True(t, auditor.Emit(&audit.Event{
		Type:   audit.EventTypeBusinessEvent,
		Action: audit.ActionCreate,
		Actor:  audit.Actor{Type: audit.ActorTypeUser, ID: "user-1"},
		Result: audit.Result{Status: audit.ResultStatusSuccess},
	}))
	require.Equal(t, 1, stub.submitted, "the injected dispatcher must receive the event")

	require.NoError(t, auditor.Shutdown(t.Context()))
}

func TestBuild_RejectsUnknownStorageType(t *testing.T) {
	t.Parallel()

	cfg := memoryConfig()
	cfg.Storage.Type = "elasticsearch"

	_, err := factory.New(cfg).Build()
	require.Error(t, err)
}

// Mongo storage needs a database; without one the failure must be explicit
// rather than a silent fallback to memory.
func TestBuild_MongoStorageRequiresDatabase(t *testing.T) {
	t.Parallel()

	cfg := memoryConfig()
	cfg.Storage = config.AuditStorage{
		Type:  config.AuditStorageTypeMongo,
		Mongo: &config.AuditStorageMongo{CollectionName: "audit_events"},
	}

	_, err := factory.New(cfg).Build()
	require.ErrorIs(t, err, factory.ErrMongoDatabaseRequired)
}

// WAL settings live under `dispatch`; the engine must actually receive them,
// which is observable through the segment directory being populated.
func TestBuild_HonorsDispatchWAL(t *testing.T) {
	t.Parallel()

	dir := filepath.Join(t.TempDir(), "wal")

	cfg := memoryConfig()
	cfg.Dispatch.WAL = &config.WAL{
		Enabled:         true,
		Dir:             dir,
		MaxSegmentBytes: 1 << 20,
		MaxBytes:        1 << 22,
		FsyncInterval:   5 * time.Millisecond,
	}

	var hooks coreruntime.HookGroup

	auditor, err := factory.New(cfg).UseShutdownHooks(&hooks).Build()
	require.NoError(t, err)

	require.True(t, auditor.Emit(&audit.Event{
		Type:   audit.EventTypeBusinessEvent,
		Action: audit.ActionCreate,
		Actor:  audit.Actor{Type: audit.ActorTypeUser, ID: "user-wal"},
		Result: audit.Result{Status: audit.ResultStatusSuccess},
	}))

	require.NoError(t, hooks.Shutdown(t.Context()))
	require.DirExists(t, dir, "WAL config must reach the engine")
}

// A builder-owned engine has no other owner, so the builder must register its
// shutdown — otherwise the workers outlive the subsystem.
func TestBuild_RegistersEngineShutdownInScope(t *testing.T) {
	t.Parallel()

	var hooks coreruntime.HookGroup

	_, err := factory.New(memoryConfig()).UseShutdownHooks(&hooks).Build()
	require.NoError(t, err)

	// Engine shutdown plus the auditor's own hook.
	require.NoError(t, hooks.Shutdown(t.Context()))
}

// An injected dispatcher belongs to the caller; the builder must not schedule
// a shutdown for something it does not own.
func TestBuild_InjectedDispatcherIsNotShutDownByBuilder(t *testing.T) {
	t.Parallel()

	var hooks coreruntime.HookGroup

	auditor, err := factory.New(memoryConfig()).
		UseDispatcher(&stubDispatcher{}).
		UseShutdownHooks(&hooks).
		Build()
	require.NoError(t, err)

	require.NoError(t, hooks.Shutdown(t.Context()))

	// Only the auditor was in the scope, and stopping it stops emission.
	require.False(t, auditor.Emit(&audit.Event{
		Type:   audit.EventTypeBusinessEvent,
		Action: audit.ActionCreate,
		Actor:  audit.Actor{Type: audit.ActorTypeUser, ID: "user-1"},
		Result: audit.Result{Status: audit.ResultStatusSuccess},
	}))
}
