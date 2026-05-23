// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package plugins

import (
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"github.com/altessa-s/go-atlas/core/encoding/hash"
)

// hashCacheEntry stores cached hash with file modification time.
type hashCacheEntry struct {
	hash  string
	mtime time.Time
	size  int64
}

// hashCache provides thread-safe caching of file hashes with mtime validation.
type hashCache struct {
	mu      sync.RWMutex
	entries map[string]hashCacheEntry // path -> entry
}

// newHashCache creates a new hash cache.
func newHashCache() *hashCache {
	return &hashCache{
		entries: make(map[string]hashCacheEntry),
	}
}

// get returns the cached hash if the file hasn't changed since caching.
// Returns empty string if cache miss or file has been modified.
func (c *hashCache) get(path string) (string, bool) {
	c.mu.RLock()
	entry, exists := c.entries[path]
	c.mu.RUnlock()

	if !exists {
		return "", false
	}

	// Verify file hasn't changed
	stat, err := os.Stat(path)
	if err != nil {
		// File might have been deleted
		c.mu.Lock()
		delete(c.entries, path)
		c.mu.Unlock()
		return "", false
	}

	// Check if mtime or size changed
	if !stat.ModTime().Equal(entry.mtime) || stat.Size() != entry.size {
		// File has been modified, invalidate cache
		c.mu.Lock()
		delete(c.entries, path)
		c.mu.Unlock()
		return "", false
	}

	return entry.hash, true
}

// put stores a hash in the cache with current file metadata.
func (c *hashCache) put(path, hash string, stat os.FileInfo) {
	c.mu.Lock()
	c.entries[path] = hashCacheEntry{
		hash:  hash,
		mtime: stat.ModTime(),
		size:  stat.Size(),
	}
	c.mu.Unlock()
}

// clear removes all entries from the cache.
func (c *hashCache) clear() {
	c.mu.Lock()
	c.entries = make(map[string]hashCacheEntry)
	c.mu.Unlock()
}

// remove deletes a specific entry from the cache.
func (c *hashCache) remove(path string) {
	c.mu.Lock()
	delete(c.entries, path)
	c.mu.Unlock()
}

// readAndHashFile reads a plugin file and computes its SHA-256 hash.
// Uses caching to avoid redundant I/O when file hasn't changed.
func readAndHashFileWithCache(path string, cache *hashCache) ([]byte, string, error) {
	// Check cache first
	if cache != nil {
		if hash, ok := cache.get(path); ok {
			// Cache hit - still need to read the file data for plugin loading
			data, err := os.ReadFile(path)
			if err != nil {
				return nil, "", fmt.Errorf("read plugin: %w", err)
			}
			return data, hash, nil
		}
	}

	// Cache miss or no cache - read and hash
	file, err := os.Open(path)
	if err != nil {
		return nil, "", fmt.Errorf("open plugin: %w", err)
	}
	defer file.Close()

	// Get file info for cache
	stat, err := file.Stat()
	if err != nil {
		return nil, "", fmt.Errorf("stat plugin: %w", err)
	}

	// Read file
	data, err := io.ReadAll(file)
	if err != nil {
		return nil, "", fmt.Errorf("read plugin: %w", err)
	}

	// Compute hash
	hashStr := hash.SHA256HexBytes(data)

	// Store in cache
	if cache != nil {
		cache.put(path, hashStr, stat)
	}

	return data, hashStr, nil
}

// readAndHashFile reads the file at path into memory and returns the raw
// bytes together with the SHA256 hex digest. Returning the bytes avoids
// a second read when signature verification needs the same data.
//
// The entire file is read at once; acceptable for .so files (typically
// under 100 MB) at load time.
//
// Tests override this via the [Manager.readAndHashFileFn] field.
func readAndHashFile(path string) ([]byte, string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, "", err
	}
	return data, hash.SHA256HexBytes(data), nil
}
