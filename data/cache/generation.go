// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package cache

import (
	"context"
	"errors"
	"strconv"

	"github.com/altessa-s/go-atlas/data/cache/generation"
)

// GenerationStore is an alias for [generation.Store] from data/cache/generation.
type GenerationStore = generation.Store

// KeyVersionFunc returns the version embedded in every key of a [Cache]
// configured with [WithKeyVersion]. A changed version orphans every entry
// stored under an older one. An error means the version is unknown: the
// cache is skipped (see [WithKeyVersion]).
type KeyVersionFunc func(context.Context) (uint64, error)

// ErrNoGeneration is returned by [Cache.InvalidateAll] on a cache built
// without [WithGeneration].
var ErrNoGeneration = errors.New("cache: no generation store configured")

// InvalidateAll orphans every entry of a cache built with [WithGeneration] by
// bumping its generation: later reads, on every replica sharing the
// generation store, use new keys. A fill that started before the bump stores
// its value under the old generation, where it is never read again. Entries
// are not deleted; they expire under their TTL or are evicted.
//
// It returns [ErrNoGeneration] without [WithGeneration], and the store's
// error when the bump failed (see the store's documentation for what other
// replicas observe then).
//
// Example:
//
//	if err := c.InvalidateAll(ctx); err != nil {
//		logger.WarnContext(ctx, "cache invalidation not published", "error", err)
//	}
func (c *Cache) InvalidateAll(ctx context.Context) error {
	if c.invalidate == nil {
		return ErrNoGeneration
	}
	return c.invalidate(ctx)
}

// versionPrefix returns the key prefix of the current key version, or "" when
// the cache has no key version.
func (c *Cache) versionPrefix(ctx context.Context) (string, error) {
	if c.keyVersion == nil {
		return "", nil
	}
	v, err := c.keyVersion(ctx)
	if err != nil {
		c.metrics.errors.Inc()
		return "", err
	}
	return strconv.FormatUint(v, 10) + ":", nil
}

// versionUnavailable reports whether a key-version error may be degraded: the
// cache is fail-open and the caller has not gone away. It records the event.
func (c *Cache) versionUnavailable(ctx context.Context) bool {
	if !c.failOpen || ctx.Err() != nil {
		return false
	}
	c.metrics.failOpenVersion.Inc()
	return true
}

// bypassFallback runs fallback directly in the caller, without the cache or
// the singleflight group, within the fallback timeout. It is the read-through
// path when the key version is unknown: the source of truth is read and
// nothing is stored, so no entry of an unknown generation is written. The
// TTL the fallback returns is ignored.
func (c *Cache) bypassFallback(ctx context.Context, fallback Fallback) (any, error) {
	c.metrics.failOpenVersion.Inc()
	return c.directFallback(ctx, fallback)
}

// directFallback runs fallback in the caller, without the cache or the
// singleflight group, within the fallback timeout, and stores nothing. A nil
// value is [ErrMissing].
func (c *Cache) directFallback(ctx context.Context, fallback Fallback) (any, error) {
	if c.fallbackTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.fallbackTimeout)
		defer cancel()
	}
	stop := c.metrics.fallbackDuration.Start()
	val, _, err := fallback(ctx)
	stop()
	if err != nil {
		return nil, err
	}
	if isNilValue(val) {
		return nil, ErrMissing
	}
	return val, nil
}

// generationOptions returns the key version and invalidation of a cache
// bound to namespace in store.
func generationOptions(store GenerationStore, namespace string) (KeyVersionFunc, func(context.Context) error) {
	version := func(ctx context.Context) (uint64, error) {
		return store.Current(ctx, namespace)
	}
	invalidate := func(ctx context.Context) error {
		_, err := store.Bump(ctx, namespace)
		return err
	}
	return version, invalidate
}
