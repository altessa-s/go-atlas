// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package cache

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/altessa-s/go-atlas/core/encoding/keys"
	"github.com/altessa-s/go-atlas/core/runtime/panics"

	corecontext "github.com/altessa-s/go-atlas/core/context"
)

// Generations is a generational read-through cache: a [Cache] whose keys
// embed the generation of a namespace from a [GenerationStore]. A write bumps
// the namespaces it touched ([Generations.Bump]), which orphans every entry
// read under an older generation at once. A fill that started before the bump
// stores its value under the old generation, where it is never read again, so
// a slow read can never resurrect data a write replaced. A store shared by
// several replicas extends this to all of them within its staleness bound.
//
// Reads go through [ReadThrough]. They skip the cache, reading the source of
// truth and storing nothing, when ctx is marked by [WithFreshReads], when the
// [WithBypass] predicate holds, when the generation is unknown (the store
// failed), or when the cache is nil (caching disabled).
//
// Generations is safe for concurrent use.
type Generations struct {
	cache  *Cache
	store  GenerationStore
	bypass func(context.Context) bool
}

// NewGenerations returns a generational cache over c. The default store is
// in-process; pass [WithGenerationStore] to share generations between
// replicas. A nil c disables caching: [ReadThrough] always fills and
// [Generations.Bump] still bumps the store.
//
// Entries are orphaned, not deleted: give c a TTL (and a bounded storage
// such as storages/lru) so they expire. The TTL is also the staleness bound
// for writes that do not call Bump (another service, a script) and for a
// bump that could not be published (see the store). Configure c with
// [WithFailOpen] so a cache failure never fails a read.
//
// Example:
//
//	storage, _ := lru.New(4096)
//	gens := cache.NewGenerations(
//		cache.New(storage, cache.WithTTL(5*time.Minute), cache.WithFailOpen(0)),
//		cache.WithGenerationStore(redisgen.New(client)),
//		cache.WithBypass(func(ctx context.Context) bool { return mongo.SessionFromContext(ctx) != nil }),
//	)
func NewGenerations(c *Cache, opts ...GenerationsOption) *Generations {
	o := newGenerationsOptions(opts...)
	return &Generations{cache: c, store: o.store, bypass: o.bypass}
}

// Key returns the cache key of parts under the current generation of
// namespace. The namespace, generation and parts are length-prefixed
// ([keys.Compose]), so distinct tuples never share a key. An error means the
// generation is unknown and the cache must not be used.
func (g *Generations) Key(ctx context.Context, namespace string, parts ...string) (string, error) {
	gen, err := g.store.Current(ctx, namespace)
	if err != nil {
		return "", err
	}
	all := make([]string, 0, len(parts)+2)
	all = append(all, namespace, strconv.FormatUint(gen, 10))
	return keys.Compose(append(all, parts...)...), nil
}

// Bump advances the generation of every namespace, orphaning their cached
// entries. Call it after the write committed, also when the write failed in a
// way that proves the cached view stale (a version conflict, a missing or an
// already existing item), so the caller's retry reloads it.
//
// Every namespace is bumped even when another fails; the joined errors are
// returned. A failed bump is not visible to other replicas: log it.
func (g *Generations) Bump(ctx context.Context, namespaces ...string) error {
	var errs []error
	for _, ns := range namespaces {
		if _, err := g.store.Bump(ctx, ns); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// skip reports whether a read of a non-nil cache must go straight to the
// source of truth.
func (g *Generations) skip(ctx context.Context) bool {
	return FreshReads(ctx) || (g.bypass != nil && g.bypass(ctx))
}

// ReadThrough returns the value of key in namespace from g's cache, or loads
// it with fill and caches it under the current generation (with the cache's
// default TTL). Concurrent misses of one key and generation share one fill
// (singleflight); every caller, the one that ran the fill included, receives
// its own decoded copy, so no slice, map or pointer is shared between callers
// or with the cache, whatever the cache's [WithIsolatedResults].
//
// fill errors are returned and not cached ([ErrMissing] is negatively cached
// when the cache has [WithNegativeTTL]); a nil pointer value is [ErrMissing].
// The read skips the cache as described on [Generations]; a nil g fills
// directly too. A skipped read runs fill in the caller, unshared, within the
// cache's [WithFallbackTimeout] (a nil g or cache has none), stores nothing
// and still maps a nil pointer value to [ErrMissing]. key may be empty (a
// namespace-wide value). Panics if fill is nil.
//
// Example:
//
//	item, err := cache.ReadThrough(ctx, gens, kind, "get:"+code, func(ctx context.Context) (Item, error) {
//		return store.Get(ctx, kind, code)
//	})
func ReadThrough[T any](ctx context.Context, g *Generations, namespace, key string, fill func(context.Context) (T, error)) (T, error) {
	panics.MustNonNil(fill, "fill must be provided")
	if g == nil || g.cache == nil {
		return fillUncached(ctx, fill)
	}
	if g.skip(ctx) {
		return fillDirect(ctx, g.cache.directFallback, fill)
	}
	k, err := g.Key(ctx, namespace, key)
	if err != nil {
		g.cache.metrics.errors.Inc()
		return fillDirect(ctx, g.cache.bypassFallback, fill)
	}
	return getWithFallbackT(ctx, g.cache, k, true, func(ctx context.Context) (T, time.Duration, error) {
		v, err := fill(ctx)
		return v, TTLUseDefault, err
	})
}

// fillDirect runs fill through run (the cache's directFallback or
// bypassFallback) under the default context timeout: the fill gets the
// cache's fallback timeout, nothing is stored and a nil value is
// [ErrMissing], as on the skip paths of [GetWithFallbackT].
func fillDirect[T any](
	ctx context.Context, run func(context.Context, Fallback) (any, error), fill func(context.Context) (T, error),
) (T, error) {
	ctx, cancel := corecontext.WithDefault(ctx, defaultContextTimeout)
	defer cancel()
	fv, err := run(ctx, func(work context.Context) (any, time.Duration, error) {
		v, err := fill(work)
		return v, TTLUseDefault, err
	})
	if err != nil {
		var zero T
		return zero, err
	}
	v, _ := fv.(T) // fv is the fill's own T
	return v, nil
}

// fillUncached runs fill for a nil [Generations] or a nil cache: there is no
// fallback timeout, but the default context timeout applies and a nil value
// is [ErrMissing], as with a cache.
func fillUncached[T any](ctx context.Context, fill func(context.Context) (T, error)) (T, error) {
	ctx, cancel := corecontext.WithDefault(ctx, defaultContextTimeout)
	defer cancel()
	v, err := fill(ctx)
	if err == nil && isNilValue(v) {
		var zero T
		return zero, ErrMissing
	}
	return v, err
}
