// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package secrets_test

import (
	"testing"

	"github.com/stretchr/testify/require"

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
			require.Equal(t, tt.expected, tt.et.String())
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
			require.Equal(t, tt.expected, tt.policy.String())
		})
	}
}

func TestFilterByKeys(t *testing.T) {
	t.Run("with keys", func(t *testing.T) {
		filter := secrets.FilterByKeys[string]("a", "b")
		require.NotNil(t, filter)
		require.True(t, filter(secrets.WatchEvent[string]{Key: "a"}))
		require.False(t, filter(secrets.WatchEvent[string]{Key: "c"}))
	})

	t.Run("empty returns nil", func(t *testing.T) {
		require.Nil(t, secrets.FilterByKeys[string]())
	})
}

func TestFilterByEventTypes(t *testing.T) {
	t.Run("with types", func(t *testing.T) {
		filter := secrets.FilterByEventTypes[string](secrets.EventTypeCreated)
		require.NotNil(t, filter)
		require.True(t, filter(secrets.WatchEvent[string]{Type: secrets.EventTypeCreated}))
		require.False(t, filter(secrets.WatchEvent[string]{Type: secrets.EventTypeDeleted}))
	})

	t.Run("empty returns nil", func(t *testing.T) {
		require.Nil(t, secrets.FilterByEventTypes[string]())
	})
}

func TestFilterBySource(t *testing.T) {
	t.Run("with sources", func(t *testing.T) {
		filter := secrets.FilterBySource[string]("src1")
		require.NotNil(t, filter)
		require.True(t, filter(secrets.WatchEvent[string]{Source: "src1"}))
		require.False(t, filter(secrets.WatchEvent[string]{Source: "other"}))
	})

	t.Run("empty returns nil", func(t *testing.T) {
		require.Nil(t, secrets.FilterBySource[string]())
	})
}
