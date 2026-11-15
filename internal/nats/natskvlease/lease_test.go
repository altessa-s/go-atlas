// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package natskvlease_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/nats/natskvlease"
)

// A lease built from the mandatory arguments alone is usable: the optional
// logger, collector, owner check and callbacks all have nil-safe defaults.
func TestNewLease_Defaults(t *testing.T) {
	t.Parallel()

	for name, opts := range map[string][]natskvlease.LeaseOption{
		"no options": nil,
		"nil logger and collector": {
			natskvlease.WithLeaseLogger(nil),
			natskvlease.WithLeaseCollector(nil),
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			lease := natskvlease.NewLease(nil, "my-lease", time.Second, []byte("node-1"), opts...)
			require.False(t, lease.IsHeld())
			require.NotNil(t, lease.GetOps())
			require.NotPanics(t, lease.StopCamping)
		})
	}
}

func TestLeaseCallbacks_NilSafe(t *testing.T) {
	// Verify zero-value callbacks don't panic when checked
	cb := natskvlease.LeaseCallbacks{}
	require.Nil(t, cb.OnAcquired)
	require.Nil(t, cb.OnLost)
	require.Nil(t, cb.OnRenewed)
	require.Nil(t, cb.OnReleased)
}
