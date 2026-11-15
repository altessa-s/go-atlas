// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package oidc

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	goredis "github.com/redis/go-redis/v9"
)

// skewBound is the staleness bound of the clock-skew scenarios.
const skewBound = 2 * time.Minute

// skewedProvider starts a provider on the shared filter with a sync schedule
// that never ticks during the test and a bound of skewBound.
func skewedProvider(t *testing.T, idp *testIdP, client *goredis.Client) *Provider {
	t.Helper()
	storage, _ := sharedReplica(client, "j1")
	return idp.newProvider(t,
		WithRevocationStorage(storage), WithRevocationItemType(RevocationItemTypeJTI),
		WithRevocationSyncSchedule("@every 24h"), WithRevocationMaxStaleness(skewBound))
}

// losingTick runs the scheduled sync of p while a peer whose source is down
// holds the rebuild lease: p is refused and no one commits.
func losingTick(t *testing.T, p *Provider, client *goredis.Client) {
	t.Helper()
	sourceDown := errors.New("source down")
	finish := holdRebuildLease(t, client, sourceDown)
	require.NoError(t, p.scheduledRevocationSync(t.Context()))
	require.ErrorIs(t, finish(), sourceDown)
}

// A Redis clock ahead of the replica's must not stretch the bound: the commit
// time looks like the future to the local clock, and re-reading it on every
// failed tick must not make the commit look new again.
func TestRevocationStaleness_RedisClockAheadDoesNotExtendBound(t *testing.T) {
	t.Parallel()

	idp := newTestIdP(t)
	client, mr := newSharedBloomRedis(t)
	redisNow := time.Now().Add(5 * time.Minute)
	mr.SetTime(redisNow)
	p := skewedProvider(t, idp, client)
	peer, _ := sharedReplica(client, "j1")
	require.NoError(t, peer.Sync(t.Context()))
	backdateRevocationSync(p, time.Hour)

	losingTick(t, p, client)
	require.NoError(t, p.checkRevocationStaleness(), "the peer just committed")
	seen := p.sharedRevocationSyncUnixNanos.Load()
	require.InDelta(t, time.Now().UnixNano(), seen, float64(10*time.Second))

	// Three minutes pass on both clocks while every sync fails.
	const elapsed = 3 * time.Minute
	mr.SetTime(redisNow.Add(elapsed))
	p.sharedRevocationSyncUnixNanos.Add(-int64(elapsed))
	for range 3 {
		losingTick(t, p, client)
		require.Equal(t, seen-int64(elapsed), p.sharedRevocationSyncUnixNanos.Load(),
			"re-reading a commit already seen must not refresh it")
		require.ErrorIs(t, p.checkRevocationStaleness(), ErrRevocationStale)
	}
}

// With the Redis clock ahead, a commit this replica reads for the first time
// is as old as Redis says it is, not "in the future".
func TestRevocationStaleness_RedisClockAheadOldCommitIsStale(t *testing.T) {
	t.Parallel()

	idp := newTestIdP(t)
	client, mr := newSharedBloomRedis(t)
	redisNow := time.Now().Add(5 * time.Minute)
	mr.SetTime(redisNow)
	p := skewedProvider(t, idp, client)
	peer, _ := sharedReplica(client, "j1")
	require.NoError(t, peer.Sync(t.Context()))
	backdateRevocationSync(p, time.Hour)

	// Three minutes later on the Redis clock — still ahead of the local one.
	mr.SetTime(redisNow.Add(3 * time.Minute))
	losingTick(t, p, client)
	require.ErrorIs(t, p.checkRevocationStaleness(), ErrRevocationStale)
}

// A Redis clock behind the replica's must not shorten the bound: a commit a
// peer made a moment ago is fresh although its timestamp is older than the
// bound on the local clock.
func TestRevocationStaleness_RedisClockBehindDoesNotShortenBound(t *testing.T) {
	t.Parallel()

	idp := newTestIdP(t)
	client, mr := newSharedBloomRedis(t)
	redisNow := time.Now().Add(-2 * time.Hour)
	mr.SetTime(redisNow)
	p := skewedProvider(t, idp, client)
	peer, _ := sharedReplica(client, "j1")
	backdateRevocationSync(p, time.Hour)

	for i := range 3 {
		// A healthy peer commits on every cycle; this replica keeps losing.
		mr.SetTime(redisNow.Add(time.Duration(i+1) * 30 * time.Second))
		require.NoError(t, peer.Sync(t.Context()))
		p.sharedRevocationSyncUnixNanos.Store(time.Now().Add(-time.Hour).UnixNano())
		losingTick(t, p, client)

		require.NoError(t, p.checkRevocationStaleness())
		require.InDelta(t, time.Now().UnixNano(), p.sharedRevocationSyncUnixNanos.Load(), float64(10*time.Second))
	}
}
