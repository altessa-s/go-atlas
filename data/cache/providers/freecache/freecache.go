// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package freecache

import (
	"context"
	"errors"
	"time"

	"github.com/coocood/freecache"

	"github.com/altessa-s/go-atlas/core/runtime/concurrency"
	"github.com/altessa-s/go-atlas/data/cache/providers"
)

// Provider is an in-memory cache provider using FreeCache.
// It provides high-performance caching with zero GC overhead.
type Provider struct {
	*freecache.Cache
}

// New creates a new FreeCache provider with the given options.
// Default maximum size is 100MB.
//
// Example:
//
//	p := freecache.New(freecache.WithMaxSize(50 * 1024 * 1024))
func New(opt ...Option) *Provider {
	options := newOptions(opt...)

	p := &Provider{
		Cache: freecache.NewCache(options.maxSize),
	}

	return p
}

// Save stores the value under key with the given TTL.
func (p *Provider) Save(_ context.Context, key string, value []byte, ttl time.Duration) error {
	return p.Set([]byte(key), value, int(ttl.Seconds()))
}

// Exists reports whether key is present in the cache.
func (p *Provider) Exists(_ context.Context, key string) (bool, error) {
	_, err := p.Cache.Get([]byte(key))
	if err != nil {
		if errors.Is(err, freecache.ErrNotFound) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// Get retrieves the value for key. Returns [providers.ErrMissing] if the key does not exist.
func (p *Provider) Get(_ context.Context, key string) ([]byte, error) {
	data, err := p.Cache.Get([]byte(key))
	if err != nil {
		if errors.Is(err, freecache.ErrNotFound) {
			return nil, providers.ErrMissing
		}
		return nil, err
	}
	return data, nil
}

// Delete removes key from the cache.
func (p *Provider) Delete(_ context.Context, key string) error {
	p.Del([]byte(key))
	return nil
}

// DeleteMany removes multiple keys concurrently using [concurrency.Process].
func (p *Provider) DeleteMany(ctx context.Context, key ...string) error {
	// Use parallel processing for better performance with multiple keys.
	// freecache is thread-safe and highly concurrent.
	return concurrency.Process(ctx, key, func(_ context.Context, k string) error {
		p.Del([]byte(k))
		return nil
	}, concurrency.BatchConfig[string]{})
}

var _ providers.Provider = (*Provider)(nil)
