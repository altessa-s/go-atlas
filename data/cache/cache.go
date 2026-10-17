// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package cache

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"time"

	"github.com/altessa-s/go-atlas/core/encoding/serializer"
	"github.com/altessa-s/go-atlas/core/runtime/panics"
	"github.com/altessa-s/go-atlas/core/runtime/singleflight"
	"github.com/altessa-s/go-atlas/core/text/strings"
	"github.com/altessa-s/go-atlas/data/cache/storages/noop"

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
// Use the supplied shared context for I/O; it has the configured fallback timeout.
// If it returns a nil value, ErrMissing is returned to the caller.
type Fallback = func(context.Context) (value any, ttl time.Duration, err error)

// negativeSentinel is a single null byte stored as the value for negative cache entries.
// It cannot collide with JSON/msgpack serializer output.
var negativeSentinel = []byte{0x00}

func isNegativeSentinel(data []byte) bool {
	return len(data) == 1 && data[0] == 0x00
}

// Cache provides caching functionality with configurable providers and serializers.
// It uses singleflight to deduplicate concurrent requests for the same key
// and rejects new distinct work at capacity (see [DefaultMaxConcurrentFallbacks]).
type Cache struct {
	provider     Storage
	ttl          time.Duration
	negativeTtl  time.Duration
	group        *singleflight.Group[string, any]
	serializer   serializer.Serializer
	metrics      *cacheMetrics
	keyNamespace KeyNamespaceFunc // nil = no per-context namespacing

	fallbackTimeout time.Duration // bounds a fallback run outside the group (fail-open overload)
	failOpen        bool
	ioTimeout       time.Duration // bounds each storage call when positive
	isolated        bool          // collapsed callers share serialized bytes
}

