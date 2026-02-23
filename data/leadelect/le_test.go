// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package leadelect_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

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
func (m *mockProvider) NodeId() string                             { return m.nodeID }
func (m *mockProvider) IsRunning() bool                            { return m.isRunning }

func TestNew(t *testing.T) {
	prov := &mockProvider{nodeID: "node1"}
	cfg := leadelect.Config{Key: "election", TTL: 10 * time.Second, NodeId: "node1"}

	le := leadelect.New(prov, cfg)
	if le == nil {
		t.Fatal("New() returned nil")
	}
}

func TestLeader_NodeId(t *testing.T) {
	prov := &mockProvider{nodeID: "node-42"}
	le := leadelect.New(prov, leadelect.Config{})

	if got := le.NodeId(); got != "node-42" {
		t.Errorf("NodeId() = %q, want %q", got, "node-42")
	}
}

func TestLeader_IsLeader(t *testing.T) {
	prov := &mockProvider{isLeader: true}
	le := leadelect.New(prov, leadelect.Config{})

	if !le.IsLeader() {
		t.Error("IsLeader() = false, want true")
	}
}

func TestLeader_LeaderId(t *testing.T) {
	prov := &mockProvider{leaderID: "leader-1"}
	le := leadelect.New(prov, leadelect.Config{})
	ctx := t.Context()

	id, err := le.LeaderId(ctx)
	if err != nil {
		t.Fatalf("LeaderId() error: %v", err)
	}
	if id != "leader-1" {
		t.Errorf("LeaderId() = %q, want %q", id, "leader-1")
	}
}

func TestLeader_IsRunning_BeforeStart(t *testing.T) {
	prov := &mockProvider{}
	le := leadelect.New(prov, leadelect.Config{})

	if le.IsRunning() {
		t.Error("IsRunning() should be false before Start()")
	}
}

func TestLeader_Start_SetsRunning(t *testing.T) {
	prov := &mockProvider{}
	le := leadelect.New(prov, leadelect.Config{Key: "test", TTL: time.Second, NodeId: "n1"})
	ctx := t.Context()

	err := le.Start(ctx)
	if err != nil {
		t.Fatalf("Start() error: %v", err)
	}
	defer le.Stop(ctx) //nolint:errcheck

	if !le.IsRunning() {
		t.Error("IsRunning() should be true after Start()")
	}
}

func TestLeader_Stop_ClearsRunning(t *testing.T) {
	prov := &mockProvider{}
	le := leadelect.New(prov, leadelect.Config{Key: "test", TTL: time.Second, NodeId: "n1"})
	ctx := t.Context()

	_ = le.Start(ctx)
	_ = le.Stop(ctx)

	if le.IsRunning() {
		t.Error("IsRunning() should be false after Stop()")
	}
}

func TestLeader_Stop_Idempotent(t *testing.T) {
	prov := &mockProvider{}
	le := leadelect.New(prov, leadelect.Config{})
	ctx := t.Context()

	_ = le.Start(ctx)
	_ = le.Stop(ctx)
	err := le.Stop(ctx)
	if err != nil {
		t.Errorf("second Stop() should return nil: %v", err)
	}
}

func TestLeader_Start_Idempotent(t *testing.T) {
	var startCount atomic.Int32
	prov := &mockProvider{
		startFn: func(_ context.Context, _ providers.Config) error {
			startCount.Add(1)
			return nil
		},
	}
	le := leadelect.New(prov, leadelect.Config{Key: "test", TTL: time.Second, NodeId: "n1"})
	ctx := t.Context()

	_ = le.Start(ctx)
	defer le.Stop(ctx) //nolint:errcheck
	_ = le.Start(ctx)  // second call should be no-op

	if got := startCount.Load(); got != 1 {
		t.Errorf("Start() called provider %d times, want 1", got)
	}
}

func TestLeader_RegisterCallbacks(t *testing.T) {
	prov := &mockProvider{}
	le := leadelect.New(prov, leadelect.Config{})

	// Should not panic
	le.RegisterOnLeaderLost(func(_ context.Context, _ leadelect.LeaderElector) {})
	le.RegisterOnBecomesLeader(func(_ context.Context, _ leadelect.LeaderElector) {})
}

func TestLeader_WithHandlerTimeout(t *testing.T) {
	prov := &mockProvider{}
	le := leadelect.New(prov, leadelect.Config{}, leadelect.WithHandlerTimeout(5*time.Second))
	if le == nil {
		t.Fatal("New() with WithHandlerTimeout returned nil")
	}
}

func TestConfig_Fields(t *testing.T) {
	cfg := leadelect.Config{
		Key:    "election-key",
		TTL:    30 * time.Second,
		NodeId: "node-1",
	}

	if cfg.Key != "election-key" {
		t.Errorf("Key = %q, want %q", cfg.Key, "election-key")
	}
	if cfg.TTL != 30*time.Second {
		t.Errorf("TTL = %v, want %v", cfg.TTL, 30*time.Second)
	}
	if cfg.NodeId != "node-1" {
		t.Errorf("NodeId = %q, want %q", cfg.NodeId, "node-1")
	}
}
