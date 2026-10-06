// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package redisfilter

import (
	"context"
	"strconv"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/altessa-s/go-atlas/data/probfilter"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
)

// LeaseTTL is the lifetime of a shared filter's rebuild lease. The holder
// renews it every LeaseTTL/leaseRenewDivisor while the rebuild runs; a
// crashed or frozen holder stops renewing and loses it after LeaseTTL.
const LeaseTTL = 30 * time.Second

// leaseRenewDivisor sets the renewal interval to LeaseTTL/leaseRenewDivisor.
const leaseRenewDivisor = 3

// acquireLeaseScript takes the next rebuild ticket and acquires the rebuild
// lease with it, or returns -1 when another rebuild holds the lease. It drops
// the journal a crashed or superseded rebuild may have left, so the new
// rebuild journals only inserts made under its own lease.
// KEYS[1] ticket sequence; KEYS[2] lease; KEYS[3] journal; ARGV[1] lease TTL ms.
var acquireLeaseScript = redis.NewScript(`
if redis.call('EXISTS', KEYS[2]) == 1 then return -1 end
local ticket = redis.call('INCR', KEYS[1])
redis.call('SET', KEYS[2], ticket, 'PX', ARGV[1])
redis.call('DEL', KEYS[3])
return ticket
`)

// renewLeaseScript extends the lease while it still holds ARGV[1].
// KEYS[1] lease; ARGV[1] ticket; ARGV[2] lease TTL ms.
var renewLeaseScript = redis.NewScript(`
if redis.call('GET', KEYS[1]) ~= ARGV[1] then return 0 end
return redis.call('PEXPIRE', KEYS[1], ARGV[2])
`)

// releaseLeaseScript deletes the lease, and the journal no rebuild needs any
// more, while the lease still holds ARGV[1].
// KEYS[1] lease; KEYS[2] journal; ARGV[1] ticket.
var releaseLeaseScript = redis.NewScript(`
if redis.call('GET', KEYS[1]) ~= ARGV[1] then return 0 end
redis.call('DEL', KEYS[2])
return redis.call('DEL', KEYS[1])
`)

// Lease is the rebuild lease of a shared filter. While a rebuild holds it, no
// other process can start rebuilding the same filter, and only the holder
// can publish (see [Lease.Stage] and [Staging.Commit]), so a rebuild with an
// older snapshot can never overwrite a newer one.
type Lease struct {
	core   *Core
	ticket int64

	stopOnce sync.Once
	stop     chan struct{}
	stopped  chan struct{}
}

// BeginRebuild acquires the filter's rebuild lease before the rebuild reads
// its source. It fails with an error wrapping
// [probfilter.ErrRebuildInProgress] when another rebuild holds the lease. The
// lease is renewed in the background until [Lease.Release].
func (c *Core) BeginRebuild(ctx context.Context) (*Lease, error) {
	if c.keyErr != nil {
		return nil, c.keyErr
	}
	ticket, err := acquireLeaseScript.Run(ctx, c.client, []string{c.seqKey, c.leaseKey, c.journalKey}, c.leaseTTL.Milliseconds()).Int64()
	if err != nil {
		return nil, coreerrs.WrapOperation(err, "acquire Redis "+c.cmds.Label+" filter rebuild lease")
	}
	if ticket < 0 {
		return nil, coreerrs.WrapOperation(probfilter.ErrRebuildInProgress, "acquire Redis "+c.cmds.Label+" filter rebuild lease")
	}

	l := &Lease{core: c, ticket: ticket, stop: make(chan struct{}), stopped: make(chan struct{})}
	// The renewer outlives the BeginRebuild call; it stops at Release, not at
	// the cancellation of ctx, but keeps ctx's values.
	go l.renew(context.WithoutCancel(ctx))
	return l, nil
}

// renew extends the lease until Release. Failures are retried at the next
// tick; a lost lease makes the commit fail, so nothing else is needed.
func (l *Lease) renew(base context.Context) {
	defer close(l.stopped)
	interval := l.core.leaseTTL / leaseRenewDivisor
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-l.stop:
			return
		case <-ticker.C:
			ctx, cancel := context.WithTimeout(base, interval)
			_ = renewLeaseScript.Run(ctx, l.core.client, []string{l.core.leaseKey},
				strconv.FormatInt(l.ticket, 10), l.core.leaseTTL.Milliseconds()).Err()
			cancel()
		}
	}
}

// Stage reserves the replacement filter of this rebuild (see [Core.Stage]);
// its commit publishes only while this lease is still held.
func (l *Lease) Stage(ctx context.Context, reserveArgs ...any) (*Staging, error) {
	st, err := l.core.Stage(ctx, reserveArgs...)
	if err != nil {
		return nil, err
	}
	st.ticket = l.ticket
	return st, nil
}

// Release stops renewing and releases the lease if it is still held.
func (l *Lease) Release(ctx context.Context) error {
	l.stopOnce.Do(func() { close(l.stop) })
	<-l.stopped
	err := releaseLeaseScript.Run(ctx, l.core.client, []string{l.core.leaseKey, l.core.journalKey}, strconv.FormatInt(l.ticket, 10)).Err()
	if err != nil {
		return coreerrs.WrapOperation(err, "release Redis "+l.core.cmds.Label+" filter rebuild lease")
	}
	return nil
}
