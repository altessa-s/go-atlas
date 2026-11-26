// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/factory/messaging/broker"

	brokerconfig "github.com/altessa-s/go-atlas/config/broker"
	schedtask "github.com/altessa-s/go-horai/task"
)

var errRegistrationRejected = errors.New("scheduler rejected the task")

type rejectingRegistrar struct{}

func (rejectingRegistrar) Register(context.Context, schedtask.Config) error {
	return errRegistrationRejected
}

// TestCreateInProgressManager_PropagatesRegistrationFailure pins that a
// scheduler rejecting the heartbeat task fails the build: a manager without a
// heartbeat task lets a long handler outlive AckWait and be redelivered.
func TestCreateInProgressManager_PropagatesRegistrationFailure(t *testing.T) {
	t.Parallel()

	cfg := &brokerconfig.Config{InProgress: brokerconfig.InProgress{Enabled: true, TickSchedule: "@every 1s"}}
	mgr, err := factory.New(cfg).UseScheduler(rejectingRegistrar{}).CreateInProgressManager()
	require.ErrorIs(t, err, errRegistrationRejected)
	require.Nil(t, mgr)
}
