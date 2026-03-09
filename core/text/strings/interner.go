// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package strings

import (
	"cmp"
	"fmt"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unique"
)

const (
	// DefaultMaxSize is the default maximum number of interned strings that an
	// [Interner] holds before LRU eviction is triggered. Used by [NewInterner]
	// when the caller supplies a non-positive maxSize and by [GlobalInterner]
	// for the singleton instance.
	DefaultMaxSize = 8192

	// EvictionBatchSize is the upper bound on the number of entries removed in a
	// single background eviction pass. The actual count may be larger when the
	// interner is significantly over capacity (see [EvictionRatio]).
	EvictionBatchSize = 512

	// EvictionRatio determines the minimum fraction of the cache to evict
	// (1/EvictionRatio). A value of 4 means at least 25 % of entries are
	// considered for removal, reducing the frequency of eviction runs.
	EvictionRatio = 4
)

// coarseTimestamp caches time.Now().Unix() to avoid a syscall on every intern hit.
// It is refreshed every PromotionCheckInterval accesses (~50 operations).
var coarseTimestamp atomic.Int64

func init() {
	coarseTimestamp.Store(time.Now().Unix())
}

// coarseNowUnix returns the cached Unix timestamp. Callers accept ~50-operation staleness.
func coarseNowUnix() int64 {
	return coarseTimestamp.Load()
}

// evictionCandidate holds data for a single eviction candidate during LRU sweep.
type evictionCandidate struct {
	key         string
	accessCount int64
	lastAccess  int64
}

// evictionCandidatesPool reuses candidate slices to avoid allocating ~256KB per eviction cycle.
var evictionCandidatesPool = sync.Pool{
	New: func() any {
		s := make([]evictionCandidate, 0, 512) //nolint:mnd // EvictionBatchSize
		return &s
	},
}

// internEntry represents an interned string with access tracking for LRU eviction.
// It uses unique.Handle for efficient underlying storage and comparison.
type internEntry struct {
	handle      unique.Handle[string]
	accessCount atomic.Int64
	lastAccess  atomic.Int64 // Unix timestamp for cleanup
}

// value returns the underlying string value from the handle.
func (e *internEntry) value() string {
	return e.handle.Value()
}

const (
	// HotCacheSlots is the number of pre-allocated atomic pointer slots in the
	// [Interner] hot cache. Each slot holds at most one frequently accessed string,
	// selected by a fast hash of the string value.
	HotCacheSlots = 32

	// HotCacheThreshold is the minimum cumulative access count an interned string
	// must reach before it is eligible for promotion from the cold cache
	// ([sync.Map]) to the lock-free hot cache ([atomic.Pointer] slots).
	HotCacheThreshold = 5

	// PromotionCheckInterval controls how often the [Interner] evaluates whether
	// a string should be promoted to the hot cache. A check occurs every
	// PromotionCheckInterval accesses across all strings.
	PromotionCheckInterval = 50
)

// Interner provides lock-free, concurrency-safe string interning with LRU
// eviction. Identical string values are deduplicated so that only one copy is
// retained in memory, significantly reducing heap usage for workloads with
// many repeated strings.
//
// The implementation uses a two-tier cache:
//
//   - A hot cache of [HotCacheSlots] [atomic.Pointer] slots for the most
//     frequently accessed strings, providing wait-free reads.
//   - A cold cache backed by [sync.Map] for general-purpose interning.
//
// Strings that exceed [HotCacheThreshold] accesses are automatically promoted
// to the hot cache. When the total number of interned strings exceeds the
// configured maximum, a background goroutine evicts the least-recently-used
// entries (see [EvictionBatchSize] and [EvictionRatio]).
//
// All methods on Interner are safe for concurrent use by multiple goroutines.
// Create instances with [NewInterner] or use the process-wide singleton
// returned by [GlobalInterner].
type Interner struct {
	// Hot cache: Pre-allocated atomic slots for most frequent strings
	hotCache [HotCacheSlots]atomic.Pointer[string]

	// Cold cache: sync.Map for general string interning
	entries     sync.Map     // Map[string]*internEntry
	currentSize atomic.Int64 // Atomic counter
	maxSize     int64        // Maximum number of entries
	evicting    atomic.Int32 // Atomic flag for eviction coordination

	// Promotion tracking
	promotionCounter atomic.Int64
}

