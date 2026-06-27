// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package nats

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/leadelect/providers"
)

// leaderProvider builds a Provider in the "running leader" state with the given
// election TTL and last-confirmed-lease instant, without any NATS connection.
// It exercises the IsLeader freshness gate in isolation.
func leaderProvider(ttl time.Duration, lastOK *time.Time) *Provider {
	p := &Provider{}
	p.isRunning.Store(true)
	p.isLeader.Store(true)
	p.providerConfig.Store(&providers.Config{Key: "k", NodeId: "n", TTL: ttl})
	if lastOK != nil {
		p.lastRenewOK.Store(lastOK)
	}
	return p
}

func TestIsLeader_FreshLeaseIsLeader(t *testing.T) {
	t.Parallel()
	now := time.Now()
	p := leaderProvider(time.Hour, &now)
	require.True(t, p.IsLeader(), "a just-confirmed lease must report leadership")
}

func TestIsLeader_StaleLeaseSelfDemotes(t *testing.T) {
	t.Parallel()
	// Confirmed long ago relative to the election TTL: leadership has expired
	// even though the cached isLeader flag is still true (frozen camping).
	old := time.Now().Add(-2 * time.Second)
	p := leaderProvider(time.Second, &old)
	require.False(t, p.IsLeader(), "a lease older than the TTL must not report leadership")
}

func TestIsLeader_LeaseBoundedByBucketTTL(t *testing.T) {
	t.Parallel()
	// Election TTL is larger than the server-side key TTL; the freshness bound
	// must clamp to DefaultBucketKeysTTL so we never report leadership past the
	// point the KV key could have expired and been re-acquired.
	old := time.Now().Add(-(DefaultBucketKeysTTL + time.Minute))
	p := leaderProvider(DefaultBucketKeysTTL+time.Hour, &old)
	require.False(t, p.IsLeader(), "leadership must expire at the bucket TTL, not the larger election TTL")

	fresh := time.Now()
	p2 := leaderProvider(DefaultBucketKeysTTL+time.Hour, &fresh)
	require.True(t, p2.IsLeader(), "a fresh lease is still leader under the large election TTL")
}

func TestIsLeader_NotRunning(t *testing.T) {
	t.Parallel()
	now := time.Now()
	p := leaderProvider(time.Hour, &now)
	p.isRunning.Store(false)
	require.False(t, p.IsLeader(), "a stopped provider is never the leader")
}

func TestIsLeader_FlagFalse(t *testing.T) {
	t.Parallel()
	now := time.Now()
	p := leaderProvider(time.Hour, &now)
	p.isLeader.Store(false)
	require.False(t, p.IsLeader())
}

func TestIsLeader_NeverConfirmed(t *testing.T) {
	t.Parallel()
	// isLeader flag set but no lease ever recorded (lastRenewOK nil): treat as
	// not-yet-confirmed rather than leader.
	p := leaderProvider(time.Hour, nil)
	require.False(t, p.IsLeader())
}

func TestMarkRenewed_RefreshesLease(t *testing.T) {
	t.Parallel()
	old := time.Now().Add(-2 * time.Second)
	p := leaderProvider(time.Second, &old)
	require.False(t, p.IsLeader(), "precondition: stale")

	p.markRenewed()
	require.True(t, p.IsLeader(), "markRenewed must refresh the lease and restore leadership")
}
