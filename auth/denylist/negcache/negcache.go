// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package negcache

import (
	"context"
	"errors"
	"sync/atomic"

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
// not trust a filter miss and asks the authoritative store for every key.
//
// A Cache is safe for concurrent use when its filter and authoritative store
// are (probfilter filters are).
type Cache struct {
	filter      probfilter.Filter
	rebuildable probfilter.RebuildableFilter // nil when filter cannot be rebuilt.
	auth        Authoritative
	metrics     *Metrics
	populated   atomic.Bool
}

// New returns a Cache that fast-paths definite filter misses and defers every
// other lookup to authoritative. The filter and authoritative store are
// required; pass [WithMetrics] to record lookup telemetry.
func New(filter probfilter.Filter, authoritative Authoritative, opts ...Option) *Cache {
	o := newOptions(opts...)
	rebuildable, _ := filter.(probfilter.RebuildableFilter)
	return &Cache{filter: filter, rebuildable: rebuildable, auth: authoritative, metrics: o.metrics}
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
	if c.isPopulated() {
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

// isPopulated reports whether the filter holds the revoked set: a Rebuild
// through this cache succeeded, or the filter reports a successful rebuild
// made elsewhere (for example a scheduled factory rebuild). A filter that
// cannot be rebuilt is never considered populated.
func (c *Cache) isPopulated() bool {
	if c.populated.Load() {
		return true
	}
	if c.rebuildable == nil || c.rebuildable.LastRebuild().IsZero() {
		return false
	}
	c.populated.Store(true)
	return true
}
