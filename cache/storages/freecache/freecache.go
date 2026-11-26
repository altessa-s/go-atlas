// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package freecache

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/coocood/freecache"

	"github.com/altessa-s/go-atlas/cache/storages"
)

// MaxTTL is ten 365-day years. Its seconds fit a 32-bit int and leave room
// for FreeCache's uint32 absolute Unix deadline until 2096.
const MaxTTL = 10 * 365 * 24 * time.Hour

// ErrTTLOutOfRange indicates a positive TTL above [MaxTTL].
var ErrTTLOutOfRange = errors.New("freecache: TTL out of range")

// Storage is an in-memory cache provider using FreeCache.
// It provides high-performance caching with zero GC overhead.
type Storage struct {
	cache *freecache.Cache
}

// New creates a new FreeCache provider with the given options.
// Default maximum size is 100MB.
//
// Example:
//
//	p := freecache.New(freecache.WithMaxSize(50 * 1024 * 1024))
func New(opt ...Option) *Storage {
	options := newOptions(opt...)
	return &Storage{cache: freecache.NewCache(options.maxSize)}
}

// Save stores the value under key with the given TTL.
// Positive TTLs round up to whole seconds. Zero and negative TTLs have no
// expiry. A TTL above [MaxTTL] returns [ErrTTLOutOfRange].
func (p *Storage) Save(_ context.Context, key string, value []byte, ttl time.Duration) error {
	seconds, err := ttlSeconds[int](ttl)
	if err != nil {
		return err
	}
	if err := p.cache.Set(storageKey(key), value, seconds); err != nil {
		if errors.Is(err, freecache.ErrLargeEntry) {
			return fmt.Errorf("%w: %w", storages.ErrEntryRejected, err)
		}
		return err
	}
	return nil
}

func ttlSeconds[T int | int32 | int64](ttl time.Duration) (T, error) {
	if ttl <= 0 {
		return 0, nil
	}
	if ttl > MaxTTL {
		return 0, ErrTTLOutOfRange
	}
	return T((ttl + time.Second - 1) / time.Second), nil
}

// Exists reports whether key is present in the cache.
func (p *Storage) Exists(_ context.Context, key string) (bool, error) {
	// GetFn lends the stored bytes instead of copying them, which Get does.
	err := p.cache.GetFn(storageKey(key), func([]byte) error { return nil })
	if err != nil {
		if errors.Is(err, freecache.ErrNotFound) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// Get retrieves the value for key. Returns [storages.ErrMissing] if the key does not exist.
func (p *Storage) Get(_ context.Context, key string) ([]byte, error) {
	data, err := p.cache.Get(storageKey(key))
	if err != nil {
		if errors.Is(err, freecache.ErrNotFound) {
			return nil, storages.ErrMissing
		}
		return nil, err
	}
	return data, nil
}

// Delete removes key from the cache.
func (p *Storage) Delete(_ context.Context, key string) error {
	p.cache.Del(storageKey(key))
	return nil
}

// DeleteMany removes multiple keys. The deletes are in-process and cheap, so a
// plain loop beats a worker pool.
func (p *Storage) DeleteMany(_ context.Context, keys ...string) error {
	for _, key := range keys {
		p.cache.Del(storageKey(key))
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

// storageKey caps the portion of the entry limit consumed by a key. Using the
// digest whenever it shortens a key keeps the physical key stable as the size
// of its stored value changes. Raw keys under the reserved prefix are hashed too.
func storageKey(key string) []byte {
	const digestPrefix = "\x00atlas-freecache-digest:"
	if len(key) <= len(digestPrefix)+sha256.Size && !strings.HasPrefix(key, digestPrefix) {
		return []byte(key)
	}
	sum := sha256.Sum256([]byte(key))
	return append([]byte(digestPrefix), sum[:]...)
}

// InProcess reports true: freecache lives in process memory, so the cache adds no
// default timeout to its calls (see [storages.InProcess]).
func (p *Storage) InProcess() bool { return true }

var (
	_ storages.Storage     = (*Storage)(nil)
	_ storages.MultiGetter = (*Storage)(nil)
	_ storages.InProcess   = (*Storage)(nil)
)
