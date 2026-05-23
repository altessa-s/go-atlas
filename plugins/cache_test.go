// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package plugins

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestHashCache_GetPut(t *testing.T) {
	t.Parallel()

	cache := newHashCache()

	// Create a temp file
	dir := t.TempDir()
	path := filepath.Join(dir, "test.so")
	data := []byte("test plugin content")
	require.NoError(t, os.WriteFile(path, data, 0o644))

	// Get file info
	stat, err := os.Stat(path)
	require.NoError(t, err)

	// Test cache miss
	hash, ok := cache.get(path)
	require.False(t, ok)
	require.Empty(t, hash)

	// Put in cache
	expectedHash := "abc123"
	cache.put(path, expectedHash, stat)

	// Test cache hit
	hash, ok = cache.get(path)
	require.True(t, ok)
	require.Equal(t, expectedHash, hash)
}

func TestHashCache_InvalidatesOnMtimeChange(t *testing.T) {
	t.Parallel()

	cache := newHashCache()

	// Create a temp file
	dir := t.TempDir()
	path := filepath.Join(dir, "test.so")
	data := []byte("initial content")
	require.NoError(t, os.WriteFile(path, data, 0o644))

	// Get initial file info and cache
	stat1, err := os.Stat(path)
	require.NoError(t, err)
	cache.put(path, "hash1", stat1)

	// Verify cached
	hash, ok := cache.get(path)
	require.True(t, ok)
	require.Equal(t, "hash1", hash)

	// Modify file after a delay to ensure mtime changes
	time.Sleep(10 * time.Millisecond)
	require.NoError(t, os.WriteFile(path, []byte("modified content"), 0o644))

	// Cache should be invalidated
	hash, ok = cache.get(path)
	require.False(t, ok)
	require.Empty(t, hash)

	// Put new hash
	stat2, err := os.Stat(path)
	require.NoError(t, err)
	cache.put(path, "hash2", stat2)

	// Verify new cache entry
	hash, ok = cache.get(path)
	require.True(t, ok)
	require.Equal(t, "hash2", hash)
}

func TestHashCache_InvalidatesOnSizeChange(t *testing.T) {
	t.Parallel()

	cache := newHashCache()

	// Create a temp file
	dir := t.TempDir()
	path := filepath.Join(dir, "test.so")
	data := []byte("abc")
	require.NoError(t, os.WriteFile(path, data, 0o644))

	// Get initial file info and cache
	stat1, err := os.Stat(path)
	require.NoError(t, err)
	cache.put(path, "hash1", stat1)

	// Verify cached
	hash, ok := cache.get(path)
	require.True(t, ok)
	require.Equal(t, "hash1", hash)

	// Change file size without changing mtime (theoretical edge case)
	// In practice, this would normally change mtime too
	require.NoError(t, os.WriteFile(path, []byte("abcdef"), 0o644))

	// Cache should be invalidated due to size change
	hash, ok = cache.get(path)
	require.False(t, ok)
	require.Empty(t, hash)
}

func TestHashCache_HandlesDeletedFile(t *testing.T) {
	t.Parallel()

	cache := newHashCache()

	// Create a temp file
	dir := t.TempDir()
	path := filepath.Join(dir, "test.so")
	data := []byte("test content")
	require.NoError(t, os.WriteFile(path, data, 0o644))

	// Cache it
	stat, err := os.Stat(path)
	require.NoError(t, err)
	cache.put(path, "hash1", stat)

	// Verify cached
	hash, ok := cache.get(path)
	require.True(t, ok)
	require.Equal(t, "hash1", hash)

	// Delete file
	require.NoError(t, os.Remove(path))

	// Cache get should handle gracefully
	hash, ok = cache.get(path)
	require.False(t, ok)
	require.Empty(t, hash)

	// Entry should be removed from cache
	cache.mu.RLock()
	_, exists := cache.entries[path]
	cache.mu.RUnlock()
	require.False(t, exists)
}

func TestHashCache_Clear(t *testing.T) {
	t.Parallel()

	cache := newHashCache()
	dir := t.TempDir()

	// Add multiple entries
	for i := 0; i < 3; i++ {
		path := filepath.Join(dir, fmt.Sprintf("test%d.so", i))
		require.NoError(t, os.WriteFile(path, []byte("data"), 0o644))
		stat, err := os.Stat(path)
		require.NoError(t, err)
		cache.put(path, fmt.Sprintf("hash%d", i), stat)
	}

	// Verify all cached
	for i := 0; i < 3; i++ {
		path := filepath.Join(dir, fmt.Sprintf("test%d.so", i))
		hash, ok := cache.get(path)
		require.True(t, ok)
		require.Equal(t, fmt.Sprintf("hash%d", i), hash)
	}

	// Clear cache
	cache.clear()

	// Verify all entries removed
	for i := 0; i < 3; i++ {
		path := filepath.Join(dir, fmt.Sprintf("test%d.so", i))
		hash, ok := cache.get(path)
		require.False(t, ok)
		require.Empty(t, hash)
	}
}

