// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package natskvlease_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/internal/natskvlease"
)

func TestLeaseConfig_Fields(t *testing.T) {
	cfg := natskvlease.LeaseConfig{
		Key:   "my-lease",
		Value: []byte("node-1"),
	}
	require.Equal(t, "my-lease", cfg.Key)
	require.Equal(t, "node-1", string(cfg.Value))
}

func TestLeaseCallbacks_NilSafe(t *testing.T) {
	// Verify zero-value callbacks don't panic when checked
	cb := natskvlease.LeaseCallbacks{}
	require.Nil(t, cb.OnAcquired)
	require.Nil(t, cb.OnLost)
	require.Nil(t, cb.OnRenewed)
	require.Nil(t, cb.OnReleased)
}
