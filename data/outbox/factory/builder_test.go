// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/config"
	"github.com/altessa-s/go-atlas/internal/testhelpers"
)

// Use the internal assembly boundary to test registration without a database.
func TestBuildPropagatesRegistrationFailureAndRetainsRetryableOutbox(t *testing.T) {
	t.Parallel()
	boom := errors.New("registrar unavailable")
	reg := &testhelpers.MockTaskRegistrar{Err: boom}
	b := New(&config.Outbox{Enabled: true, DispatchSchedule: "@every 1m",
		DispatchTaskID: "dispatch", UnlockTaskID: "unlock", ExpireTaskID: "expire", CleanupTaskID: "cleanup", StatsTaskID: "stats",
	}).UseScheduler(reg)
	ob, err := b.createOutboxWithStore(nil, nil)
	require.ErrorIs(t, err, boom)
	require.NotNil(t, ob)
	reg.Err = nil
	require.NoError(t, ob.RegisterTasks(t.Context()))
}
