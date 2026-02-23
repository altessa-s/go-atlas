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

func TestBucketConfig_Defaults(t *testing.T) {
	cfg := natskvlease.BucketConfig{
		Bucket: "test-bucket",
	}
	if cfg.Bucket != "test-bucket" {
		t.Errorf("Bucket = %q, want %q", cfg.Bucket, "test-bucket")
	}
	if cfg.TTL != 0 {
		t.Errorf("TTL should default to 0 (set by KVHelper)")
	}
}

func TestErrLeaseNotHeld(t *testing.T) {
	if natskvlease.ErrLeaseNotHeld == nil {
		t.Fatal("ErrLeaseNotHeld should not be nil")
	}
	if natskvlease.ErrLeaseNotHeld.Error() != "lease not held" {
		t.Errorf("ErrLeaseNotHeld = %q", natskvlease.ErrLeaseNotHeld.Error())
	}
}

func TestErrLeaseExists(t *testing.T) {
	if natskvlease.ErrLeaseExists == nil {
		t.Fatal("ErrLeaseExists should not be nil")
	}
}

func TestErrProviderClosed(t *testing.T) {
	if natskvlease.ErrProviderClosed == nil {
		t.Fatal("ErrProviderClosed should not be nil")
	}
}

func TestErrProviderStopped(t *testing.T) {
	if natskvlease.ErrProviderStopped == nil {
		t.Fatal("ErrProviderStopped should not be nil")
	}
}
