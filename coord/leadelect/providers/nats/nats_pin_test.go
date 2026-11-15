// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package nats_test

import (
	"strings"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/coord/leadelect/providers"
	"github.com/altessa-s/go-atlas/internal/testhelpers/servers/natstest"

	lenats "github.com/altessa-s/go-atlas/coord/leadelect/providers/nats"
)

// TestCamping_TickPin drives the renewal ticker: the leader keeps renewing,
// a follower keeps failing to acquire, and the leader loses leadership once
// the broker is gone and renewal and re-acquisition both fail.
func TestCamping_TickPin(t *testing.T) {
	t.Parallel()
	ns := natstest.StartNATSServer(t)
	bucket := strings.ReplaceAll(t.Name(), "/", "-")

	start := func(nodeID string) (p *lenats.Provider, became, lost chan struct{}) {
		p, err := lenats.New(t.Context(), natstest.ConnectNATS(t, ns), lenats.WithBucket(bucket), lenats.WithRenewRatio(0.1))
		require.NoError(t, err)
		became, lost = make(chan struct{}, 1), make(chan struct{}, 1)
		require.NoError(t, p.Start(t.Context(), providers.Config{
			Key:      "tick",
			TTL:      time.Second,
			NodeId:   nodeID,
			LostCh:   lost,
			BecameCh: became,
			StopCh:   make(chan struct{}, 1),
		}))
		return p, became, lost
	}

	leader, became, lost := start("node-1")
	select {
	case <-became:
	case <-time.After(3 * time.Second):
		t.Fatal("node-1 did not become leader")
	}
	_, js := natstest.ConnectJetStream(t, ns)
	kv, err := js.KeyValue(t.Context(), bucket)
	require.NoError(t, err)
	updates, err := kv.WatchAll(t.Context(), jetstream.UpdatesOnly())
	require.NoError(t, err)
	t.Cleanup(func() { _ = updates.Stop() })

	follower, _, _ := start("node-2")

	// Several renewal ticks: each renewal is a KV update of the lease. The
	// leader keeps the lease, the follower does not get it.
	for range 3 {
		select {
		case entry := <-updates.Updates():
			require.Equal(t, "node-1", string(entry.Value()), "only the leader renews the lease")
		case <-time.After(5 * time.Second):
			t.Fatal("the leader did not renew its lease")
		}
	}
	require.True(t, leader.IsLeader())
	require.False(t, follower.IsLeader())

	ns.Shutdown()
	select {
	case <-lost:
	case <-time.After(30 * time.Second):
		t.Fatal("leadership was not lost after the broker went away")
	}
	require.False(t, leader.IsLeader())
}
