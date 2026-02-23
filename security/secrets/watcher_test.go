// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package secrets_test

import (
	"testing"

	"github.com/altessa-s/go-atlas/security/secrets"
)

func TestEventType_String(t *testing.T) {
	tests := []struct {
		name     string
		et       secrets.EventType
		expected string
	}{
		{"created", secrets.EventTypeCreated, "created"},
		{"updated", secrets.EventTypeUpdated, "updated"},
		{"deleted", secrets.EventTypeDeleted, "deleted"},
		{"error", secrets.EventTypeError, "error"},
		{"unknown", secrets.EventType(99), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.et.String(); got != tt.expected {
				t.Errorf("EventType.String() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestBufferOverflowPolicy_String(t *testing.T) {
	tests := []struct {
		name     string
		policy   secrets.BufferOverflowPolicy
		expected string
	}{
		{"drop newest", secrets.OverflowPolicyDropNewest, "drop_newest"},
		{"drop oldest", secrets.OverflowPolicyDropOldest, "drop_oldest"},
		{"block", secrets.OverflowPolicyBlock, "block"},
		{"unknown", secrets.BufferOverflowPolicy(99), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.policy.String(); got != tt.expected {
				t.Errorf("BufferOverflowPolicy.String() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestDefaultWatchOptions(t *testing.T) {
	opts := secrets.DefaultWatchOptions()
	if opts.BufferSize != 100 {
		t.Errorf("DefaultWatchOptions().BufferSize = %d, want 100", opts.BufferSize)
	}
}

func TestFilterByKeys(t *testing.T) {
	t.Run("with keys", func(t *testing.T) {
		filter := secrets.FilterByKeys[string]("a", "b")
		if filter == nil {
			t.Fatal("FilterByKeys() returned nil")
		}
		if !filter(secrets.WatchEvent[string]{Key: "a"}) {
			t.Error("expected Key=a to pass filter")
		}
		if filter(secrets.WatchEvent[string]{Key: "c"}) {
			t.Error("expected Key=c to be rejected")
		}
	})

	t.Run("empty returns nil", func(t *testing.T) {
		if secrets.FilterByKeys[string]() != nil {
			t.Error("expected nil filter for empty keys")
		}
	})
}

func TestFilterByEventTypes(t *testing.T) {
	t.Run("with types", func(t *testing.T) {
		filter := secrets.FilterByEventTypes[string](secrets.EventTypeCreated)
		if filter == nil {
			t.Fatal("FilterByEventTypes() returned nil")
		}
		if !filter(secrets.WatchEvent[string]{Type: secrets.EventTypeCreated}) {
			t.Error("expected Created to pass filter")
		}
		if filter(secrets.WatchEvent[string]{Type: secrets.EventTypeDeleted}) {
			t.Error("expected Deleted to be rejected")
		}
	})

	t.Run("empty returns nil", func(t *testing.T) {
		if secrets.FilterByEventTypes[string]() != nil {
			t.Error("expected nil filter for empty types")
		}
	})
}

func TestFilterBySource(t *testing.T) {
	t.Run("with sources", func(t *testing.T) {
		filter := secrets.FilterBySource[string]("src1")
		if filter == nil {
			t.Fatal("FilterBySource() returned nil")
		}
		if !filter(secrets.WatchEvent[string]{Source: "src1"}) {
			t.Error("expected Source=src1 to pass filter")
		}
		if filter(secrets.WatchEvent[string]{Source: "other"}) {
			t.Error("expected Source=other to be rejected")
		}
	})

	t.Run("empty returns nil", func(t *testing.T) {
		if secrets.FilterBySource[string]() != nil {
			t.Error("expected nil filter for empty sources")
		}
	})
}
