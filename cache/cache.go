// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package cache

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"time"

	"github.com/altessa-s/go-atlas-core/encoding/serializer"
	"github.com/altessa-s/go-atlas-core/runtime/panics"
	"github.com/altessa-s/go-atlas-core/runtime/singleflight"
	"github.com/altessa-s/go-atlas-core/text/strings"
	"github.com/altessa-s/go-atlas/cache/storages"
	"github.com/altessa-s/go-atlas/cache/storages/noop"

	corecontext "github.com/altessa-s/go-atlas-core/context"
)

// NoTTL is the TTL value indicating that a cache item will not expire.
const NoTTL time.Duration = 0

// TTLUseDefault is the TTL value indicating that the cache's default TTL should be used.
const TTLUseDefault time.Duration = -1

// defaultContextTimeout bounds an operation whose context has no deadline.
// It is skipped for a [storages.InProcess] storage without key versioning
// (see [Cache.opTimeout]).
const defaultContextTimeout = 15 * time.Second

// Fallback is a function called when a cache key is not found.
// It returns the value to cache, optional TTL (use TTLUseDefault for cache default), and any error.
// Use the supplied shared context for I/O; it has the configured fallback timeout.
// If it returns a nil value, ErrMissing is returned to the caller.
type Fallback = func(context.Context) (value any, ttl time.Duration, err error)

// negativeSentinel is a single null byte stored as the value for negative cache entries.
// Positive payloads beginning with a null byte escape their first byte.
var negativeSentinel = []byte{0x00}

func isNegativeSentinel(data []byte) bool {
	return len(data) == 1 && data[0] == 0x00
}

// encodePayload escapes a leading zero so that even a one-byte zero payload
// can be stored alongside negative entries. JSON never needs escaping.
func encodePayload(data []byte) []byte {
	if len(data) != 0 && data[0] == 0 {
		return append([]byte{0}, data...)
	}
	return data
}

// decodePayload recovers the serialized bytes of a positive entry.
func decodePayload(data []byte) []byte {
	if len(data) >= 2 && data[0] == 0 && data[1] == 0 {
		return data[1:]
	}
	return data
}

// Cache provides caching functionality with configurable providers and serializers.
// It uses singleflight to deduplicate concurrent requests for the same key
// and rejects new distinct work at capacity (see [DefaultMaxConcurrentFallbacks]).
type Cache struct {
	provider     storages.Storage
	ttl          time.Duration
	negativeTtl  time.Duration
	group        *singleflight.Group[string, any]
	serializer   serializer.Serializer
	metrics      *cacheMetrics
	formatPrefix string           // separates codec formats and optional key components
	customCodec  bool             // the caller-supplied codec may retain input buffers
	keyNamespace KeyNamespaceFunc // nil = no per-context namespacing

	fallbackTimeout time.Duration // bounds a fallback run outside the group (fail-open overload)
	failOpen        bool
	ioTimeout       time.Duration // bounds each storage call when positive
	isolated        bool          // collapsed callers share serialized bytes

	keyVersion KeyVersionFunc              // nil = unversioned keys
	invalidate func(context.Context) error // nil = InvalidateAll unsupported

	opTimeout time.Duration // default bound of an operation without a deadline; zero = none

	loads inflightLoads // running fallback loads, consulted by Delete and DeleteMany
}

// New creates a new Cache instance with the given provider and options.
// Panics if provider is nil. Default TTL is 1 hour with JSON serialization.
//
// Example:
//
//	c := cache.New(redisStorage, cache.WithTTL(10*time.Minute))
func New(p storages.Storage, opts ...Option) *Cache {
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
		metrics:      newCacheMetrics(options.meter, options.name),
		keyNamespace: options.keyNamespace,

		fallbackTimeout: options.fallbackTimeout,
		failOpen:        options.failOpen,
		ioTimeout:       options.ioTimeout,
		isolated:        options.isolatedResults,

		keyVersion: options.keyVersion,
		invalidate: options.invalidate,

		opTimeout: opTimeout(p, options.keyVersion),
	}
	// Both formats need a tag because arbitrary JSON cache keys could occupy
	// the prefix reserved for custom codecs. Presence bits keep an unversioned
	// key from looking like a versioned or namespaced key for another cache.
	// Use a single byte to minimize overhead in size-limited storages.
	format := byte('0')
	if _, jsonCodec := c.serializer.(*serializer.JSON); !jsonCodec {
		format += 4
		c.customCodec = true
	}
	if c.keyVersion != nil {
		format++
	}
	if c.keyNamespace != nil {
		format += 2
	}
	c.formatPrefix = string(format)
	return c
}