var globalInternerPtr atomic.Pointer[Interner]

// GlobalInterner returns the process-wide singleton [Interner] initialized with
// [DefaultMaxSize]. The instance is created lazily on the first call using an
// atomic compare-and-swap, so concurrent callers are safe and will always
// receive the same instance.
func GlobalInterner() *Interner {
	if interner := globalInternerPtr.Load(); interner != nil {
		return interner
	}

	newInterner := NewInterner(DefaultMaxSize)
	if !globalInternerPtr.CompareAndSwap(nil, newInterner) {
		return globalInternerPtr.Load()
	}
	return newInterner
}

// ResetGlobalInterner clears all state from the global [Interner] singleton
// (interned strings, hot cache, counters). This is primarily useful in tests
// to prevent state leaking between test cases:
//
//	func TestFoo(t *testing.T) {
//	    t.Cleanup(strings.ResetGlobalInterner)
//	    // ...
//	}
func ResetGlobalInterner() {
	if interner := globalInternerPtr.Load(); interner != nil {
		interner.Reset()
	}
}

// NewInterner creates a new [Interner] that holds up to maxSize interned
// strings before triggering background LRU eviction. If maxSize is less than
// or equal to zero, [DefaultMaxSize] is used instead.
func NewInterner(maxSize int) *Interner {
	if maxSize <= 0 {
		maxSize = DefaultMaxSize
	}

	return &Interner{
		maxSize: int64(maxSize),
	}
}

// String returns the canonical (deduplicated) instance of s. If an equal string
// has already been interned, the previously stored instance is returned,
// allowing the caller's copy to be garbage collected. Empty strings are
// returned immediately without interning.
//
// The lookup proceeds in three phases:
//  1. Hot cache -- lock-free [atomic.Pointer] read (fastest).
//  2. Cold cache -- [sync.Map] lookup with access-count tracking.
//  3. Slow path  -- creates a new [unique.Handle], stores it, and may trigger
//     background eviction if the interner is over capacity.
func (si *Interner) String(s string) string {
	if s == "" {
		return s
	}

	// Ultra-fast path: Check hot cache first (atomic.Pointer access)
	hotSlot := si.fastHash(s) % HotCacheSlots
	if hotPtr := si.hotCache[hotSlot].Load(); hotPtr != nil {
		if *hotPtr == s {
			return *hotPtr
		}
	}

	// Fast path: lookup in cold cache
	if value, ok := si.entries.Load(s); ok {
		entry := value.(*internEntry) //nolint:errcheck // type is guaranteed by internal usage

		// Update access information atomically using coarse timestamp
		newAccessCount := entry.accessCount.Add(1)
		entry.lastAccess.Store(coarseNowUnix())

		// Periodically check for hot cache promotion and refresh coarse timestamp
		if si.promotionCounter.Add(1)%PromotionCheckInterval == 0 {
			coarseTimestamp.Store(time.Now().Unix())
			if newAccessCount >= HotCacheThreshold {
				si.promoteToHotCache(entry.value(), hotSlot, newAccessCount)
			}
		}

		return entry.value()
	}

	// Slow path: create new handle and entry
	handle := unique.Make(s)
	canonicalString := handle.Value()
	now := coarseNowUnix()
	newEntry := &internEntry{handle: handle}
	newEntry.accessCount.Store(1)
	newEntry.lastAccess.Store(now)

	if actual, loaded := si.entries.LoadOrStore(canonicalString, newEntry); loaded {
		entry := actual.(*internEntry) //nolint:errcheck // type is guaranteed by internal usage
		entry.accessCount.Add(1)
		entry.lastAccess.Store(now)
		return entry.value()
	}

	newSize := si.currentSize.Add(1)
	if newSize > si.maxSize {
		si.triggerBackgroundEviction()
	}

	return canonicalString
}

