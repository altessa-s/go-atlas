// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// cache.go provides caching infrastructure for time formatting and color instances.

package colorized

import (
	"time"
	"unsafe"

	"github.com/fatih/color"
)

// CacheShards is the number of shards used by the time cache.
const CacheShards = 16

// timeCacheKey uniquely identifies a time format combination
type timeCacheKey struct {
	unix   int64  // Unix timestamp in seconds or milliseconds
	format string // Time format string
}

// colorKey uniquely identifies a color combination
type colorKey struct {
	attrs [8]color.Attribute // Fixed size array for efficient hashing
	len   int
}

// newColorKey creates a color key from attributes
func newColorKey(attrs []color.Attribute) colorKey {
	var key colorKey
	key.len = min(len(attrs), len(key.attrs))
	copy(key.attrs[:key.len], attrs)
	return key
}

// Global cache instances
var (
	globalTimeCache  *shardedTimeCache
	globalColorCache *colorCacheAdapter
)

// colorCacheAdapter wraps the color cache with the color creation logic
type colorCacheAdapter struct {
	*colorCache
}

// #nosec G103 -- intentional unsafe for cache pointer storage
func (a *colorCacheAdapter) GetColor(attrs ...color.Attribute) *color.Color {
	if len(attrs) == 0 {
		return nil
	}

	key := newColorKey(attrs)

	// Try to get from cache
	if ptr := a.getColor(key); ptr != nil {
		return (*color.Color)(ptr)
	}

	// Create new color
	c := color.New(attrs...)
	a.putColor(key, unsafe.Pointer(c))
	return c
}

// formatTimeOptimized is the optimized entry point for time formatting
func formatTimeOptimized(t time.Time, format string) string {
	// Skip caching for empty times
	if t.IsZero() {
		return ""
	}

	// Determine cache key based on format
	var unix int64
	switch format {
	case time.RFC3339:
		unix = t.Unix()
	case time.RFC3339Nano:
		unix = t.UnixMilli()
	default:
		unix = t.Unix()
	}

	// Try to get from cache
	key := timeCacheKey{unix: unix, format: format}
	if formatted, ok := globalTimeCache.get(key); ok {
		return formatted
	}

	// Format and cache
	formatted := t.Format(format)
	globalTimeCache.put(key, formatted)

	return formatted
}

// getColor is the optimized entry point for color retrieval
func getColor(attrs ...color.Attribute) *color.Color {
	return globalColorCache.GetColor(attrs...)
}

// formatTime is the main entry point for time formatting with caching
func formatTime(t time.Time, format string) string {
	return formatTimeOptimized(t, format)
}

// preCacheColors pre-creates commonly used colors
func preCacheColors() {
	// Pre-cache level colors
	_ = getColor(color.BgBlue)   // Debug
	_ = getColor(color.BgGreen)  // Info
	_ = getColor(color.BgYellow) // Warn
	_ = getColor(color.BgRed)    // Error

	// Pre-cache common attribute colors
	_ = getColor(color.FgCyan)
	_ = getColor(color.FgWhite)
	_ = getColor(color.FgBlack)
	_ = getColor(color.FgRed, color.Bold)
	_ = getColor(color.BgCyan)
}

// Initialize caches at startup
func init() {
	globalTimeCache = newShardedTimeCache(1000 / CacheShards) //nolint:mnd
	globalColorCache = &colorCacheAdapter{newColorCache()}
	preCacheColors()
}
