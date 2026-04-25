// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package outboxstore

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestDefaultOptions(t *testing.T) {
	opts := defaultOptions()
	require.Equal(t, DefaultCollectionName, opts.collectionName)
	require.Equal(t, DefaultIndexCreateTimeout, opts.indexTimeout)
	require.Equal(t, nil, opts.ctx)
}

func TestWithCollectionName_String(t *testing.T) {
	opts := newOptions(WithCollectionName("my_events"))
	require.Equal(t, "my_events", opts.collectionName)
}

func TestWithCollectionName_StringPtr(t *testing.T) {
	name := "ptr_events"
	opts := newOptions(WithCollectionName(&name))
	require.Equal(t, "ptr_events", opts.collectionName)
}

func TestWithCollectionName_NilPtr(t *testing.T) {
	opts := newOptions(WithCollectionName[*string](nil))
	require.Equal(t, DefaultCollectionName, opts.collectionName)
}

func TestWithCollectionName_Trimmed(t *testing.T) {
	opts := newOptions(WithCollectionName("  spaced  "))
	require.Equal(t, "spaced", opts.collectionName)
}

func TestWithContext(t *testing.T) {
	ctx := t.Context()
	opts := newOptions(WithContext(ctx))
	require.Equal(t, ctx, opts.ctx)
}

func TestWithIndexCreateTimeout(t *testing.T) {
	opts := newOptions(WithIndexCreateTimeout(30 * time.Second))
	require.Equal(t, 30*time.Second, opts.indexTimeout)
}

func TestWithIndexCreateTimeout_Negative(t *testing.T) {
	opts := newOptions(WithIndexCreateTimeout(-1))
	require.Equal(t, DefaultIndexCreateTimeout, opts.indexTimeout)
}

func TestWithIndexCreateTimeout_Zero(t *testing.T) {
	opts := newOptions(WithIndexCreateTimeout(0))
	require.Equal(t, DefaultIndexCreateTimeout, opts.indexTimeout)
}

func TestConstants(t *testing.T) {
	require.NotEmpty(t, DefaultCollectionName, "DefaultCollectionName is empty")
	require.True(t, DefaultIndexCreateTimeout > 0, "DefaultIndexCreateTimeout = %v", DefaultIndexCreateTimeout)
}

func TestEventsIndexes(t *testing.T) {
	require.NotEmpty(t, eventsIndexes, "eventsIndexes is empty")
	for i, idx := range eventsIndexes {
		require.NotNil(t, idx.Keys, "index %d has nil Keys", i)
	}
}

func TestCollectionFieldConstants(t *testing.T) {
	fields := []string{
		collectionFieldId,
		collectionFieldPublishedAt,
		collectionFieldStatus,
		collectionFieldLockedOn,
		collectionFieldCreatedAt,
		collectionFieldLastAttemptOn,
		collectionFieldLastAttempts,
		collectionFieldExpiresAt,
	}
	seen := make(map[string]bool)
	for _, f := range fields {
		require.NotEmpty(t, f, "empty field constant")
		require.False(t, seen[f], "duplicate field constant: %q", f)
		seen[f] = true
	}
}
