// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package cache

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"time"

	"github.com/altessa-s/go-atlas/core/encoding/serializer"
	"github.com/altessa-s/go-atlas/core/runtime/panics"
	"github.com/altessa-s/go-atlas/core/text/strings"
	"github.com/altessa-s/go-atlas/data/cache/providers/noop"

	"golang.org/x/sync/semaphore"
	"golang.org/x/sync/singleflight"

	corecontext "github.com/altessa-s/go-atlas/core/context"
)

// NoTTL is the TTL value indicating that a cache item will not expire.
const NoTTL time.Duration = 0

// TTLUseDefault is the TTL value indicating that the cache's default TTL should be used.
const TTLUseDefault time.Duration = -1

// defaultContextTimeout is the timeout applied when no context is provided.
const defaultContextTimeout = 15 * time.Second

// Fallback is a function called when a cache key is not found.
// It returns the value to cache, optional TTL (use TTLUseDefault for cache default), and any error.
// If it returns a nil value, ErrMissing is returned to the caller.
type Fallback = func() (value any, ttl time.Duration, err error)

// negativeSentinel is a single null byte stored as the value for negative cache entries.
// It cannot collide with JSON/msgpack serializer output.
var negativeSentinel = []byte{0x00}

func isNegativeSentinel(data []byte) bool {
	return len(data) == 1 && data[0] == 0x00
}

// Cache provides caching functionality with configurable providers and serializers.
// It uses singleflight to deduplicate concurrent requests for the same key
// and bounds the total number of in-flight fallback functions via a
// semaphore (see [DefaultMaxConcurrentFallbacks]).
type Cache struct {
	provider     Provider
	ttl          time.Duration
	negativeTtl  time.Duration
	group        *singleflight.Group
	fallbackSem  *semaphore.Weighted // nil when maxConcurrentFallbacks <= 0
	serializer   serializer.Serializer
	metrics      *cacheMetrics
	keyNamespace KeyNamespaceFunc // nil = no per-context namespacing
}

// New creates a new Cache instance with the given provider and options.
// Panics if provider is nil. Default TTL is 1 hour with JSON serialization.
//
// Example:
//
//	c := cache.New(redisProvider, cache.WithTtl(10*time.Minute))
func New(p Provider, opts ...Option) *Cache {
	panics.MustNonNil(p, "provider must be provided")

	options := newOptions(opts...)

	c := &Cache{
		provider:     p,
		ttl:          options.ttl,
		negativeTtl:  options.negativeTtl,
		group:        &singleflight.Group{},
		serializer:   options.serializer,
		metrics:      newCacheMetrics(options.collector, options.name),
		keyNamespace: options.keyNamespace,
	}
	if options.maxConcurrentFallbacks > 0 {
		c.fallbackSem = semaphore.NewWeighted(int64(options.maxConcurrentFallbacks))
	}
	return c
}

// NewNoop creates a new Cache instance with a no-op provider that discards all writes.
// Useful for testing or disabling caching without code changes.
//
// Example:
//
//	c := cache.NewNoop()
func NewNoop(opts ...Option) *Cache {
	return New(noop.New(), opts...)
}

// effectiveKey applies the optional per-context namespace so cache entries and
// singleflight de-duplication are isolated by tenant/subject. It is applied
// uniformly to every provider operation and to the singleflight key, so a Get
// and its matching Save always agree. A nil namespace or an empty result leaves
// the key unchanged (backward-compatible default).
func (c *Cache) effectiveKey(ctx context.Context, key string) string {
	if c.keyNamespace == nil {
		return key
	}
	if ns := c.keyNamespace(ctx); ns != "" {
		return ns + ":" + key
	}
	return key
}

// effectiveKeys maps effectiveKey over a slice, allocating only when a non-empty
// namespace is actually applied.
func (c *Cache) effectiveKeys(ctx context.Context, keys []string) []string {
	if c.keyNamespace == nil {
		return keys
	}
	ns := c.keyNamespace(ctx)
	if ns == "" {
		return keys
	}
	out := make([]string, len(keys))
	for i, k := range keys {
		out[i] = ns + ":" + k
	}
	return out
}

