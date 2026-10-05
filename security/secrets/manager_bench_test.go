// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package secrets_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/security/secrets"

	secretmemory "github.com/altessa-s/go-atlas/security/secrets/providers/memory"
)

// newBenchManager returns a Manager with key-a cached.
func newBenchManager(b *testing.B) *secrets.Manager[string] {
	b.Helper()
	inner, err := secretmemory.New(map[string]string{"key-a": "a-secret-payload"})
	require.NoError(b, err)
	mgr, err := secrets.New[string](inner)
	require.NoError(b, err)
	_, err = mgr.Value(b.Context(), "key-a", true)
	require.NoError(b, err)
	return mgr
}

// BenchmarkManager_Value measures a cache hit that returns a caller-owned
// copy, cleared after use as callers should.
func BenchmarkManager_Value(b *testing.B) {
	mgr := newBenchManager(b)
	ctx := b.Context()
	for b.Loop() {
		v, err := mgr.Value(ctx, "key-a", false)
		if err != nil {
			b.Fatal(err)
		}
		v.Clear()
	}
}

// BenchmarkManager_ValueShared measures a zero-copy cache hit.
func BenchmarkManager_ValueShared(b *testing.B) {
	mgr := newBenchManager(b)
	ctx := b.Context()
	for b.Loop() {
		if _, err := mgr.ValueShared(ctx, "key-a", false); err != nil {
			b.Fatal(err)
		}
	}
}
