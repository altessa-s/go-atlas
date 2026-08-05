// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package leadelect_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/leadelect"
	"github.com/altessa-s/go-atlas/data/leadelect/providers"
)

// mockProvider implements providers.Provider for testing.
type mockProvider struct {
	startFn   func(ctx context.Context, cfg providers.Config) error
	stopFn    func(ctx context.Context) error
	leaderID  string
	isLeader  bool
	nodeID    string
	isRunning bool
	fence     uint64
}

func (m *mockProvider) Start(ctx context.Context, cfg providers.Config) error {
	if m.startFn != nil {
		return m.startFn(ctx, cfg)
	}
	return nil
}

func (m *mockProvider) Stop(ctx context.Context) error {
	if m.stopFn != nil {
		return m.stopFn(ctx)
	}
	return nil
}

func (m *mockProvider) LeaderId(_ context.Context) (string, error) { return m.leaderID, nil }
func (m *mockProvider) IsLeader() bool                             { return m.isLeader }
func (m *mockProvider) Fence() uint64                              { return m.fence }
func (m *mockProvider) NodeId() string                             { return m.nodeID }
func (m *mockProvider) IsRunning() bool                            { return m.isRunning }

func TestNew(t *testing.T) {
	prov := &mockProvider{nodeID: "node1"}

	le := leadelect.New(prov, "election", "node1", leadelect.WithTTL(10*time.Second))
	require.NotNil(t, le)
}

func TestLeader_NodeId(t *testing.T) {
	prov := &mockProvider{nodeID: "node-42"}
	le := leadelect.New(prov, "", "")

	require.Equal(t, "node-42", le.NodeId())
}

func TestLeader_IsLeader(t *testing.T) {
	prov := &mockProvider{isLeader: true}
	le := leadelect.New(prov, "", "")

	require.True(t, le.IsLeader())
}

func TestLeader_Fence(t *testing.T) {
	prov := &mockProvider{isLeader: true, fence: 99}
	le := leadelect.New(prov, "", "")

	require.Equal(t, uint64(99), le.Fence(), "Fence must delegate to the provider")
}

func TestLeader_LeaderId(t *testing.T) {
	prov := &mockProvider{leaderID: "leader-1"}
	le := leadelect.New(prov, "", "")
	ctx := t.Context()

	id, err := le.LeaderId(ctx)
	require.NoError(t, err)
	require.Equal(t, "leader-1", id)
}

func TestLeader_IsRunning_BeforeStart(t *testing.T) {
	prov := &mockProvider{}
	le := leadelect.New(prov, "", "")

	require.False(t, le.IsRunning(), "IsRunning() should be false before Start()")
}

func TestLeader_Start_SetsRunning(t *testing.T) {
	prov := &mockProvider{}
	le := leadelect.New(prov, "test", "n1", leadelect.WithTTL(time.Second))
	ctx := t.Context()

	err := le.Start(ctx)
	require.NoError(t, err)
	defer le.Stop(ctx) //nolint:errcheck

	require.True(t, le.IsRunning(), "IsRunning() should be true after Start()")
}

func TestLeader_Stop_ClearsRunning(t *testing.T) {
	prov := &mockProvider{}
	le := leadelect.New(prov, "test", "n1", leadelect.WithTTL(time.Second))
	ctx := t.Context()

	_ = le.Start(ctx)
	_ = le.Stop(ctx)

	require.False(t, le.IsRunning(), "IsRunning() should be false after Stop()")
}

func TestLeader_Stop_Idempotent(t *testing.T) {
	prov := &mockProvider{}
	le := leadelect.New(prov, "", "")
	ctx := t.Context()

	_ = le.Start(ctx)
	_ = le.Stop(ctx)
	err := le.Stop(ctx)
	require.NoError(t, err)
}

func TestLeader_Start_Idempotent(t *testing.T) {
	var startCount atomic.Int32
	prov := &mockProvider{
		startFn: func(_ context.Context, _ providers.Config) error {
			startCount.Add(1)
			return nil
		},
	}
	le := leadelect.New(prov, "test", "n1", leadelect.WithTTL(time.Second))
	ctx := t.Context()

	_ = le.Start(ctx)
	defer le.Stop(ctx) //nolint:errcheck
	_ = le.Start(ctx)  // second call should be no-op

	require.Equal(t, int32(1), startCount.Load())
}

func TestLeader_RegisterCallbacks(t *testing.T) {
	prov := &mockProvider{}
	le := leadelect.New(prov, "", "")

	// Should not panic
	le.RegisterOnLeaderLost(func(_ context.Context, _ leadelect.LeaderElector) {})
	le.RegisterOnBecomesLeader(func(_ context.Context, _ leadelect.LeaderElector) {})
}

func TestLeader_WithHandlerTimeout(t *testing.T) {
	prov := &mockProvider{}
	le := leadelect.New(prov, "", "", leadelect.WithHandlerTimeout(5*time.Second))
	require.NotNil(t, le)
}
