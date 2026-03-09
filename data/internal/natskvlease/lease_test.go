// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package natskvlease_test

import (
	"testing"

	"github.com/altessa-s/go-atlas/data/internal/natskvlease"
)

func TestLeaseConfig_Fields(t *testing.T) {
	cfg := natskvlease.LeaseConfig{
		Key:   "my-lease",
		Value: []byte("node-1"),
	}
	if cfg.Key != "my-lease" {
		t.Errorf("Key = %q, want %q", cfg.Key, "my-lease")
	}
	if string(cfg.Value) != "node-1" {
		t.Errorf("Value = %q, want %q", cfg.Value, "node-1")
	}
}

func TestLeaseCallbacks_NilSafe(t *testing.T) {
	// Verify zero-value callbacks don't panic when checked
	cb := natskvlease.LeaseCallbacks{}
	if cb.OnAcquired != nil {
		t.Error("OnAcquired should be nil by default")
	}
	if cb.OnLost != nil {
		t.Error("OnLost should be nil by default")
	}
	if cb.OnRenewed != nil {
		t.Error("OnRenewed should be nil by default")
	}
	if cb.OnReleased != nil {
		t.Error("OnReleased should be nil by default")
	}
}
