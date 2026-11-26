// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package recovery_test

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/testhelpers/servers/natstest"
	"github.com/altessa-s/go-atlas/messaging/broker/providers/nats/recovery"

	natsprovider "github.com/altessa-s/go-atlas/messaging/broker/providers/nats"
	schedtask "github.com/altessa-s/go-horai/task"
)

type recordingRegistrar struct {
	mu  sync.Mutex
	ids []string
}

func (r *recordingRegistrar) Register(_ context.Context, cfg schedtask.Config) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ids = append(r.ids, cfg.ID)
	return nil
}

// TestManager_TaskIDsAreUniquePerInstance verifies that managers with default
// options register distinct scheduler task IDs (the scheduler keys tasks by
// ID, so a shared ID leaves only the last manager running), and that an
// explicit ID is used as given.
func TestManager_TaskIDsAreUniquePerInstance(t *testing.T) {
	t.Parallel()

	ns := natstest.StartNATSServer(t)
	conn, _ := natstest.ConnectJetStream(t, ns)
	provider, err := natsprovider.New(conn)
	require.NoError(t, err)

	reg := &recordingRegistrar{}
	newManager := func(extra ...recovery.Option) {
		t.Helper()
		opts := append([]recovery.Option{
			recovery.WithScheduler(reg),
			recovery.WithHealthCheckSchedule("@every 1m"),
			recovery.WithStaleRecoveryCleanupSchedule("@every 1m"),
		}, extra...)
		m, err := recovery.New(provider, opts...)
		require.NoError(t, err)
		t.Cleanup(m.Close)
	}
	newManager()
	newManager()
	newManager(recovery.WithHealthCheckTaskID("orders-health"), recovery.WithStaleCleanupTaskID("orders-stale"))

	require.Len(t, reg.ids, 6)
	require.Len(t, map[string]struct{}{reg.ids[0]: {}, reg.ids[1]: {}, reg.ids[2]: {}, reg.ids[3]: {}}, 4,
		"default task IDs of two managers must not collide: %v", reg.ids[:4])
	require.Equal(t, []string{"orders-health", "orders-stale"}, reg.ids[4:])
}
