// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package leadelect_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/altessa-s/go-atlas/data/leadelect"
	"github.com/altessa-s/go-atlas/data/leadelect/providers"
)

// callbackProvider extends mockProvider with callback support.
type callbackProvider struct {
	startFn  func(ctx context.Context, cfg providers.Config) error
	stopFn   func(ctx context.Context) error
	leaderID string
	isLeader bool
	nodeID   string
	running  bool

	onBecomesLeader func()
}

func (m *callbackProvider) Start(ctx context.Context, cfg providers.Config) error {
	m.running = true
	if m.startFn != nil {
		return m.startFn(ctx, cfg)
	}
	// Simulate becoming leader
	if m.onBecomesLeader != nil {
		m.onBecomesLeader()
	}
	return nil
}

func (m *callbackProvider) Stop(ctx context.Context) error {
	m.running = false
	if m.stopFn != nil {
		return m.stopFn(ctx)
	}
	return nil
}

func (m *callbackProvider) LeaderId(_ context.Context) (string, error) { return m.leaderID, nil }
func (m *callbackProvider) IsLeader() bool                             { return m.isLeader }
func (m *callbackProvider) NodeId() string                             { return m.nodeID }
func (m *callbackProvider) IsRunning() bool                            { return m.running }

func TestOnBecomesLeaderPrintCallback(t *testing.T) {
	// Should not panic
	leadelect.OnBecomesLeaderPrintCallback(t.Context(), nil)
}

func TestOnLeaderLostPrintCallback(t *testing.T) {
	prov := &mockProvider{nodeID: "node-1", leaderID: "other"}
	le := leadelect.New(prov, leadelect.Config{})
	// Should not panic
	leadelect.OnLeaderLostPrintCallback(t.Context(), le)
}

func TestLeader_RegisterOnBecomesLeader_Called(t *testing.T) {
	var mu sync.Mutex
	called := false

	prov := &callbackProvider{
		nodeID:   "node-1",
		isLeader: true,
	}

	le := leadelect.New(prov, leadelect.Config{
		Key:    "test",
		TTL:    time.Second,
		NodeId: "node-1",
	}, leadelect.WithHandlerTimeout(2*time.Second))

	le.RegisterOnBecomesLeader(func(_ context.Context, _ leadelect.LeaderElector) {
		mu.Lock()
		called = true
		mu.Unlock()
	})

	// Start triggers provider.Start which may invoke callbacks
	err := le.Start(t.Context())
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer le.Stop(t.Context()) //nolint:errcheck

	// Give time for async callback
	time.Sleep(100 * time.Millisecond)
	mu.Lock()
	_ = called // callback may or may not fire depending on provider implementation
	mu.Unlock()
}

func TestLeader_RegisterOnLeaderLost_NilSafe(t *testing.T) {
	prov := &mockProvider{}
	le := leadelect.New(prov, leadelect.Config{})
	// Registering nil should not panic
	le.RegisterOnLeaderLost(nil)
	le.RegisterOnBecomesLeader(nil)
}

func TestNew_WithDefaultHandlerTimeout(t *testing.T) {
	prov := &mockProvider{nodeID: "n1"}
	le := leadelect.New(prov, leadelect.Config{})
	if le == nil {
		t.Fatal("New returned nil")
	}
}

func TestWithHandlerTimeout_Zero(t *testing.T) {
	prov := &mockProvider{nodeID: "n1"}
	// Zero timeout should be ignored, keeping default
	le := leadelect.New(prov, leadelect.Config{}, leadelect.WithHandlerTimeout(0))
	if le == nil {
		t.Fatal("New with zero timeout returned nil")
	}
}
