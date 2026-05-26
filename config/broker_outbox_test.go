// Copyright 2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package config_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/config"
	"github.com/altessa-s/go-atlas/config/loader"
	"github.com/altessa-s/go-atlas/data/outbox"
)

// TestOutbox_DefaultTaskIDs_RoundTripThroughLoader pins the YAML
// `default:` tags on the new DispatchTaskID / UnlockTaskID /
// ExpireTaskID / CleanupTaskID fields. The string literals in the
// tags have to match the outbox.Default*TaskID constants — the
// config package can't import data/outbox without closing an import
// cycle, so the values are duplicated as struct-tag literals and
// kept in sync by this test instead.
//
// Without this guard, a future rename of (say) outbox.DefaultDispatchTaskID
// from "outbox-dispatch" to "outbox-dispatch-v2" would silently drift
// the YAML-resolved default ("outbox-dispatch") away from the runtime
// constant ("outbox-dispatch-v2"). YAML-driven services would then
// register a task with the *old* ID even though every other code path
// in the outbox package uses the new one — a confusing partial-rename
// bug that costs an oncall ticket to diagnose.
func TestOutbox_DefaultTaskIDs_RoundTripThroughLoader(t *testing.T) {
	t.Parallel()

	type wrapper struct {
		Outbox config.Outbox `yaml:"outbox"`
	}

	cfg := &wrapper{}
	_, err := loader.New(nil).Load(cfg)
	require.NoError(t, err)

	require.Equal(t, outbox.DefaultDispatchTaskID, cfg.Outbox.DispatchTaskID,
		"YAML default for dispatchTaskID must equal outbox.DefaultDispatchTaskID")
	require.Equal(t, outbox.DefaultUnlockTaskID, cfg.Outbox.UnlockTaskID,
		"YAML default for unlockTaskID must equal outbox.DefaultUnlockTaskID")
	require.Equal(t, outbox.DefaultExpireTaskID, cfg.Outbox.ExpireTaskID,
		"YAML default for expireTaskID must equal outbox.DefaultExpireTaskID")
	require.Equal(t, outbox.DefaultCleanupTaskID, cfg.Outbox.CleanupTaskID,
		"YAML default for cleanupTaskID must equal outbox.DefaultCleanupTaskID")
}

// TestOutbox_Validate_RejectsEmptyTaskID guards the validation rule:
// the YAML loader's default-tag fills in the standard IDs, but a
// programmatic caller building a config.Outbox{} struct by hand
// would leave the fields empty. Catching it at Validate() means the
// loader pipeline (Validate runs after defaults + env + secret
// expansion) surfaces the misconfig before the outbox is even
// constructed.
func TestOutbox_Validate_RejectsEmptyTaskID(t *testing.T) {
	t.Parallel()

	type wrapper struct {
		Outbox config.Outbox `yaml:"outbox"`
	}

	// Load defaults first so every other Required field is populated,
	// then zero out only the field under test to isolate the failure.
	cfg := &wrapper{}
	_, err := loader.New(nil).Load(cfg)
	require.NoError(t, err)

	cfg.Outbox.DispatchTaskID = ""
	require.Error(t, cfg.Outbox.Validate(),
		"empty dispatchTaskID must be rejected — the scheduler upserts by ID and would otherwise fail with a generic 'task ID cannot be empty' downstream")
}
