// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package nats_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/internal/natskey/natskeytest"
	"github.com/altessa-s/go-atlas/data/leadelect/providers"
	"github.com/altessa-s/go-atlas/internal/testhelpers"

	lenats "github.com/altessa-s/go-atlas/data/leadelect/providers/nats"
)

func electionConfig(key, node string) providers.Config {
	return providers.Config{
		Key:      key,
		TTL:      2 * time.Second,
		NodeId:   node,
		LostCh:   make(chan struct{}, 1),
		BecameCh: make(chan struct{}, 1),
		StopCh:   make(chan struct{}, 1),
	}
}

// TestProvider_KeyNATSRejectsVerbatimHandsOver elects a leader on a key the
// server rejects as it is, then has it resign: the follower learns of it
// through its watch on the same key and takes over.
func TestProvider_KeyNATSRejectsVerbatimHandsOver(t *testing.T) {
	t.Parallel()
	ns := testhelpers.StartNATSServer(t)
	ctx := t.Context()
	key := "election:scheduler/eu.ü"

	newProvider := func() *lenats.Provider {
		p, err := lenats.New(ctx, testhelpers.ConnectNATS(t, ns), lenats.WithBucket("handover"))
		require.NoError(t, err)
		t.Cleanup(func() { _ = p.Stop(context.Background()) })
		return p
	}
	first, second := newProvider(), newProvider()

	require.NoError(t, first.Start(ctx, electionConfig(key, "node-1")))
	require.Eventually(t, first.IsLeader, 3*time.Second, 20*time.Millisecond)
	require.NoError(t, second.Start(ctx, electionConfig(key, "node-2")))

	leader, err := second.LeaderId(ctx)
	require.NoError(t, err)
	require.Equal(t, "node-1", leader)
	require.False(t, second.IsLeader())

	require.NoError(t, first.Stop(ctx))
	require.Eventually(t, second.IsLeader, 3*time.Second, 20*time.Millisecond)
}

// TestProvider_KeysNATSRejectsVerbatimAreSeparateElections starts one
// election per key; every node wins its own.
func TestProvider_KeysNATSRejectsVerbatimAreSeparateElections(t *testing.T) {
	t.Parallel()
	ns := testhelpers.StartNATSServer(t)
	ctx := t.Context()

	provs := make([]*lenats.Provider, 0, len(natskeytest.Keys()))
	for _, key := range natskeytest.Keys() {
		p, err := lenats.New(ctx, testhelpers.ConnectNATS(t, ns), lenats.WithBucket("separate"))
		require.NoError(t, err)
		t.Cleanup(func() { _ = p.Stop(context.Background()) })
		require.NoError(t, p.Start(ctx, electionConfig(key, "node "+key)), key)
		provs = append(provs, p)
	}
	for i, p := range provs {
		require.Eventually(t, p.IsLeader, 3*time.Second, 20*time.Millisecond, natskeytest.Keys()[i])
	}
}

func TestProvider_Start_KeyTooLong(t *testing.T) {
	t.Parallel()
	provider := setupProvider(t)

	err := provider.Start(t.Context(), electionConfig(strings.Repeat(":", 2000), "node-1"))
	require.ErrorIs(t, err, lenats.ErrKeyTooLong)
	require.False(t, provider.IsRunning())
}
