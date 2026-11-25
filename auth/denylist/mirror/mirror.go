// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mirror

import (
	"context"
	"fmt"
	"iter"
	"sync/atomic"
)

// Source streams the keys an authoritative revocation store currently considers
// revoked. It is the consumer-side view [Cache] reads on every [Cache.Refresh];
// [github.com/altessa-s/go-atlas/auth/denylist/storages/redis.Store.RawSource] supplies bare Redis keys.
type Source interface {
	// StreamValues yields every currently-revoked key. A non-nil error ends the
	// stream and aborts the refresh, leaving the previous snapshot in place.
	StreamValues(ctx context.Context) iter.Seq2[string, error]
}

// Cache is a synchronous, periodically-refreshed local snapshot of a
// distributed denylist. It satisfies [github.com/altessa-s/go-atlas/auth/jwt.RevocationChecker], so a jwt or
// selfjwt verifier can enforce distributed revocation without a network call on
// the hot path. Reads are lock-free; a nil *Cache is not valid — construct with
// [New].
type Cache struct {
	refresh chan struct{} // serializes source reads and snapshot publication
	src     Source
	metrics *Metrics
	set     atomic.Pointer[map[string]struct{}]
}

// New returns a Cache backed by src. The snapshot starts empty — nothing is
// reported revoked — so call [Cache.Refresh] once before serving traffic and
// then on a schedule to keep it current. src must be non-nil. Pass
// [WithMetrics] to record refresh outcomes and snapshot size.
func New(src Source, opts ...Option) *Cache {
	o := newOptions(opts...)
	c := &Cache{src: src, metrics: o.metrics, refresh: make(chan struct{}, 1)}
	empty := map[string]struct{}{}
	c.set.Store(&empty)
	return c
}

// IsRevoked reports whether id (a jti or subject) is in the current snapshot.
// It reads the snapshot lock-free and never blocks, satisfying the synchronous
// [github.com/altessa-s/go-atlas/auth/jwt.RevocationChecker] contract.
func (c *Cache) IsRevoked(id string) bool {
	m := c.set.Load()
	if m == nil {
		return false
	}
	_, ok := (*m)[id]
	return ok
}

// Refresh rebuilds the snapshot from the authoritative store and atomically
// swaps it in. On a stream error the previous snapshot is kept and the error is
// returned, so a transient store failure degrades to slightly staler data
// rather than an empty denylist. Refresh is intended to run on a scheduler; it
// is safe to call concurrently with [Cache.IsRevoked].
func (c *Cache) Refresh(ctx context.Context) error {
	select {
	case c.refresh <- struct{}{}:
		defer func() { <-c.refresh }()
	case <-ctx.Done():
		c.metrics.recordRefresh(resultError, 0)
		return ctx.Err()
	}
	next := make(map[string]struct{})
	for key, err := range c.src.StreamValues(ctx) {
		if err != nil {
			c.metrics.recordRefresh(resultError, 0)
			return fmt.Errorf("denylist mirror: refresh: %w", err)
		}
		next[key] = struct{}{}
	}
	c.set.Store(&next)
	c.metrics.recordRefresh(resultOK, len(next))
	return nil
}

// Len reports the number of keys in the current snapshot, for observability and
// tests.
func (c *Cache) Len() int {
	m := c.set.Load()
	if m == nil {
		return 0
	}
	return len(*m)
}
