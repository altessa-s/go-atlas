// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package outboxconfig_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-nomos/loader"

	"github.com/altessa-s/go-atlas/reliability/outbox"

	outboxconfig "github.com/altessa-s/go-atlas/config/outbox"
)

// TestDefaultTaskIDs_RoundTripThroughLoader keeps the YAML defaults in sync
// with the runtime scheduler task IDs.
func TestDefaultTaskIDs_RoundTripThroughLoader(t *testing.T) {
	t.Parallel()

	type wrapper struct {
		Outbox outboxconfig.Config `yaml:"outbox"`
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
	require.Equal(t, outbox.DefaultStatsTaskID, cfg.Outbox.StatsTaskID)
}

func TestValidate_RejectsEmptyTaskID(t *testing.T) {
	t.Parallel()

	type wrapper struct {
		Outbox outboxconfig.Config `yaml:"outbox"`
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
