// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package brokerconfig

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// See config/validate_min_test.go: each case zeroes one Min-guarded field of
// an otherwise valid configuration and asserts that Validate rejects it.
func TestValidate_ZeroRejectedByMinGuardedFields(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		validate func() error
	}{
		{"InProgress.MaxEntries", func() error {
			c := validInProgress()
			c.MaxEntries = 0
			return c.Validate()
		}},
		{"Outbox.FetchTimeout", func() error {
			c := validOutbox()
			c.FetchTimeout = 0
			return c.Validate()
		}},
		{"Outbox.HandleTimeout", func() error {
			c := validOutbox()
			c.HandleTimeout = 0
			return c.Validate()
		}},
		{"Outbox.UpdateTimeout", func() error {
			c := validOutbox()
			c.UpdateTimeout = 0
			return c.Validate()
		}},
		{"Outbox.MessagesBatchSize", func() error {
			c := validOutbox()
			c.MessagesBatchSize = 0
			return c.Validate()
		}},
		{"Outbox.RetryMaxAttempts", func() error {
			c := validOutbox()
			c.RetryMaxAttempts = 0
			return c.Validate()
		}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Error(t, tc.validate(), "zero value must be rejected")
		})
	}
}

// The valid* helpers must themselves validate.
func TestValidate_ZeroRejectedByMinGuardedFields_BaselinesAreValid(t *testing.T) {
	t.Parallel()

	require.NoError(t, func() error { c := validInProgress(); return c.Validate() }())
	require.NoError(t, func() error { c := validOutbox(); return c.Validate() }())
}

func validInProgress() InProgress {
	return InProgress{
		Enabled:                  true,
		TickSchedule:             defaultInProgressTickSchedule,
		DefaultHeartbeatInterval: defaultInProgressHeartbeatInterval,
		MaxEntries:               defaultInProgressMaxEntries,
		Metrics: InProgressMetrics{
			Enabled: defaultInProgressMetricsEnabled,
			Prefix:  defaultInProgressMetricsPrefix,
		},
	}
}

func validOutbox() Outbox {
	return Outbox{
		Enabled:           defaultOutboxEnabled,
		DispatchSchedule:  "@every 2s",
		FetchTimeout:      defaultOutboxFetchTimeout,
		HandleTimeout:     defaultOutboxHandleTimeout,
		UpdateTimeout:     defaultOutboxUpdateTimeout,
		CleanupSchedule:   "@every 10m",
		UnlockSchedule:    "@every 11s",
		MessagesBatchSize: defaultOutboxMessagesBatchSize,
		RetryMaxAttempts:  defaultOutboxRetryMaxAttempts,
		RetryBaseDelay:    defaultOutboxRetryBaseDelay,
		RetryMaxDelay:     defaultOutboxRetryMaxDelay,
		MaxLockTime:       defaultOutboxMaxLockTime,
		MaxPayloadBytes:   defaultOutboxMaxPayloadBytes,
		ExpireSchedule:    "@every 11s",
		StatsSchedule:     "@every 30s",
		DispatchTaskID:    "outbox-dispatch",
		UnlockTaskID:      "outbox-unlock",
		ExpireTaskID:      "outbox-expire",
		CleanupTaskID:     "outbox-cleanup",
		StatsTaskID:       "outbox-stats",
	}
}