// opTimeout returns the default timeout of an operation whose context has no
// deadline. An in-process storage cannot block on I/O, so it needs none unless
// a key version lookup, which may be remote, runs under the same context.
func opTimeout(p storages.Storage, keyVersion KeyVersionFunc) time.Duration {
	if ip, ok := p.(storages.InProcess); ok && ip.InProcess() && keyVersion == nil {
		return 0
	}
	return defaultContextTimeout
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

// effectiveKey prepends the codec format prefix, the optional "<version>:"
// and the optional "<len>:<namespace>:" to key. An error means the key
// version is unknown.
func (c *Cache) effectiveKey(ctx context.Context, key string) (string, error) {
	prefix, err := c.keyPrefix(ctx)
	if err != nil {
		return "", err
	}
	return prefix + key, nil
}

// keyPrefix resolves the common storage prefix once per operation.
func (c *Cache) keyPrefix(ctx context.Context) (string, error) {
	version, err := c.versionPrefix(ctx)
	if err != nil {
		return "", err
	}
	if c.keyNamespace == nil {
		return c.formatPrefix + version, nil
	}
	return c.formatPrefix + version + namespacePrefix(c.keyNamespace(ctx)), nil
}

func namespacePrefix(ns string) string {
	return strconv.Itoa(len(ns)) + ":" + ns + ":"
}

func (c *Cache) effectiveKeys(ctx context.Context, keys []string) ([]string, error) {
	prefix, err := c.keyPrefix(ctx)
	if err != nil {
		return keys, err
	}
	out := make([]string, len(keys))
	for i, key := range keys {
		out[i] = prefix + key
	}
	return out, nil
}

// GetWithFallback retrieves a cached value or calls fallback if not found.
// The value parameter must be a pointer. Panics if key is empty or fallback is nil.
// Uses singleflight to prevent duplicate fallback calls for the same key.
// A context marked by [WithFreshReads] skips the cache: the fallback runs
// directly in the caller and nothing is stored.
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

	ctx, cancel := corecontext.WithDefault(ctx, c.opTimeout)
	defer cancel()

	dst := reflect.Indirect(reflect.ValueOf(value))

	if FreshReads(ctx) {
		fv, err := c.directFallback(ctx, fallback)
		if err != nil {
			return err
		}
		dst.Set(reflect.Indirect(reflect.ValueOf(fv)))
		return nil
	}

	// Namespace the key for both the backend and singleflight so concurrent
	// callers from different tenants never collapse onto one another's result.
	ek, err := c.effectiveKey(ctx, key)
	if err != nil {
		// Unknown key version: read the source of truth, store nothing.
		fv, fErr := c.bypassFallback(ctx, fallback)
		if fErr != nil {
			return fErr
		}
		dst.Set(reflect.Indirect(reflect.ValueOf(fv)))
		return nil
	}

	data, found, err := c.lookupRaw(ctx, ek)
	if err != nil {
		return err
	}
	if found {
		err = c.deserialize(decodePayload(data), value)
		if err == nil || !c.failOpen {
			return err
		}
		c.dropCorrupt(ctx, ek, data)
	}

	ld, err := c.runFallback(ctx, ek, fallback)
	if err != nil {
		return err
	}

	if c.isolated {
		// Decode a private copy of the shared serialized bytes over a zeroed
		// destination, mirroring the full overwrite of the shared-value path.
		dst.SetZero()
		return c.deserialize(ld.data, value)
	}
	dst.Set(reflect.Indirect(reflect.ValueOf(ld.value)))

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
	return getWithFallbackT(ctx, c, key, c.isolated, fallback)
}

