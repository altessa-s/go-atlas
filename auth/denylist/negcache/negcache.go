// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package negcache

import (
	"context"
	"errors"

	"github.com/altessa-s/go-atlas/data/probfilter"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
)

// ErrFilterNotRebuildable is returned by [Cache.Rebuild] when the configured
// filter does not implement [probfilter.RebuildableFilter] (for example a
// Cuckoo filter, which supports Delete but not Rebuild).
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
// A Cache is safe for concurrent use when its filter and authoritative store
// are (probfilter filters are).
type Cache struct {
	filter  probfilter.Filter
	auth    Authoritative
	metrics *Metrics
}

// New returns a Cache that fast-paths definite filter misses and defers every
// other lookup to authoritative. The filter and authoritative store are
// required; pass [WithMetrics] to record lookup telemetry.
func New(filter probfilter.Filter, authoritative Authoritative, opts ...Option) *Cache {
	o := newOptions(opts...)
	return &Cache{filter: filter, auth: authoritative, metrics: o.metrics}
}

// IsRevoked reports whether key is revoked. When the filter rules the key out
// it returns false without touching the authoritative store; otherwise, and on
// any filter error, it defers to the authoritative store for the exact answer.
// The only path that skips the authoritative store is a definite filter miss,
// so a filter failure degrades to correctness (an extra lookup), never to a
// wrongly allowed token.
func (c *Cache) IsRevoked(ctx context.Context, key string) (bool, error) {
	might, ferr := c.filter.MightExist(ctx, key)
	filterState := filterOK
	if ferr != nil {
		filterState = filterError
	}
	if ferr == nil && !might {
		c.metrics.recordLookup(resultFastNegative, filterState)
		return false, nil // definitely not revoked — skip the round trip.
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
// The configured filter must implement [probfilter.RebuildableFilter] (a Bloom
// filter does); otherwise Rebuild returns [ErrFilterNotRebuildable].
func (c *Cache) Rebuild(ctx context.Context, loader probfilter.DataLoader) error {
	rebuildable, ok := c.filter.(probfilter.RebuildableFilter)
	if !ok {
		return coreerrs.Wrapf(ErrFilterNotRebuildable, "filter type %T", c.filter)
	}
	return rebuildable.Rebuild(ctx, loader)
}