func TestHashCache_Remove(t *testing.T) {
	t.Parallel()

	cache := newHashCache()
	dir := t.TempDir()

	// Add multiple entries
	paths := make([]string, 3)
	for i := 0; i < 3; i++ {
		path := filepath.Join(dir, fmt.Sprintf("test%d.so", i))
		paths[i] = path
		require.NoError(t, os.WriteFile(path, []byte("data"), 0o644))
		stat, err := os.Stat(path)
		require.NoError(t, err)
		cache.put(path, fmt.Sprintf("hash%d", i), stat)
	}

	// Remove middle entry
	cache.remove(paths[1])

	// First and last should still be cached
	hash, ok := cache.get(paths[0])
	require.True(t, ok)
	require.Equal(t, "hash0", hash)

	hash, ok = cache.get(paths[2])
	require.True(t, ok)
	require.Equal(t, "hash2", hash)

	// Middle should be gone
	hash, ok = cache.get(paths[1])
	require.False(t, ok)
	require.Empty(t, hash)
}

func TestReadAndHashFileWithCache(t *testing.T) {
	t.Parallel()

	cache := newHashCache()

	// Create a temp file
	dir := t.TempDir()
	path := filepath.Join(dir, "test.so")
	expectedData := []byte("test plugin content")
	require.NoError(t, os.WriteFile(path, expectedData, 0o644))

	// First read - cache miss
	data1, hash1, err := readAndHashFileWithCache(path, cache)
	require.NoError(t, err)
	require.Equal(t, expectedData, data1)
	require.NotEmpty(t, hash1)

	// Second read - cache hit (hash is reused but file is still read)
	data2, hash2, err := readAndHashFileWithCache(path, cache)
	require.NoError(t, err)
	require.Equal(t, expectedData, data2)
	require.Equal(t, hash1, hash2)

	// Modify file
	time.Sleep(10 * time.Millisecond)
	newData := []byte("modified content")
	require.NoError(t, os.WriteFile(path, newData, 0o644))

	// Third read - cache invalidated
	data3, hash3, err := readAndHashFileWithCache(path, cache)
	require.NoError(t, err)
	require.Equal(t, newData, data3)
	require.NotEqual(t, hash1, hash3)
}

func TestReadAndHashFileWithCache_NilCache(t *testing.T) {
	t.Parallel()

	// Create a temp file
	dir := t.TempDir()
	path := filepath.Join(dir, "test.so")
	expectedData := []byte("test plugin content")
	require.NoError(t, os.WriteFile(path, expectedData, 0o644))

	// Should work without cache
	data, hash, err := readAndHashFileWithCache(path, nil)
	require.NoError(t, err)
	require.Equal(t, expectedData, data)
	require.NotEmpty(t, hash)
}

func BenchmarkHashCache_Get(b *testing.B) {
	cache := newHashCache()
	dir := b.TempDir()
	path := filepath.Join(dir, "bench.so")

	// Create file and cache it
	require.NoError(b, os.WriteFile(path, []byte("bench data"), 0o644))
	stat, _ := os.Stat(path)
	cache.put(path, "hash123", stat)

	b.ResetTimer()
	for b.Loop() {
		_, _ = cache.get(path)
	}
}

func BenchmarkHashCache_Put(b *testing.B) {
	cache := newHashCache()
	dir := b.TempDir()
	path := filepath.Join(dir, "bench.so")

	require.NoError(b, os.WriteFile(path, []byte("bench data"), 0o644))
	stat, _ := os.Stat(path)

	b.ResetTimer()
	for b.Loop() {
		cache.put(path, "hash123", stat)
	}
}

func BenchmarkReadAndHashFile_WithCache(b *testing.B) {
	cache := newHashCache()
	dir := b.TempDir()
	path := filepath.Join(dir, "bench.so")

	// Create 1MB file
	data := make([]byte, 1<<20)
	require.NoError(b, os.WriteFile(path, data, 0o644))

	b.SetBytes(int64(len(data)))
	b.ResetTimer()
	for b.Loop() {
		_, _, _ = readAndHashFileWithCache(path, cache)
	}
}

func BenchmarkReadAndHashFile_NoCache(b *testing.B) {
	dir := b.TempDir()
	path := filepath.Join(dir, "bench.so")

	// Create 1MB file
	data := make([]byte, 1<<20)
	require.NoError(b, os.WriteFile(path, data, 0o644))

	b.SetBytes(int64(len(data)))
	b.ResetTimer()
	for b.Loop() {
		_, _, _ = readAndHashFileWithCache(path, nil)
	}
}