// New creates a new Cache instance with the given provider and options.
// Panics if provider is nil. Default TTL is 1 hour with JSON serialization.
//
// Example:
//
//	c := cache.New(redisStorage, cache.WithTTL(10*time.Minute))
func New(p Storage, opts ...Option) *Cache {
	panics.MustNonNil(p, "provider must be provided")

	options := newOptions(opts...)

	c := &Cache{
		provider:    p,
		ttl:         options.ttl,
		negativeTtl: options.negativeTtl,
		group: singleflight.New[string, any](
			singleflight.WithMaxConcurrent(options.maxConcurrentFallbacks),
			singleflight.WithTimeout(options.fallbackTimeout)),
		serializer:   options.serializer,
		metrics:      newCacheMetrics(options.collector, options.name),
		keyNamespace: options.keyNamespace,

		fallbackTimeout: options.fallbackTimeout,
		failOpen:        options.failOpen,
		ioTimeout:       options.ioTimeout,
		isolated:        options.isolatedResults,
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

// effectiveKey encodes a namespace and key without ambiguous delimiters.
func (c *Cache) effectiveKey(ctx context.Context, key string) string {
	if c.keyNamespace == nil {
		return key
	}
	return namespacePrefix(c.keyNamespace(ctx)) + key
}

func namespacePrefix(ns string) string {
	return strconv.Itoa(len(ns)) + ":" + ns + ":"
}

func (c *Cache) effectiveKeys(ctx context.Context, keys []string) []string {
	if c.keyNamespace == nil {
		return keys
	}
	prefix := namespacePrefix(c.keyNamespace(ctx))
	out := make([]string, len(keys))
	for i, key := range keys {
		out[i] = prefix + key
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
//	err := c.GetWithFallback(ctx, "user:123", &user, func(work context.Context) (any, time.Duration, error) {
//		user, err := db.GetUser(work, 123)
//		return user, cache.TTLUseDefault, err
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
		err = c.serializer.Deserialize(data, value)
		if err == nil || !c.failOpen {
			return err
		}
		c.dropCorrupt(ctx, ek)
	}

	fv, err := c.runFallback(ctx, ek, fallback)
	if err != nil {
		return err
	}

	dst := reflect.Indirect(reflect.ValueOf(value))
	if c.isolated {
		// fv is this caller's view of the shared serialized bytes; decode a
		// private copy over a zeroed destination, mirroring the full overwrite
		// of the shared-value path.
		data, err := isolatedBytes(fv)
		if err != nil {
			return err
		}
		dst.SetZero()
		return c.serializer.Deserialize(data, value)
	}
	dst.Set(reflect.Indirect(reflect.ValueOf(fv)))

	return nil
}

// GetWithFallbackT is the typed variant of [Cache.GetWithFallback] for hot
// call paths: the destination is a value of T, so the per-call reflection of
// the any-based API (assignability assertion and copy-out) disappears. The
// untyped method remains for heterogeneous call sites.
//
// Example:
//
//	user, err := cache.GetWithFallbackT(ctx, c, "user:123", func(work context.Context) (User, time.Duration, error) {
//		user, err := db.GetUser(work, 123)
//		return user, cache.TTLUseDefault, err
//	})
func GetWithFallbackT[T any](ctx context.Context, c *Cache, key string, fallback func(context.Context) (T, time.Duration, error)) (T, error) {
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
		err = c.serializer.Deserialize(data, &out)
		if err == nil {
			return out, nil
		}
		if !c.failOpen {
			return out, err
		}
		c.dropCorrupt(ctx, ek)
		var zero T
		out = zero
	}

	// The boxing wrapper is built only on the miss path; hits stay free of
	// per-call closures.
	fv, err := c.runFallback(ctx, ek, func(work context.Context) (any, time.Duration, error) {
		return fallback(work)
	})
	if err != nil {
		return out, err
	}

	if c.isolated {
		raw, err := isolatedBytes(fv)
		if err != nil {
			return out, err
		}
		if err = c.serializer.Deserialize(raw, &out); err != nil {
			var zero T
			return zero, err
		}
		return out, nil
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
// caller should run the fallback path. In fail-open mode a storage error or
// timeout is also reported as a plain miss.
func (c *Cache) lookupRaw(ctx context.Context, ek string) (data []byte, found bool, err error) {
	val, err := c.storageGet(ctx, ek)
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
		if c.failOpen && ctx.Err() == nil {
			c.metrics.failOpenGet.Inc()
			return nil, false, nil
		}
		return nil, false, err
	}

	c.metrics.misses.Inc()
	return nil, false, nil
}

// storageGet reads ek from the provider, bounded by the fail-open I/O timeout.
func (c *Cache) storageGet(ctx context.Context, ek string) ([]byte, error) {
	ioCtx, cancel := c.ioContext(ctx)
	defer cancel()
	return c.provider.Get(ioCtx, ek)
}

// storageSave writes ek to the provider, bounded by the fail-open I/O timeout.
func (c *Cache) storageSave(ctx context.Context, ek string, data []byte, ttl time.Duration) error {
	ioCtx, cancel := c.ioContext(ctx)
	defer cancel()
	return c.provider.Save(ioCtx, ek, data, ttl)
}

// ioContext applies the fail-open I/O timeout to ctx when one is configured.
func (c *Cache) ioContext(ctx context.Context) (context.Context, context.CancelFunc) {
	if c.ioTimeout > 0 {
		return context.WithTimeout(ctx, c.ioTimeout)
	}
	return ctx, func() {}
}

// dropCorrupt records a stored value that failed to decode in fail-open mode
// and deletes it best-effort so the next read reloads it.
func (c *Cache) dropCorrupt(ctx context.Context, ek string) {
	c.metrics.errors.Inc()
	c.metrics.failOpenDecode.Inc()
	ioCtx, cancel := c.ioContext(ctx)
	defer cancel()
	_ = c.provider.Delete(ioCtx, ek)
}

// failOpenSaveError reports whether a failed write may be ignored: the cache
// is fail-open and the caller has not gone away. It records the event.
func (c *Cache) failOpenSaveError(ctx context.Context) bool {
	if !c.failOpen || ctx.Err() != nil {
		return false
	}
	c.metrics.failOpenSave.Inc()
	return true
}

// runFallback executes fallback under bounded admission and the
// singleflight group, persisting the produced value (or the negative
// sentinel) exactly as the lookup flow expects. With isolated results the
// shared value is the serialized []byte, not the loaded value. In fail-open
// mode a call rejected with [ErrOverloaded] runs the fallback directly in the
// caller, without sharing it.
func (c *Cache) runFallback(ctx context.Context, ek string, fallback Fallback) (any, error) {
	v, err := c.group.Do(ctx, ek, func(sharedCtx context.Context) (any, error) {
		return c.loadAndStore(sharedCtx, ek, fallback)
	})
	if !c.failOpen || !errors.Is(err, ErrOverloaded) {
		return v, err
	}

	c.metrics.failOpenOverload.Inc()
	if c.fallbackTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.fallbackTimeout)
		defer cancel()
	}
	return c.loadAndStore(ctx, ek, fallback)
}