// GetWithFallback retrieves a cached value or calls fallback if not found.
// The value parameter must be a pointer. Panics if key is empty or fallback is nil.
// Uses singleflight to prevent duplicate fallback calls for the same key.
//
// Example:
//
//	var user User
//	err := c.GetWithFallback(ctx, "user:123", &user, func() (any, time.Duration, error) {
//		return db.GetUser(123), cache.TTLUseDefault, nil
//	})
func (c *Cache) GetWithFallback(ctx context.Context, key string, value any, fallback Fallback) error {
	panics.Must(!strings.IsEmpty(key), "key must be provided")
	panics.Must(reflect.Indirect(reflect.ValueOf(value)).CanSet(), "value type must be assignable")
	panics.MustNonNil(fallback, "fallback must be provided")

	ctx, cancel := corecontext.WithDefault(ctx, defaultContextTimeout)
	defer cancel()

	// Namespace the key for both the backend and singleflight so concurrent
	// callers from different tenants never collapse onto one another's result.
	ek := c.effectiveKey(ctx, key)

	data, found, err := c.lookupRaw(ctx, ek)
	if err != nil {
		return err
	}
	if found {
		return c.serializer.Deserialize(data, value)
	}

	fv, err := c.runFallback(ctx, ek, fallback)
	if err != nil {
		return err
	}

	reflect.Indirect(reflect.ValueOf(value)).Set(reflect.Indirect(reflect.ValueOf(fv)))

	return nil
}

// GetWithFallbackT is the typed variant of [Cache.GetWithFallback] for hot
// call paths: the destination is a value of T, so the per-call reflection of
// the any-based API (assignability assertion and copy-out) disappears. The
// untyped method remains for heterogeneous call sites.
//
// Example:
//
//	user, err := cache.GetWithFallbackT(ctx, c, "user:123", func() (User, time.Duration, error) {
//		return db.GetUser(123), cache.TTLUseDefault, nil
//	})
func GetWithFallbackT[T any](ctx context.Context, c *Cache, key string, fallback func() (T, time.Duration, error)) (T, error) {
	var out T
	panics.Must(!strings.IsEmpty(key), "key must be provided")
	panics.MustNonNil(fallback, "fallback must be provided")

	ctx, cancel := corecontext.WithDefault(ctx, defaultContextTimeout)
	defer cancel()

	ek := c.effectiveKey(ctx, key)

	data, found, err := c.lookupRaw(ctx, ek)
	if err != nil {
		return out, err
	}
	if found {
		if err = c.serializer.Deserialize(data, &out); err != nil {
			return out, err
		}
		return out, nil
	}

	// The boxing wrapper is built only on the miss path; hits stay free of
	// per-call closures.
	fv, err := c.runFallback(ctx, ek, func() (any, time.Duration, error) {
		return fallback()
	})
	if err != nil {
		return out, err
	}

	// A concurrent untyped caller may have won the singleflight with a *T:
	// accept both shapes, mirroring the reflect.Indirect copy-out of the
	// untyped method.
	if v, ok := fv.(T); ok {
		return v, nil
	}
	if p, ok := fv.(*T); ok && p != nil {
		return *p, nil
	}
	return out, fmt.Errorf("cache: singleflight value type %T does not match requested type %T", fv, out)
}

// lookupRaw performs the provider read with negative-sentinel handling and
// hit/miss metrics. found=false with a nil error means a plain miss — the
// caller should run the fallback path.
func (c *Cache) lookupRaw(ctx context.Context, ek string) (data []byte, found bool, err error) {
	val, err := c.provider.Get(ctx, ek)
	if err == nil {
		if isNegativeSentinel(val) {
			c.metrics.negativeHits.Inc()
			return nil, false, ErrMissing
		}
		c.metrics.hits.Inc()
		return val, true, nil
	}
	if !errors.Is(err, ErrMissing) {
		c.metrics.errors.Inc()
		return nil, false, err
	}

	c.metrics.misses.Inc()
	return nil, false, nil
}

// runFallback executes fallback under the concurrency semaphore and the
// singleflight group, persisting the produced value (or the negative
// sentinel) exactly as the lookup flow expects.
func (c *Cache) runFallback(ctx context.Context, ek string, fallback Fallback) (any, error) {
	// Bound the total number of in-flight fallbacks across all keys.
	// singleflight only collapses requests for the SAME key — an
	// attacker driving distinct keys can launch one fallback per key
	// and starve the downstream. The semaphore provides backpressure
	// independent of key cardinality. Acquire honors ctx so callers see
	// timeouts rather than indefinite waits.
	if c.fallbackSem != nil {
		if err := c.fallbackSem.Acquire(ctx, 1); err != nil {
			c.metrics.errors.Inc()
			return nil, err
		}
		defer c.fallbackSem.Release(1)
	}

	fv, err, _ := c.group.Do(ek, func() (any, error) {
		stop := c.metrics.fallbackDuration.Start()
		val, ttl, fErr := fallback()
		stop()
		if fErr != nil {
			// ErrMissing from the fallback signals an authoritative
			// not-found. Write the negative-cache sentinel so a stream
			// of attacker-driven misses for the same key does NOT keep
			// stampeding the downstream — previously this branch
			// skipped the negative write and every subsequent miss
			// re-ran the fallback.
			if errors.Is(fErr, ErrMissing) && c.negativeTtl > 0 {
				if sErr := c.provider.Save(ctx, ek, negativeSentinel, c.negativeTtl); sErr != nil {
					c.metrics.errors.Inc()
				}
			}
			return nil, fErr
		}

		if vv := reflect.ValueOf(val); vv.Kind() == reflect.Pointer && vv.IsNil() {
			if c.negativeTtl > 0 {
				if sErr := c.provider.Save(ctx, ek, negativeSentinel, c.negativeTtl); sErr != nil {
					c.metrics.errors.Inc()
				}
			}
			return nil, ErrMissing
		}

		cacheData, fErr := c.serializer.Serialize(val)
		if fErr != nil {
			return nil, fErr
		}

		cacheTtl := c.ttl
		if ttl > TTLUseDefault {
			cacheTtl = ttl
		}

		if fErr = c.provider.Save(ctx, ek, cacheData, cacheTtl); fErr != nil {
			return nil, fErr
		}

		return val, nil
	})
	return fv, err
}

