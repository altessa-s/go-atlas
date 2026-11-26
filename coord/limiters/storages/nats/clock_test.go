// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package nats_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas-core/time/clock"
	"github.com/altessa-s/go-atlas/coord/limiters"
	"github.com/altessa-s/go-atlas/internal/testhelpers/servers/natstest"

	limitnats "github.com/altessa-s/go-atlas/coord/limiters/storages/nats"
)

func TestProvider_WindowUsesClock(t *testing.T) {
	t.Parallel()
	ns := natstest.StartNATSServer(t)
	_, js := natstest.ConnectJetStream(t, ns)
	epoch := time.Unix(1_000, 0)
	fake := clock.NewFake(epoch)
	p, err := limitnats.New(t.Context(), js, limitnats.WithBucket("clock"), limitnats.WithClock(fake))
	require.NoError(t, err)
	info, err := p.Allow(t.Context(), "clock", 1, time.Minute)
	require.NoError(t, err)
	require.Equal(t, epoch.Add(time.Minute).Unix(), info.Reset)
	_, err = p.Allow(t.Context(), "clock", 1, time.Minute)
	require.ErrorIs(t, err, limiters.ErrLimitExceeded)
	fake.Advance(time.Minute)
	_, err = p.Allow(t.Context(), "clock", 1, time.Minute)
	require.NoError(t, err)
}