// UpperString converts s to uppercase and returns the canonical interned
// instance of the result. Equivalent to calling si.String(strings.ToUpper(s)).
//
// Example:
//
//	s := interner.UpperString("hello")  // "HELLO"
func (si *Interner) UpperString(s string) string {
	return si.String(strings.ToUpper(s))
}

// LowerString converts s to lowercase and returns the canonical interned
// instance of the result. Equivalent to calling si.String(strings.ToLower(s)).
//
// Example:
//
//	s := interner.LowerString("HELLO")  // "hello"
func (si *Interner) LowerString(s string) string {
	return si.String(strings.ToLower(s))
}

// triggerBackgroundEviction triggers background eviction if not already running.
func (si *Interner) triggerBackgroundEviction() {
	// Only one eviction goroutine at a time using atomic compare-and-swap
	if si.evicting.CompareAndSwap(0, 1) {
		go si.evictInBackground()
	}
}

// evictInBackground performs LRU eviction in a separate goroutine.
func (si *Interner) evictInBackground() {
	defer si.evicting.Store(0)

	currentSize := si.currentSize.Load()
	if currentSize <= si.maxSize {
		return // Size may have been reduced by another eviction
	}

	// Determine how many entries to evict (at least 25% to reduce frequent evictions)
	evictCount := max(int64(1), min(int64(EvictionBatchSize), currentSize/EvictionRatio))
	if currentSize > si.maxSize {
		// Evict more aggressively if over capacity
		evictCount = max(evictCount, currentSize-si.maxSize+1)
	}

	// Get a pooled candidates slice to avoid allocating ~256KB per eviction cycle.
	candidatesPtr := evictionCandidatesPool.Get().(*[]evictionCandidate) //nolint:errcheck // pool type is guaranteed
	candidates := (*candidatesPtr)[:0]

	si.entries.Range(func(key, value any) bool {
		entry := value.(*internEntry) //nolint:errcheck // Range guarantees correct type
		candidates = append(candidates, evictionCandidate{
			key:         key.(string), //nolint:errcheck // Range guarantees correct type
			accessCount: entry.accessCount.Load(),
			lastAccess:  entry.lastAccess.Load(),
		})
		return true
	})

	if len(candidates) == 0 {
		*candidatesPtr = candidates
		evictionCandidatesPool.Put(candidatesPtr)
		return
	}

	// Sort candidates by access count (ascending), then by last access (ascending)
	// This gives us LRU eviction with frequency consideration
	slices.SortFunc(candidates, func(a, b evictionCandidate) int {
		if c := cmp.Compare(a.accessCount, b.accessCount); c != 0 {
			return c
		}
		return cmp.Compare(a.lastAccess, b.lastAccess)
	})

	// Evict the least accessed entries
	actualEvictCount := min(evictCount, int64(len(candidates)))
	evictedCount := int64(0)
	for i := range actualEvictCount {
		key := candidates[i].key
		if _, loaded := si.entries.LoadAndDelete(key); loaded {
			evictedCount++
		}
	}

	// Return pooled slice
	*candidatesPtr = candidates
	evictionCandidatesPool.Put(candidatesPtr)

	// Update size counter
	si.currentSize.Add(-evictedCount)
}

// Reset removes all interned strings from both the hot and cold caches and
// resets internal counters to zero. After Reset, [Interner.Size] returns 0.
//
// Reset is safe to call concurrently with other methods, but callers should
// be aware that concurrent [Interner.String] calls may re-populate the cache
// immediately.
//
// Example:
//
//	interner.Reset()  // clears all cached strings
func (si *Interner) Reset() {
	// Clear all entries
	si.entries.Range(func(key, value any) bool {
		si.entries.Delete(key)
		return true
	})

	// Reset size counter
	si.currentSize.Store(0)

	// Reset eviction flag
	si.evicting.Store(0)

	// Reset hot cache
	for i := range si.hotCache {
		si.hotCache[i].Store(nil)
	}

	// Reset promotion counter
	si.promotionCounter.Store(0)
}

