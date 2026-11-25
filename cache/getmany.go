// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package cache

import (
	"context"
	"errors"

	"github.com/altessa-s/go-atlas-core/runtime/panics"
	"github.com/altessa-s/go-atlas-core/text/strings"
	"github.com/altessa-s/go-atlas/cache/storages"

	corecontext "github.com/altessa-s/go-atlas-core/context"
)

// GetManyRaw reads several keys at once and returns the serialized bytes of the
// keys that were found, keyed by the caller's keys (namespacing from
// [WithKeyNamespace] is applied to the storage keys only). Missing keys and
// negatively cached keys are absent from the result. Duplicate keys are read
// once. Callers own the returned bytes. Panics if any key is empty.
//
// The read is one storage call when the storage implements
// [storages.MultiGetter], otherwise a loop of Get calls. In fail-open mode
// ([WithFailOpen]) a failed or timed-out read leaves the affected keys out of
// the result instead of returning an error.
//
// Example:
//
//	raw, err := c.GetManyRaw(ctx, []string{"user:1", "user:2"})
func (c *Cache) GetManyRaw(ctx context.Context, keys []string) (map[string][]byte, error) {
	ctx, cancel := corecontext.WithDefault(ctx, c.opTimeout)
	defer cancel()

	out, _, err := c.getManyRaw(ctx, keys)
	for key, data := range out {
		out[key] = decodePayload(data)
	}
	return out, err
}

// getManyRaw reads encoded bytes and resolves the effective storage keys
// once, so cleanup can match exactly what was read.
func (c *Cache) getManyRaw(ctx context.Context, keys []string) (map[string][]byte, string, error) {
	for _, key := range keys {
		panics.Must(!strings.IsEmpty(key), "key must be provided")
	}

	out := make(map[string][]byte, len(keys))
	if len(keys) == 0 {
		return out, "", nil
	}

	prefix, err := c.keyPrefix(ctx)
	if err != nil {
		if c.versionUnavailable(ctx) {
			return out, "", nil
		}
		return nil, "", err
	}
	byEffective := make(map[string]string, len(keys))
	unique := make([]string, 0, len(keys))
	for _, key := range keys {
		ek := prefix + key
		if _, ok := byEffective[ek]; ok {
			continue
		}
		byEffective[ek] = key
		unique = append(unique, ek)
	}

	found, err := c.storageGetMany(ctx, unique)
	if err != nil {
		return nil, "", err
	}

	for _, ek := range unique {
		val, ok := found[ek]
		switch {
		case !ok:
			c.metrics.misses.Inc()
		case isNegativeSentinel(val):
			c.metrics.negativeHits.Inc()
		default:
			c.metrics.hits.Inc()
			out[byEffective[ek]] = val
		}
	}
	return out, prefix, nil
}

// storageGetMany reads eks, keyed by effective key, through
// [storages.MultiGetter] when the provider has it and through Get otherwise.
// Keys whose read failed in fail-open mode are absent; their failures are
// recorded.
func (c *Cache) storageGetMany(ctx context.Context, eks []string) (map[string][]byte, error) {
	if mg, ok := c.provider.(storages.MultiGetter); ok {
		ioCtx, cancel := c.ioContext(ctx)
		defer cancel()
		found, err := mg.GetMany(ioCtx, eks)
		if err == nil {
			return found, nil
		}
		c.metrics.errors.Inc()
		if c.failOpen && ctx.Err() == nil {
			c.metrics.failOpenGet.Inc()
			return map[string][]byte{}, nil
		}
		return nil, err
	}

	found := make(map[string][]byte, len(eks))
	for _, ek := range eks {
		val, err := c.storageGet(ctx, ek)
		switch {
		case err == nil:
			found[ek] = val
		case errors.Is(err, ErrMissing):
		default:
			c.metrics.errors.Inc()
			if !c.failOpen || ctx.Err() != nil {
				return nil, err
			}
			c.metrics.failOpenGet.Inc()
		}
	}
	return found, nil
}

// GetManyT is the typed variant of [Cache.GetManyRaw]: every hit is decoded
// into its own value of T, so no two entries share memory. A value that cannot
// be decoded returns the decode error; in fail-open mode ([WithFailOpen]) it
// is a miss for that key only. See [storages.CompareDeleter] for cleanup.
//
// Example:
//
//	users, err := cache.GetManyT[User](ctx, c, []string{"user:1", "user:2"})
func GetManyT[T any](ctx context.Context, c *Cache, keys []string) (map[string]T, error) {
	ctx, cancel := corecontext.WithDefault(ctx, c.opTimeout)
	defer cancel()

	raw, prefix, err := c.getManyRaw(ctx, keys)
	if err != nil {
		return nil, err
	}

	out := make(map[string]T, len(raw))
	for key, data := range raw {
		var v T
		if err = c.deserialize(decodePayload(data), &v); err != nil {
			if !c.failOpen {
				return nil, err
			}
			c.dropCorrupt(ctx, prefix+key, data)
			continue
		}
		out[key] = v
	}
	return out, nil
}
