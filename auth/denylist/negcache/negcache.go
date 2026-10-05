// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package negcache

import (
	"context"
	"errors"
	"io"
	"sync/atomic"
	"time"

	"github.com/altessa-s/go-atlas/data/probfilter"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
)

// ErrFilterNotRebuildable is returned by [Cache.Rebuild] when the configured
// filter does not implement [probfilter.RebuildableFilter].
var ErrFilterNotRebuildable = errors.New("negcache: filter is not rebuildable")

// Authoritative is the exact revocation store the cache fronts — typically a
// distributed backend (Redis, a database) whose lookups cost a network round
// trip. The cache consults it only when the negative filter cannot rule a key
// out. Following the project convention, the dependency is declared here, on
// the consumer side, so any exact store can be injected.
type Authoritative interface {
	// IsRevoked reports, authoritatively, whether key is revoked.
	IsRevoked(ctx context.Context, key string) (bool, error)
}

// Cache is a probabilistic negative cache over a [probfilter.Filter] placed in
// front of an [Authoritative] revocation store. A definite filter miss is
// answered locally; anything else falls through to the authoritative store for
// the exact answer. See the package doc for the superset invariant and the
// rebuild-staleness window that make this safe.
//
// Until the filter has been populated by a successful rebuild, the cache does
// not trust a filter miss and asks the authoritative store for every key. A
// filter shared between processes (Redis) also counts as populated once any
// process committed a rebuild of it ([probfilter.RebuildCommitReporter]).
//
// A Cache is safe for concurrent use when its filter and authoritative store
// are (probfilter filters are).
type Cache struct {
	filter      probfilter.Filter
	rebuildable probfilter.RebuildableFilter // nil when filter cannot be rebuilt.
	// reporter reports rebuilds committed by any process; nil when the
	// filter is not rebuildable or cannot report them.
	reporter      probfilter.RebuildCommitReporter
	auth          Authoritative
	metrics       *Metrics
	checkInterval time.Duration
	populated     atomic.Bool
	// nextCheck is the UnixNano time before which no shared-rebuild check
	// runs; claimed with a CAS so concurrent lookups check once.
	nextCheck atomic.Int64
}

// New returns a Cache that fast-paths definite filter misses and defers every
// other lookup to authoritative. The filter and authoritative store are
// required; pass [WithMetrics] to record lookup telemetry.
func New(filter probfilter.Filter, authoritative Authoritative, opts ...Option) *Cache {
	o := newOptions(opts...)
	c := &Cache{filter: filter, auth: authoritative, metrics: o.metrics, checkInterval: o.sharedRebuildCheckInterval}
	c.rebuildable, _ = filter.(probfilter.RebuildableFilter)
	if c.rebuildable != nil {
		c.reporter, _ = filter.(probfilter.RebuildCommitReporter)
	}
	return c
}

// IsRevoked reports whether key is revoked. When the populated filter rules
// the key out it returns false without touching the authoritative store;
// otherwise — the filter was never populated, might contain the key, or
// failed — it defers to the authoritative store for the exact answer. The only
// path that skips the authoritative store is a definite miss of a populated
// filter, so an empty or failing filter degrades to correctness (an extra
// lookup), never to a wrongly allowed token.
func (c *Cache) IsRevoked(ctx context.Context, key string) (bool, error) {
	filterState := filterUnpopulated
	if c.isPopulated(ctx) {
		might, ferr := c.filter.MightExist(ctx, key)
		filterState = filterOK
		if ferr != nil {
			filterState = filterError
		}
		if ferr == nil && !might {
			c.metrics.recordLookup(resultFastNegative, filterState)
			return false, nil // definitely not revoked — skip the round trip.
		}
	}

	// Possibly revoked, or the filter is unavailable: confirm exactly.
	revoked, err := c.auth.IsRevoked(ctx, key)
	switch {
	case err != nil:
		c.metrics.recordLookup(resultAuthoritativeError, filterState)
	case revoked:
		c.metrics.recordLookup(resultAuthoritativeHit, filterState)
	default:
		c.metrics.recordLookup(resultAuthoritativeMiss, filterState)
	}
	return revoked, err
}

// Add records key as revoked in the negative filter so subsequent lookups fall
// through to the authoritative store instead of being fast-pathed as absent.
// Adding does not mark the cache populated; only a successful rebuild does.
// Call it whenever this node revokes a key, to keep the filter a superset of
// the revoked set between rebuilds. Adding to the filter does not revoke the
// key in the authoritative store; that write is the caller's responsibility.
func (c *Cache) Add(ctx context.Context, key string) error {
	return c.filter.Add(ctx, key)
}

// Rebuild repopulates the negative filter from loader — the authoritative
// store's full stream of revoked keys — reclaiming a saturated filter and
// absorbing revocations made on other nodes. Schedule it at the cadence the
// deployment's revocation-propagation SLA allows.
//
// The first successful Rebuild marks the cache populated: from then on a
// filter miss is answered locally. The rebuild is atomic, so lookups keep
// using the previous contents while it runs, a failed rebuild leaves them in
// place (the cache stays populated), and [Cache.Add] calls made during it are
// kept.
//
// The configured filter must implement [probfilter.RebuildableFilter] (Bloom
// and Cuckoo filters do); otherwise Rebuild returns [ErrFilterNotRebuildable].
func (c *Cache) Rebuild(ctx context.Context, loader probfilter.DataLoader) error {
	if c.rebuildable == nil {
		return coreerrs.Wrapf(ErrFilterNotRebuildable, "filter type %T", c.filter)
	}
	if err := c.rebuildable.Rebuild(ctx, loader); err != nil {
		return err
	}
	c.populated.Store(true)
	return nil
}

// Close closes the filter the cache was built with when it can be closed
// (the Bloom and Cuckoo facades, or an [io.Closer]); otherwise it does
// nothing. The cache owns its filter: closing it interrupts a running rebuild
// and stops the rebuilds a factory scheduled for it (the process-local cron;
// a task registered with a scheduler becomes a no-op). Do not use the cache
// after Close.
func (c *Cache) Close(ctx context.Context) error {
	switch closer := c.filter.(type) {
	case interface{ Close(context.Context) error }:
		return closer.Close(ctx)
	case io.Closer:
		return closer.Close()
	}
	return nil
}

// isPopulated reports whether the filter holds the revoked set: a Rebuild
// through this cache succeeded, the filter reports a successful rebuild made
// elsewhere in this process (for example a scheduled factory rebuild), or a
// shared filter reports a rebuild committed by any process. A filter that
// cannot be rebuilt is never considered populated.
func (c *Cache) isPopulated(ctx context.Context) bool {
	if c.populated.Load() {
		return true
	}
	if c.rebuildable == nil {
		return false
	}
	if !c.rebuildable.LastRebuild().IsZero() || c.sharedRebuildCommitted(ctx) {
		c.populated.Store(true)
		return true
	}
	return false
}

// sharedRebuildCommitted asks the filter, at most once per check interval,
// whether a rebuild was committed by any process. An error counts as not
// committed, so the lookup falls back to the authoritative store.
func (c *Cache) sharedRebuildCommitted(ctx context.Context) bool {
	if c.reporter == nil {
		return false
	}
	now := time.Now().UnixNano()
	next := c.nextCheck.Load()
	if now < next || !c.nextCheck.CompareAndSwap(next, now+int64(c.checkInterval)) {
		return false
	}
	committed, err := c.reporter.RebuildCommitted(ctx)
	return err == nil && committed
}