// Size returns the current number of strings held in the cold cache. The
// count is maintained atomically and is safe to read concurrently. Hot cache
// entries are a subset of cold cache entries, so they do not add to the total.
func (si *Interner) Size() int64 {
	return si.currentSize.Load()
}

// MaxSize returns the maximum number of strings the [Interner] will hold
// before background LRU eviction is triggered.
func (si *Interner) MaxSize() int64 {
	return si.maxSize
}

// IsEmpty reports whether the cold cache contains zero interned strings.
func (si *Interner) IsEmpty() bool {
	return si.currentSize.Load() == 0
}

// IsFull reports whether the number of interned strings has reached or
// exceeded [Interner.MaxSize]. When full, the next call to [Interner.String]
// that inserts a new entry will trigger background eviction.
func (si *Interner) IsFull() bool {
	return si.currentSize.Load() >= si.maxSize
}

// LoadFactor returns the ratio of current interned strings to the maximum
// capacity, as a float64 in the range [0.0, 1.0+]. Values above 1.0 are
// possible transiently while background eviction is in progress. Returns 0
// if [Interner.MaxSize] is zero.
func (si *Interner) LoadFactor() float64 {
	current := float64(si.currentSize.Load())
	maximum := float64(si.maxSize)
	if maximum == 0 {
		return 0
	}
	return current / maximum
}

// TrimString strips leading and trailing whitespace from s and returns the
// canonical interned instance of the result.
//
// Example:
//
//	s := interner.TrimString("  hello  ")  // "hello"
func (si *Interner) TrimString(s string) string {
	return si.String(strings.TrimSpace(s))
}

// CleanPathString normalizes a file path by collapsing consecutive slashes
// into one and removing any trailing slash (except for the root "/"), then
// returns the canonical interned instance. An empty path is interned as the
// empty string.
//
// Example:
//
//	s := interner.CleanPathString("//foo//bar/")  // "/foo/bar"
func (si *Interner) CleanPathString(path string) string {
	if path == "" {
		return si.String("")
	}

	// Fast path: no consecutive slashes and no trailing slash — intern as-is.
	if !strings.Contains(path, "//") && (len(path) <= 1 || path[len(path)-1] != '/') {
		return si.String(path)
	}

	// Single-pass: collapse consecutive slashes and strip trailing slash.
	b := GetStringBuilder()
	defer PutStringBuilder(b)
	b.Grow(len(path))

	prev := byte(0)
	for i := range len(path) {
		ch := path[i]
		if ch == '/' && prev == '/' {
			prev = ch
			continue
		}
		b.WriteByte(ch)
		prev = ch
	}

	// Remove trailing slash unless it's root
	result := b.String()
	if len(result) > 1 && result[len(result)-1] == '/' {
		result = result[:len(result)-1]
	}

	return si.String(result)
}

// PrefixString prepends prefix to s and returns the canonical interned
// instance of the concatenated result.
//
// Example:
//
//	s := interner.PrefixString("world", "hello ")  // "hello world"
func (si *Interner) PrefixString(s, prefix string) string {
	return si.String(prefix + s)
}

// SuffixString appends suffix to s and returns the canonical interned
// instance of the concatenated result.
//
// Example:
//
//	s := interner.SuffixString("hello", " world")  // "hello world"
func (si *Interner) SuffixString(s, suffix string) string {
	return si.String(s + suffix)
}

// WrapString surrounds s with prefix and suffix and returns the canonical
// interned instance of the concatenated result (prefix + s + suffix).
//
// Example:
//
//	s := interner.WrapString("hello", "[", "]")  // "[hello]"
func (si *Interner) WrapString(s, prefix, suffix string) string {
	return si.String(prefix + s + suffix)
}