// getWithFallbackT implements [GetWithFallbackT]; isolated forces every
// caller to decode its own copy whatever the cache's [WithIsolatedResults].
func getWithFallbackT[T any](
	ctx context.Context, c *Cache, key string, isolated bool, fallback func(context.Context) (T, time.Duration, error),
) (T, error) {
	var out T
	panics.Must(!strings.IsEmpty(key), "key must be provided")
	panics.MustNonNil(fallback, "fallback must be provided")

	ctx, cancel := corecontext.WithDefault(ctx, c.opTimeout)
	defer cancel()

	if FreshReads(ctx) {
		fv, err := c.directFallback(ctx, func(work context.Context) (any, time.Duration, error) {
			return fallback(work)
		})
		if err != nil {
			return out, err
		}
		v, _ := fv.(T) // fv is the fallback's own T
		return v, nil
	}

	ek, err := c.effectiveKey(ctx, key)
	if err != nil {
		// Unknown key version: read the source of truth, store nothing.
		fv, fErr := c.bypassFallback(ctx, func(work context.Context) (any, time.Duration, error) {
			return fallback(work)
		})
		if fErr != nil {
			return out, fErr
		}
		v, _ := fv.(T) // fv is the fallback's own T
		return v, nil
	}

	data, found, err := c.lookupRaw(ctx, ek)
	if err != nil {
		return out, err
	}
	if found {
		err = c.deserialize(decodePayload(data), &out)
		if err == nil {
			return out, nil
		}
		if !c.failOpen {
			return out, err
		}
		c.dropCorrupt(ctx, ek, data)
		var zero T
		out = zero
	}

	// The boxing wrapper is built only on the miss path; hits stay free of
	// per-call closures.
	ld, err := c.runFallback(ctx, ek, func(work context.Context) (any, time.Duration, error) {
		return fallback(work)
	})
	if err != nil {
		return out, err
	}

	if isolated {
		if err = c.deserialize(ld.data, &out); err != nil {
			var zero T
			return zero, err
		}
		return out, nil
	}

	// A concurrent untyped caller may have won the singleflight with a *T:
	// accept both shapes, mirroring the reflect.Indirect copy-out of the
	// untyped method.
	if v, ok := ld.value.(T); ok {
		return v, nil
	}
	if p, ok := ld.value.(*T); ok && p != nil {
		return *p, nil
	}
	return out, fmt.Errorf("cache: singleflight value type %T does not match requested type %T", ld.value, out)
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

// ioContext applies the fail-open I/O timeout to ctx when one is configured.
func (c *Cache) ioContext(ctx context.Context) (context.Context, context.CancelFunc) {
	if c.ioTimeout > 0 {
		return context.WithTimeout(ctx, c.ioTimeout)
	}
	return ctx, func() {}
}

// dropCorrupt records a decode failure and removes only the bytes that failed.
func (c *Cache) dropCorrupt(ctx context.Context, ek string, encoded []byte) {
	c.metrics.errors.Inc()
	c.metrics.failOpenDecode.Inc()
	deleter, ok := c.provider.(storages.CompareDeleter)
	if !ok {
		return
	}
	ioCtx, cancel := c.ioContext(ctx)
	defer cancel()
	_, _ = deleter.CompareAndDelete(ioCtx, ek, encoded)
}

// Custom codecs may borrow their input or return a slice of it. Snapshot their
// output for storage and give every decoder its own input buffer. Our JSON
// codec already owns Marshal's output and preserves no reference to its input.
func (c *Cache) serialize(value any) ([]byte, error) {
	data, err := c.serializer.Serialize(value)
	if err == nil && c.customCodec {
		data = bytes.Clone(data)
	}
	return data, err
}

func (c *Cache) deserialize(data []byte, value any) error {
	if c.customCodec {
		data = bytes.Clone(data)
	}
	return c.serializer.Deserialize(data, value)
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

// loaded is the result a fallback run shares with every collapsed caller:
// the loaded value and its serialized bytes. A caller that must not share
// memory with the others decodes data; the others use value.
type loaded struct {
	value any
	data  []byte
}

// runFallback executes fallback under bounded admission and the
// singleflight group, persisting the produced value (or the negative
// sentinel) exactly as the lookup flow expects. In fail-open mode a call
// rejected with [ErrOverloaded] runs the fallback directly in the caller,
// without sharing it.
func (c *Cache) runFallback(ctx context.Context, ek string, fallback Fallback) (*loaded, error) {
	v, err := c.group.Do(ctx, ek, func(sharedCtx context.Context) (any, error) {
		return c.loadAndStore(sharedCtx, ek, fallback)
	})
	if !c.failOpen || !errors.Is(err, ErrOverloaded) {
		if err != nil {
			return nil, err
		}
		ld, _ := v.(*loaded) // the group only ever shares *loaded
		return ld, nil
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
// under ek. It returns the loaded value with its serialized bytes. A load
// invalidated by Delete or a successful Save still returns its value.
func (c *Cache) loadAndStore(ctx context.Context, ek string, fallback Fallback) (*loaded, error) {
	guard := c.loads.begin(ek)
	defer c.loads.end(ek, guard)

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
			if sErr := c.storeLoaded(ctx, guard, ek, negativeSentinel, c.negativeTtl); sErr != nil {
				c.metrics.errors.Inc()
			}
		}
		return nil, fErr
	}

	if isNilValue(val) {
		if c.negativeTtl > 0 {
			if sErr := c.storeLoaded(ctx, guard, ek, negativeSentinel, c.negativeTtl); sErr != nil {
				c.metrics.errors.Inc()
			}
		}
		return nil, ErrMissing
	}

	cacheData, fErr := c.serialize(val)
	if fErr != nil {
		return nil, fErr
	}

	cacheTtl := c.ttl
	if ttl > TTLUseDefault {
		cacheTtl = ttl
	}

	if fErr = c.storeLoaded(ctx, guard, ek, encodePayload(cacheData), cacheTtl); fErr != nil {
		if !c.failOpen {
			return nil, fErr
		}
		// The loaded value is still good: count the failed write and
		// hand the value to the callers.
		c.metrics.errors.Inc()
		c.metrics.failOpenSave.Inc()
	}

	return &loaded{value: val, data: cacheData}, nil
}

// storeLoaded serializes fallback writes with explicit mutations of the key.
func (c *Cache) storeLoaded(ctx context.Context, guard *loadGuard, ek string, data []byte, ttl time.Duration) error {
	ioCtx, cancel := c.ioContext(ctx)
	defer cancel()
	if err := guard.lock(ioCtx); err != nil {
		return err
	}
	defer guard.unlock()
	if guard.invalidated {
		return nil
	}
	return c.provider.Save(ioCtx, ek, data, ttl)
}

// isNilValue reports whether a fallback returned no value: nil or a nil
// pointer.
func isNilValue(val any) bool {
	vv := reflect.ValueOf(val)
	return val == nil || (vv.Kind() == reflect.Pointer && vv.IsNil())
}

// Save stores a value in the cache with the given key.
// Panics if key is empty. Uses default TTL if none provided. A successful
// write suppresses storage of overlapping fallback results.
//
// Example:
//
//	err := c.Save(ctx, "user:123", &user, 10*time.Minute)
func (c *Cache) Save(ctx context.Context, key string, value any, ttl ...time.Duration) error {
	panics.Must(!strings.IsEmpty(key), "key must be provided")

	ctx, cancel := corecontext.WithDefault(ctx, c.opTimeout)
	defer cancel()

	cacheTtl := c.ttl
	if len(ttl) > 0 && ttl[0] > TTLUseDefault {
		cacheTtl = ttl[0]
	}

	cacheData, err := c.serialize(value)
	if err != nil {
		return err
	}

	ek, err := c.effectiveKey(ctx, key)
	if err != nil {
		if c.versionUnavailable(ctx) {
			return nil
		}
		return err
	}

	stop := c.metrics.writeDuration.Start()
	err = c.saveExplicit(ctx, ek, encodePayload(cacheData), cacheTtl)
	stop()
	if err != nil && c.failOpenSaveError(ctx) {
		c.metrics.errors.Inc()
		return nil
	}
	return err
}

func (c *Cache) saveExplicit(ctx context.Context, key string, data []byte, ttl time.Duration) error {
	guard := c.loads.begin(key)
	defer c.loads.end(key, guard)
	ioCtx, cancel := c.ioContext(ctx)
	defer cancel()
	if err := guard.lock(ioCtx); err != nil {
		return err
	}
	defer guard.unlock()
	if err := c.provider.Save(ioCtx, key, data, ttl); err != nil {
		return err
	}
	guard.invalidated = true
	return nil
}

// Exists checks if a key exists in the cache.
// Returns false without error if the key is not found. Panics if key is empty.
//
// Example:
//
//	exists, err := c.Exists(ctx, "user:123")
func (c *Cache) Exists(ctx context.Context, key string) (bool, error) {
	panics.Must(!strings.IsEmpty(key), "key must be provided")

	ctx, cancel := corecontext.WithDefault(ctx, c.opTimeout)
	defer cancel()

	ek, err := c.effectiveKey(ctx, key)
	if err != nil {
		if c.versionUnavailable(ctx) {
			return false, nil
		}
		return false, err
	}

	_, found, err := c.lookupRaw(ctx, ek)
	if errors.Is(err, ErrMissing) {
		return false, nil
	}
	return found, err
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

	ctx, cancel := corecontext.WithDefault(ctx, c.opTimeout)
	defer cancel()

	ek, err := c.effectiveKey(ctx, key)
	if err != nil {
		if c.versionUnavailable(ctx) {
			return ErrMissing
		}
		return err
	}
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
	err = c.deserialize(decodePayload(data), value)
	if err != nil && c.failOpen {
		c.dropCorrupt(ctx, ek, data)
		return ErrMissing
	}
	return err
}

func pointerToStructAssertion(value any) {
	val := reflect.ValueOf(value)

	panics.Must(val.Kind() == reflect.Pointer, "value must be pointer to struct")
}

// Delete removes a key from the cache.
// Panics if key is empty. A fallback load of the key that is still running
// returns its value to its callers but does not store it.
//
// Example:
//
//	err := c.Delete(ctx, "user:123")
func (c *Cache) Delete(ctx context.Context, key string) error {
	panics.Must(!strings.IsEmpty(key), "key must be provided")

	ctx, cancel := corecontext.WithDefault(ctx, c.opTimeout)
	defer cancel()

	ek, err := c.effectiveKey(ctx, key)
	if err != nil {
		return err
	}
	ioCtx, ioCancel := c.ioContext(ctx)
	defer ioCancel()
	return c.loads.invalidate(ioCtx, []string{ek}, func() error {
		return c.provider.Delete(ioCtx, ek)
	})
}

// DeleteMany removes multiple keys from the cache in a single operation.
// Panics if no keys are provided. Running fallback loads of the keys are
// handled as in [Cache.Delete].
//
// Example:
//
//	err := c.DeleteMany(ctx, "user:123", "user:456")
func (c *Cache) DeleteMany(ctx context.Context, key ...string) error {
	panics.Must(len(key) != 0, "one or more keys must be provided")

	ctx, cancel := corecontext.WithDefault(ctx, c.opTimeout)
	defer cancel()

	eks, err := c.effectiveKeys(ctx, key)
	if err != nil {
		return err
	}
	ioCtx, ioCancel := c.ioContext(ctx)
	defer ioCancel()
	return c.loads.invalidate(ioCtx, eks, func() error {
		return c.provider.DeleteMany(ioCtx, eks...)
	})
}
