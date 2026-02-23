// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package nats_test

import (
	"strings"
	"testing"
	"time"

	"github.com/altessa-s/go-atlas/data/leadelect/providers"
	"github.com/altessa-s/go-atlas/internal/testhelpers"

	lenats "github.com/altessa-s/go-atlas/data/leadelect/providers/nats"
)

func setupProvider(tb testing.TB) *lenats.Provider {
	tb.Helper()
	ns := testhelpers.StartNATSServer(tb)
	nc := testhelpers.ConnectNATS(tb, ns)
	ctx := tb.Context()

	bucket := strings.ReplaceAll(tb.Name(), "/", "-")
	provider, err := lenats.New(ctx, nc, lenats.WithBucket(bucket))
	if err != nil {
		tb.Fatalf("failed to create NATS provider: %v", err)
	}
	return provider
}

func TestNew(t *testing.T) {
	provider := setupProvider(t)
	if provider == nil {
		t.Fatal("New() returned nil")
	}
}

func TestProvider_IsLeader_BeforeStart(t *testing.T) {
	provider := setupProvider(t)
	if provider.IsLeader() {
		t.Error("IsLeader() should be false before Start()")
	}
}

func TestProvider_IsRunning_BeforeStart(t *testing.T) {
	provider := setupProvider(t)
	if provider.IsRunning() {
		t.Error("IsRunning() should be false before Start()")
	}
}

func TestProvider_Start_Stop(t *testing.T) {
	provider := setupProvider(t)
	ctx := t.Context()

	cfg := providers.Config{
		Key:      "test-election",
		TTL:      5 * time.Second,
		NodeId:   "node-1",
		LostCh:   make(chan struct{}, 1),
		BecameCh: make(chan struct{}, 1),
		StopCh:   make(chan struct{}, 1),
	}

	err := provider.Start(ctx, cfg)
	if err != nil {
		t.Fatalf("Start() error: %v", err)
	}

	if !provider.IsRunning() {
		t.Error("IsRunning() should be true after Start()")
	}

	// Wait a bit for the camping loop to acquire leadership
	time.Sleep(500 * time.Millisecond)

	if !provider.IsLeader() {
		t.Error("expected to become leader (single node)")
	}

	if got := provider.NodeId(); got != "node-1" {
		t.Errorf("NodeId() = %q, want %q", got, "node-1")
	}

	leaderID, err := provider.LeaderId(ctx)
	if err != nil {
		t.Fatalf("LeaderId() error: %v", err)
	}
	if leaderID != "node-1" {
		t.Errorf("LeaderId() = %q, want %q", leaderID, "node-1")
	}

	err = provider.Stop(ctx)
	if err != nil {
		t.Fatalf("Stop() error: %v", err)
	}

	if provider.IsRunning() {
		t.Error("IsRunning() should be false after Stop()")
	}
}

func TestProvider_Start_Validations(t *testing.T) {
	tests := []struct {
		name string
		cfg  providers.Config
	}{
		{
			name: "empty key",
			cfg: providers.Config{
				Key:    "",
				TTL:    5 * time.Second,
				NodeId: "node-1",
			},
		},
		{
			name: "empty node ID",
			cfg: providers.Config{
				Key:    "election",
				TTL:    5 * time.Second,
				NodeId: "",
			},
		},
		{
			name: "TTL too small",
			cfg: providers.Config{
				Key:    "election",
				TTL:    500 * time.Millisecond,
				NodeId: "node-1",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := setupProvider(t)
			ctx := t.Context()

			err := provider.Start(ctx, tt.cfg)
			if err == nil {
				provider.Stop(ctx) //nolint:errcheck
				t.Error("Start() should return error for invalid config")
			}
		})
	}
}

func TestProvider_Start_DoubleStart(t *testing.T) {
	provider := setupProvider(t)
	ctx := t.Context()

	cfg := providers.Config{
		Key:      "test-election",
		TTL:      5 * time.Second,
		NodeId:   "node-1",
		LostCh:   make(chan struct{}, 1),
		BecameCh: make(chan struct{}, 1),
		StopCh:   make(chan struct{}, 1),
	}

	err := provider.Start(ctx, cfg)
	if err != nil {
		t.Fatalf("first Start() error: %v", err)
	}
	defer provider.Stop(ctx) //nolint:errcheck

	err = provider.Start(ctx, cfg)
	if err == nil {
		t.Error("second Start() should return error")
	}
}

func TestProvider_Stop_DoubleStop(t *testing.T) {
	provider := setupProvider(t)
	ctx := t.Context()

	cfg := providers.Config{
		Key:      "test-election",
		TTL:      5 * time.Second,
		NodeId:   "node-1",
		LostCh:   make(chan struct{}, 1),
		BecameCh: make(chan struct{}, 1),
		StopCh:   make(chan struct{}, 1),
	}

	_ = provider.Start(ctx, cfg)
	_ = provider.Stop(ctx)

	err := provider.Stop(ctx)
	if err == nil {
		t.Error("second Stop() should return error")
	}
}

func TestProvider_BecameCh_Notification(t *testing.T) {
	provider := setupProvider(t)
	ctx := t.Context()

	becameCh := make(chan struct{}, 1)
	cfg := providers.Config{
		Key:      "test-notify",
		TTL:      5 * time.Second,
		NodeId:   "node-1",
		LostCh:   make(chan struct{}, 1),
		BecameCh: becameCh,
		StopCh:   make(chan struct{}, 1),
	}

	err := provider.Start(ctx, cfg)
	if err != nil {
		t.Fatalf("Start() error: %v", err)
	}
	defer provider.Stop(ctx) //nolint:errcheck

	select {
	case <-becameCh:
		// Received notification
	case <-time.After(3 * time.Second):
		t.Error("timed out waiting for BecameCh notification")
	}
}