// StringSlice interns every element of slice and returns a new slice
// containing the canonical instances. Returns nil when slice is nil or empty.
//
// Example:
//
//	s := interner.StringSlice([]string{"a", "b"})
func (si *Interner) StringSlice(slice []string) []string {
	if len(slice) == 0 {
		return nil
	}
	result := make([]string, len(slice))
	for i, s := range slice {
		result[i] = si.String(s)
	}
	return result
}

// StringMap interns every key and value in m and returns a new map
// containing the canonical instances. Returns nil when m is nil or empty.
//
// Example:
//
//	m := interner.StringMap(map[string]string{"a": "b"})
func (si *Interner) StringMap(m map[string]string) map[string]string {
	if len(m) == 0 {
		return nil
	}
	result := make(map[string]string, len(m))
	for k, v := range m {
		result[si.String(k)] = si.String(v)
	}
	return result
}

// FormatString formats a string using [fmt.Sprintf] and returns the canonical
// interned instance of the result. Useful for interning computed identifiers
// or composite keys.
//
// Example:
//
//	s := interner.FormatString("hello %s", "world")  // "hello world"
func (si *Interner) FormatString(format string, args ...any) string {
	formatted := fmt.Sprintf(format, args...)
	return si.String(formatted)
}

// JoinString concatenates parts with separator and returns the canonical
// interned instance of the result. Returns the interned empty string when
// parts is empty.
//
// Example:
//
//	s := interner.JoinString([]string{"a", "b"}, ",")  // "a,b"
func (si *Interner) JoinString(parts []string, separator string) string {
	if len(parts) == 0 {
		return si.String("")
	}
	joined := strings.Join(parts, separator)
	return si.String(joined)
}

// JoinWith applies a caller-supplied joiner function to parts and returns
// the canonical interned instance of the result. Returns the interned empty
// string when parts is empty.
//
// Example:
//
//	s := interner.JoinWith(parts, strings.Join)
func (si *Interner) JoinWith(parts []string, joiner func([]string) string) string {
	if len(parts) == 0 {
		return si.String("")
	}
	joined := joiner(parts)
	return si.String(joined)
}

// InternString returns the canonical (deduplicated) instance of s using the
// process-wide [GlobalInterner]. It is a convenience wrapper around
// GlobalInterner().String(s).
//
// Example:
//
//	s := InternString("hello")
func InternString(s string) string {
	return GlobalInterner().String(s)
}

// InternLowerString converts key to lowercase and returns the canonical
// interned instance via the [GlobalInterner].
//
// Example:
//
//	s := InternLowerString("HELLO")  // "hello"
func InternLowerString(key string) string {
	return GlobalInterner().LowerString(key)
}

// InternUpperString converts key to uppercase and returns the canonical
// interned instance via the [GlobalInterner].
//
// Example:
//
//	s := InternUpperString("hello")  // "HELLO"
func InternUpperString(key string) string {
	return GlobalInterner().UpperString(key)
}

// InternTrimString strips whitespace from s and returns the canonical interned
// instance via the [GlobalInterner].
//
// Example:
//
//	s := InternTrimString("  hello  ")  // "hello"
func InternTrimString(s string) string {
	return GlobalInterner().TrimString(s)
}

// InternCleanPathString normalizes path by collapsing duplicate slashes and
// removing trailing slashes, then returns the canonical interned instance via
// the [GlobalInterner]. See [Interner.CleanPathString] for details.
//
// Example:
//
//	s := InternCleanPathString("//foo//bar/")  // "/foo/bar"
func InternCleanPathString(path string) string {
	return GlobalInterner().CleanPathString(path)
}

// InternPrefixString prepends prefix to s and returns the canonical interned
// instance via the [GlobalInterner].
//
// Example:
//
//	s := InternPrefixString("world", "hello ")  // "hello world"
func InternPrefixString(s, prefix string) string {
	return GlobalInterner().PrefixString(s, prefix)
}

