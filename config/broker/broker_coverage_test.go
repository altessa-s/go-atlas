// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package brokerconfig_test

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	brokerconfig "github.com/altessa-s/go-atlas/config/broker"
	natsconfig "github.com/altessa-s/go-atlas/config/nats"
	validation "github.com/go-ozzo/ozzo-validation/v4"
)

func brokerFieldErrors(t *testing.T, err error) validation.Errors {
	t.Helper()
	verrs, ok := errors.AsType[validation.Errors](err)
	require.True(t, ok, "want validation.Errors, got %v", err)
	return verrs
}

func TestDefault(t *testing.T) {
	t.Parallel()

	c := brokerconfig.Default()
	require.Equal(t, brokerconfig.ProviderNATS, c.Provider)
	require.True(t, c.InProgress.Enabled)
	require.True(t, c.Outbox.Enabled)
	require.Positive(t, c.Outbox.MessagesBatchSize)
	require.NotNil(t, c.NATS, "the nats provider requires its section")
	require.NoError(t, c.Validate())

	// Default matches the default tags read by go-nomos/loader.
	o := c.Outbox
	require.Equal(t, "@every 2s", o.DispatchSchedule)
	require.Equal(t, "@every 10m", o.CleanupSchedule)
	require.Equal(t, "@every 11s", o.UnlockSchedule)
	require.Equal(t, "@every 30s", o.StatsSchedule)
	require.Equal(t, "@every 11s", o.ExpireSchedule)
	require.Equal(t, time.Second, o.RetryBaseDelay)
	require.Equal(t, 5*time.Minute, o.RetryMaxDelay)
	require.Equal(t, 40*time.Second, o.MaxLockTime)
	require.Equal(t, 1<<20, o.MaxPayloadBytes)
	require.Equal(t, []string{"outbox-dispatch", "outbox-unlock", "outbox-expire", "outbox-cleanup", "outbox-stats"},
		[]string{o.DispatchTaskID, o.UnlockTaskID, o.ExpireTaskID, o.CleanupTaskID, o.StatsTaskID})

	c.NATS = nil
	require.Contains(t, brokerFieldErrors(t, c.Validate()), "NATS", "nats section is required for the nats provider")
}

func TestConfig_Validate(t *testing.T) {
	t.Parallel()

	nats := natsconfig.Default()

	unknown := brokerconfig.Default()
	unknown.NATS = &nats
	unknown.Outbox.Enabled = false
	unknown.Provider = "kafka"
	require.Contains(t, brokerFieldErrors(t, unknown.Validate()), "Provider")

	badNested := brokerconfig.Default()
	badNested.NATS = &nats
	badNested.InProgress.TickSchedule = ""
	badNested.Outbox.MessagesBatchSize = 0
	verrs := brokerFieldErrors(t, badNested.Validate())
	require.Contains(t, verrs, "InProgress")
	require.Contains(t, verrs, "Outbox")
}
