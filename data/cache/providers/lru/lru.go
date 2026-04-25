// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package lru

import (
	"context"
	"time"

	"github.com/altessa-s/go-atlas/data/cache/lru"
	"github.com/altessa-s/go-atlas/data/cache/providers"
)

// item wraps a cache value with an expiration time.
type item struct {
	value     []byte
	expiresAt int64 // unix nano, 0 means no expiration
}

// Provider is an LRU-based cache provider for []byte data.
type Provider struct {
	cache lru.Cacher[string, item]
}

// New creates a new LRU provider with the given size and options.
func New(size int, opts ...lru.Option) (*Provider, error) {
	cache, err := lru.NewShardedCache[string, item](size, opts...)
	if err != nil {
		return nil, err
	}
	return &Provider{cache: cache}, nil
}

// Save stores a value with the given key and TTL.
func (p *Provider) Save(_ context.Context, key string, value []byte, ttl time.Duration) error {
	var expiresAt int64
	if ttl > 0 {
		expiresAt = time.Now().Add(ttl).UnixNano()
	}

	p.cache.Put(key, item{
		value:     value,
		expiresAt: expiresAt,
	})
	return nil
}

// Get retrieves a value by key. Returns ErrMissing if not found or expired.
// Expired items are treated as cache misses without eager removal to avoid a
// race where a concurrent Save could store a fresh value between the expiry
// check and the Remove call. The LRU eviction policy reclaims space instead.
func (p *Provider) Get(_ context.Context, key string) ([]byte, error) {
	it, ok := p.cache.Get(key)
	if !ok {
		return nil, providers.ErrMissing
	}

	if it.expiresAt > 0 && time.Now().UnixNano() > it.expiresAt {
		return nil, providers.ErrMissing
	}

	return it.value, nil
}

// Exists checks if a key exists and is not expired.
func (p *Provider) Exists(ctx context.Context, key string) (bool, error) {
	_, err := p.Get(ctx, key)
	if err != nil {
		return false, nil
	}
	return true, nil
}

// Delete removes a key from the cache.
func (p *Provider) Delete(_ context.Context, key string) error {
	p.cache.Remove(key)
	return nil
}

// DeleteMany removes multiple keys from the cache.
func (p *Provider) DeleteMany(_ context.Context, keys ...string) error {
	for _, key := range keys {
		p.cache.Remove(key)
	}
	return nil
}

var _ providers.Provider = (*Provider)(nil)