// InternSuffixString appends suffix to s and returns the canonical interned
// instance via the [GlobalInterner].
//
// Example:
//
//	s := InternSuffixString("hello", " world")  // "hello world"
func InternSuffixString(s, suffix string) string {
	return GlobalInterner().SuffixString(s, suffix)
}

// InternWrapString surrounds s with prefix and suffix and returns the
// canonical interned instance via the [GlobalInterner].
//
// Example:
//
//	s := InternWrapString("hello", "[", "]")  // "[hello]"
func InternWrapString(s, prefix, suffix string) string {
	return GlobalInterner().WrapString(s, prefix, suffix)
}

// InternStringSlice interns every element of slice and returns a new slice
// containing the canonical instances via the [GlobalInterner]. Returns nil
// when slice is nil or empty.
//
// Example:
//
//	s := InternStringSlice([]string{"a", "b"})
func InternStringSlice(slice []string) []string {
	return GlobalInterner().StringSlice(slice)
}

// InternStringMap interns every key and value of m and returns a new map
// containing the canonical instances via the [GlobalInterner]. Returns nil
// when m is nil or empty.
//
// Example:
//
//	m := InternStringMap(map[string]string{"a": "b"})
func InternStringMap(m map[string]string) map[string]string {
	return GlobalInterner().StringMap(m)
}

// InternFormatString formats a string with [fmt.Sprintf] and returns the
// canonical interned instance via the [GlobalInterner].
//
// Example:
//
//	s := InternFormatString("hello %s", "world")  // "hello world"
func InternFormatString(format string, args ...any) string {
	return GlobalInterner().FormatString(format, args...)
}

// InternJoinString concatenates parts with separator and returns the canonical
// interned instance via the [GlobalInterner].
//
// Example:
//
//	s := InternJoinString([]string{"a", "b"}, ",")  // "a,b"
func InternJoinString(parts []string, separator string) string {
	return GlobalInterner().JoinString(parts, separator)
}

// InternJoinWith applies joiner to parts and returns the canonical interned
// instance via the [GlobalInterner].
//
// Example:
//
//	s := InternJoinWith(parts, customJoiner)
func InternJoinWith(parts []string, joiner func([]string) string) string {
	return GlobalInterner().JoinWith(parts, joiner)
}

// fastHash computes a fast FNV-1a hash for hot cache slot distribution.
func (si *Interner) fastHash(s string) uint32 {
	if len(s) == 0 {
		return 0
	}

	// Simple FNV-1a hash variant optimized for short strings
	const fnvBasis = 2166136261
	const fnvPrime = 16777619

	hash := uint32(fnvBasis)
	for i := range len(s) {
		hash ^= uint32(s[i])
		hash *= fnvPrime
	}
	return hash
}

// promoteToHotCache attempts to promote frequently accessed string to hot cache.
// If the slot is empty, claims it. If occupied, replaces only when the new string
// has significantly higher access count (2x threshold) to avoid thrashing.
func (si *Interner) promoteToHotCache(s string, slot uint32, accessCount int64) {
	// Try to claim an empty slot first (common case)
	if si.hotCache[slot].CompareAndSwap(nil, &s) {
		return
	}

	// Slot is occupied — check if the new entry is significantly hotter
	current := si.hotCache[slot].Load()
	if current == nil {
		si.hotCache[slot].CompareAndSwap(nil, &s)
		return
	}

	if entry, ok := si.entries.Load(*current); ok {
		existing := entry.(*internEntry)                 //nolint:errcheck // type is guaranteed by internal usage
		if accessCount > existing.accessCount.Load()*2 { //nolint:mnd // 2x threshold for replacement
			si.hotCache[slot].Store(&s)
		}
	} else {
		// Previous occupant was evicted from cold cache; take the slot
		si.hotCache[slot].Store(&s)
	}
}