// Save stores a value in the cache with the given key.
// Panics if key is empty. Uses default TTL if none provided.
//
// Example:
//
//	err := c.Save(ctx, "user:123", &user, 10*time.Minute)
func (c *Cache) Save(ctx context.Context, key string, value any, ttl ...time.Duration) error {
	panics.Must(!strings.IsEmpty(key), "key must be provided")

	ctx, cancel := corecontext.WithDefault(ctx, defaultContextTimeout)
	defer cancel()

	cacheTtl := c.ttl
	if len(ttl) > 0 && ttl[0] > TTLUseDefault {
		cacheTtl = ttl[0]
	}

	cacheData, err := c.serializer.Serialize(value)
	if err != nil {
		return err
	}

	stop := c.metrics.writeDuration.Start()
	err = c.provider.Save(ctx, c.effectiveKey(ctx, key), cacheData, cacheTtl)
	stop()
	return err
}

// Exists checks if a key exists in the cache.
// Returns false without error if the key is not found. Panics if key is empty.
//
// Example:
//
//	exists, err := c.Exists(ctx, "user:123")
func (c *Cache) Exists(ctx context.Context, key string) (bool, error) {
	panics.Must(!strings.IsEmpty(key), "key must be provided")

	ctx, cancel := corecontext.WithDefault(ctx, defaultContextTimeout)
	defer cancel()

	data, err := c.provider.Get(ctx, c.effectiveKey(ctx, key))
	if err != nil {
		if errors.Is(err, ErrMissing) {
			return false, nil
		}
		return false, err
	}

	if isNegativeSentinel(data) {
		return false, nil
	}

	return true, nil
}

// Get retrieves a cached value by key and deserializes it into value.
// The value parameter must be a pointer. Panics if key is empty.
// Returns ErrMissing if the key does not exist.
//
// Example:
//
//	var user User
//	err := c.Get(ctx, "user:123", &user)
func (c *Cache) Get(ctx context.Context, key string, value any) error {
	panics.Must(!strings.IsEmpty(key), "key must be provided")
	pointerToStructAssertion(value)

	ctx, cancel := corecontext.WithDefault(ctx, defaultContextTimeout)
	defer cancel()

	data, err := c.provider.Get(ctx, c.effectiveKey(ctx, key))
	if err != nil {
		if errors.Is(err, ErrMissing) {
			c.metrics.misses.Inc()
		} else {
			c.metrics.errors.Inc()
		}
		return err
	}

	if isNegativeSentinel(data) {
		c.metrics.negativeHits.Inc()
		return ErrMissing
	}

	c.metrics.hits.Inc()
	return c.serializer.Deserialize(data, value)
}

func pointerToStructAssertion(value any) {
	val := reflect.ValueOf(value)

	panics.Must(val.Kind() == reflect.Pointer, "value must be pointer to struct")
}

// Delete removes a key from the cache.
// Panics if key is empty.
//
// Example:
//
//	err := c.Delete(ctx, "user:123")
func (c *Cache) Delete(ctx context.Context, key string) error {
	panics.Must(!strings.IsEmpty(key), "key must be provided")

	ctx, cancel := corecontext.WithDefault(ctx, defaultContextTimeout)
	defer cancel()

	return c.provider.Delete(ctx, c.effectiveKey(ctx, key))
}

// DeleteMany removes multiple keys from the cache in a single operation.
// Panics if no keys are provided.
//
// Example:
//
//	err := c.DeleteMany(ctx, "user:123", "user:456")
func (c *Cache) DeleteMany(ctx context.Context, key ...string) error {
	panics.Must(len(key) != 0, "one or more keys must be provided")

	ctx, cancel := corecontext.WithDefault(ctx, defaultContextTimeout)
	defer cancel()

	return c.provider.DeleteMany(ctx, c.effectiveKeys(ctx, key)...)
}
