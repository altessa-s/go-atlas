// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package lru

import (
	"bytes"
	"context"
	"errors"
	"sync"
	"time"

	"github.com/altessa-s/go-atlas/cache/lru"
	"github.com/altessa-s/go-atlas/cache/storages"
)

// item wraps a cache value with an expiration time.
type item struct {
	value     []byte
	expiresAt int64 // unix nano, 0 means no expiration
}

// Storage is an LRU-based cache provider for []byte data.
type Storage struct {
	cache lru.Cacher[string, item]
	// Ordinary writers can proceed concurrently while a compare-and-delete
	// excludes all writes for its read/delete pair.
	mutations sync.RWMutex
}

// New creates a new LRU provider with the given size and options.
// Capacity and eviction follow [lru.NewShardedCache].
func New(size int, opts ...lru.Option) (*Storage, error) {
	cache, err := lru.NewShardedCache[string, item](size, opts...)
	if err != nil {
		return nil, err
	}
	return &Storage{cache: cache}, nil
}

// Save stores a value with the given key and TTL.
func (p *Storage) Save(_ context.Context, key string, value []byte, ttl time.Duration) error {
	var expiresAt int64
	if ttl > 0 {
		expiresAt = time.Now().Add(ttl).UnixNano()
	}

	data := bytes.Clone(value)
	p.mutations.RLock()
	defer p.mutations.RUnlock()
	p.cache.Put(key, item{
		value:     data,
		expiresAt: expiresAt,
	})
	return nil
}

// Get retrieves a value by key. Returns ErrMissing if not found or expired.
// Expired items are treated as cache misses without eager removal to avoid a
// race where a concurrent Save could store a fresh value between the expiry
// check and the Remove call. The LRU eviction policy reclaims space instead.
func (p *Storage) Get(_ context.Context, key string) ([]byte, error) {
	it, ok := p.get(key)
	if !ok {
		return nil, storages.ErrMissing
	}
	return bytes.Clone(it.value), nil
}

func (p *Storage) get(key string) (item, bool) {
	it, ok := p.cache.Get(key)
	return it, ok && (it.expiresAt <= 0 || time.Now().UnixNano() <= it.expiresAt)
}

// Exists checks if a key exists and is not expired.
func (p *Storage) Exists(_ context.Context, key string) (bool, error) {
	_, ok := p.get(key)
	return ok, nil
}

// CompareAndDelete implements [storages.CompareDeleter].
func (p *Storage) CompareAndDelete(ctx context.Context, key string, expected []byte) (bool, error) {
	p.mutations.Lock()
	defer p.mutations.Unlock()
	if err := ctx.Err(); err != nil {
		return false, err
	}
	it, ok := p.get(key)
	if !ok || !bytes.Equal(it.value, expected) {
		return false, nil
	}
	return p.cache.Remove(key), nil
}

// Delete removes a key from the cache.
func (p *Storage) Delete(_ context.Context, key string) error {
	p.mutations.RLock()
	defer p.mutations.RUnlock()
	p.cache.Remove(key)
	return nil
}

// DeleteMany removes multiple keys from the cache.
func (p *Storage) DeleteMany(_ context.Context, keys ...string) error {
	p.mutations.RLock()
	defer p.mutations.RUnlock()
	for _, key := range keys {
		p.cache.Remove(key)
	}
	return nil
}

// GetMany retrieves the values of the keys that are present and not expired.
// Missing keys are absent from the returned map.
func (p *Storage) GetMany(ctx context.Context, keys []string) (map[string][]byte, error) {
	out := make(map[string][]byte, len(keys))
	for _, key := range keys {
		val, err := p.Get(ctx, key)
		if err != nil {
			if errors.Is(err, storages.ErrMissing) {
				continue
			}
			return nil, err
		}
		out[key] = val
	}
	return out, nil
}

// InProcess reports true: the LRU lives in process memory, so the cache adds no
// default timeout to its calls (see [storages.InProcess]).
func (p *Storage) InProcess() bool { return true }

var (
	_ storages.Storage        = (*Storage)(nil)
	_ storages.MultiGetter    = (*Storage)(nil)
	_ storages.CompareDeleter = (*Storage)(nil)
	_ storages.InProcess      = (*Storage)(nil)
)