// loadAndStore runs fallback and writes its result (or the negative sentinel)
// under ek. It returns the loaded value, or its serialized bytes when results
// are isolated.
func (c *Cache) loadAndStore(ctx context.Context, ek string, fallback Fallback) (any, error) {
	stop := c.metrics.fallbackDuration.Start()
	val, ttl, fErr := fallback(ctx)
	stop()
	if fErr != nil {
		// ErrMissing from the fallback signals an authoritative
		// not-found. Write the negative-cache sentinel so a stream
		// of attacker-driven misses for the same key does NOT keep
		// stampeding the downstream — previously this branch
		// skipped the negative write and every subsequent miss
		// re-ran the fallback.
		if errors.Is(fErr, ErrMissing) && c.negativeTtl > 0 {
			if sErr := c.storageSave(ctx, ek, negativeSentinel, c.negativeTtl); sErr != nil {
				c.metrics.errors.Inc()
			}
		}
		return nil, fErr
	}

	if vv := reflect.ValueOf(val); val == nil || (vv.Kind() == reflect.Pointer && vv.IsNil()) {
		if c.negativeTtl > 0 {
			if sErr := c.storageSave(ctx, ek, negativeSentinel, c.negativeTtl); sErr != nil {
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

	if fErr = c.storageSave(ctx, ek, cacheData, cacheTtl); fErr != nil {
		if !c.failOpen {
			return nil, fErr
		}
		// The loaded value is still good: count the failed write and
		// hand the value to the callers.
		c.metrics.errors.Inc()
		c.metrics.failOpenSave.Inc()
	}

	if c.isolated {
		return cacheData, nil
	}
	return val, nil
}

// isolatedBytes unwraps a fallback result in isolated mode, where
// [Cache.runFallback] hands every collapsed caller the serialized bytes.
func isolatedBytes(fv any) ([]byte, error) {
	data, ok := fv.([]byte)
	if !ok {
		return nil, fmt.Errorf("cache: isolated fallback result is %T, want []byte", fv)
	}
	return data, nil
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
	err = c.storageSave(ctx, c.effectiveKey(ctx, key), cacheData, cacheTtl)
	stop()
	if err != nil && c.failOpenSaveError(ctx) {
		c.metrics.errors.Inc()
		return nil
	}
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

	ek := c.effectiveKey(ctx, key)
	data, err := c.storageGet(ctx, ek)
	if err != nil {
		if errors.Is(err, ErrMissing) {
			c.metrics.misses.Inc()
		} else {
			c.metrics.errors.Inc()
			if c.failOpen && ctx.Err() == nil {
				c.metrics.failOpenGet.Inc()
				return ErrMissing
			}
		}
		return err
	}

	if isNegativeSentinel(data) {
		c.metrics.negativeHits.Inc()
		return ErrMissing
	}

	c.metrics.hits.Inc()
	err = c.serializer.Deserialize(data, value)
	if err != nil && c.failOpen {
		c.dropCorrupt(ctx, ek)
		return ErrMissing
	}
	return err
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
