// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package slices

import (
	"cmp"
	"container/list"
	"context"
	"errors"
	"runtime"
	"slices"
	"sync"

	"github.com/altessa-s/go-atlas/core/runtime/concurrency"
	"github.com/altessa-s/go-atlas/core/types/nilcheck"
)

const (
	// minMapCapacity is the minimum initial capacity used for internal maps in
	// functions like [GroupBy], ensuring a reasonable starting size even for
	// small collections.
	minMapCapacity = 16

	// maxPreallocMap is the maximum pre-allocated map size in [DeduplicateBy].
	// For collections larger than this, the seen-set map is capped at this value
	// to avoid over-allocation when duplicates are expected to be common.
	maxPreallocMap = 128
)

// errFiltered is a sentinel error used by [FilterParallel] to signal that an
// element did not pass the predicate. Using a pre-allocated sentinel avoids a
// heap allocation per rejected element.
var errFiltered = errors.New("filtered")

// Deduplicate returns a new slice containing only the unique elements of collection,
// preserving the order of first occurrence. It is a convenience wrapper around
// [DeduplicateBy] using an identity key function. If collection is empty, nil is returned.
//
// Example:
//
//	nums := []int{1, 2, 2, 3, 1, 4}
//	unique := Deduplicate(nums) // unique is []int{1, 2, 3, 4}
func Deduplicate[T comparable](collection []T) []T {
	// Use DeduplicateBy with identity function
	return DeduplicateBy(collection, func(v T) T { return v })
}

// Delete removes the first occurrence of el from collection and returns the modified
// slice. If el is not found, the original slice is returned unchanged. Only the first
// match is removed; use a loop or filter for removing all occurrences.
//
// Example:
//
//	words := []string{"a", "b", "c"}
//	result := Delete(words, "b") // result is []string{"a", "c"}
//
//	nums := []int{1, 2, 3}
//	result2 := Delete(nums, 4) // result2 is []int{1, 2, 3} (unchanged)
func Delete[T comparable](collection []T, el T) []T {
	idx := slices.Index(collection, el)
	if idx > -1 {
		return slices.Delete(collection, idx, idx+1)
	}
	return collection
}

// ToList converts a slice to a [container/list.List], preserving element order.
// If collection is empty, nil is returned. See [FromList] for the inverse operation.
//
// Example:
//
//	nums := []int{1, 2, 3}
//	l := ToList(nums) // l is a *list.List containing 1, 2, 3
func ToList[T any](collection []T) *list.List {
	if len(collection) == 0 {
		return nil
	}

	l := list.New()
	for _, value := range collection {
		l.PushBack(value)
	}
	return l
}

// FromList converts a [container/list.List] to a typed slice, preserving element order.
// Each element undergoes a type assertion to T; elements that cannot be asserted are
// silently skipped. If l is nil or empty, nil is returned.
// See [ToList] for the inverse operation.
//
// Example:
//
//	l := list.New()
//	l.PushBack(1)
//	l.PushBack(2)
//	nums := FromList[int](l) // nums is []int{1, 2}
func FromList[T any](l *list.List) []T {
	if l == nil || l.Len() == 0 {
		return nil
	}

	collection := make([]T, 0, l.Len())
	for e := l.Front(); e != nil; e = e.Next() {
		if v, ok := e.Value.(T); ok {
			collection = append(collection, v)
		}
	}
	return collection
}

// ToAny converts a typed slice to a []any, boxing each element. The underlying type
// constraint ~[]E allows named slice types to be passed directly.
// If collection is empty, nil is returned.
//
// Example:
//
//	nums := []int{1, 2, 3}
//	anySlice := ToAny(nums) // anySlice is []any{1, 2, 3}
func ToAny[T ~[]E, E any](collection T) []any {
	if len(collection) == 0 {
		return nil
	}

	result := make([]any, len(collection))
	for i, value := range collection {
		result[i] = value
	}
	return result
}

// To applies fn to each element of collection, returning a new slice of the
// transformed values. The underlying type constraint ~[]E allows named slice types
// to be passed directly. If collection is empty, nil is returned.
//
// Example:
//
//	nums := []int{1, 2, 3}
//	strs := To[int, string](nums, strconv.Itoa) // strs is []string{"1", "2", "3"}
func To[T ~[]E, E any, V any](collection T, fn func(E) V) []V {
	if len(collection) == 0 {
		return nil
	}

	result := make([]V, len(collection))
	for i, value := range collection {
		result[i] = fn(value)
	}
	return result
}

// ToStrings extracts all string-typed elements from a []any slice, preserving their
// order. Non-string elements are silently skipped.
// If collection is empty or contains no strings, nil is returned.
//
// Example:
//
//	anySlice := []any{"hello", 1, "world", true}
//	strs := ToStrings(anySlice) // strs is []string{"hello", "world"}
func ToStrings(collection []any) []string {
	if len(collection) == 0 {
		return nil
	}

	result := make([]string, 0, len(collection))
	for _, item := range collection {
		if str, ok := item.(string); ok {
			result = append(result, str)
		}
	}

	if len(result) == 0 {
		return nil
	}
	return result
}

// FilterParallel returns a new slice containing only the elements of collection for
// which predicate returns true, using multiple goroutines for large inputs. For slices
// smaller than batchSize * NumCPU * 2, it falls back to the sequential [Filter] path.
//
// The predicate must be safe for concurrent invocation from multiple goroutines.
// Element order in the result may not match the original order.
// If collection is empty, nil is returned.
func FilterParallel[T any](collection []T, predicate func(T) bool) []T {
	if len(collection) == 0 {
		return nil
	}

	const batchSize = 32
	cores := runtime.NumCPU()

	if len(collection) < batchSize*cores*2 {
		return slices.Collect(Filter(collection, predicate))
	}

	// Parallel processing.
	// We ignore the error because FilterParallel is designed to be a best-effort parallel filter
	// where errors (like our "filtered" sentinel) are expected and indicate exclusion.
	results, err := concurrency.ProcessCollect(context.Background(), collection, func(ctx context.Context, item T) (T, error) {
		if predicate(item) {
			return item, nil
		}
		var zero T
		return zero, errFiltered
	}, concurrency.BatchConfig[T]{
		StopOnError: false,
	})
	if err != nil {
		// Error is expected during filtering, we continue with whatever was collected.
		return results
	}

	return results
}

// FilterFirst returns the first element of collection that satisfies predicate,
// scanning from index 0 forward. It returns the element and true if found, or the
// zero value of T and false if no element matches. This is a zero-allocation
// operation with early return.
//
// Example:
//
//	nums := []int{1, 2, 3, 4, 5}
//	firstEven, found := FilterFirst(nums, func(n int) bool { return n%2 == 0 }) // firstEven is 2, found is true
//	_, notFound := FilterFirst(nums, func(n int) bool { return n > 10 })        // notFound is false
func FilterFirst[T any](collection []T, predicate func(T) bool) (T, bool) {
	for _, value := range collection {
		if predicate(value) {
			return value, true
		}
	}

	var t T
	return t, false
}

// FilterLast returns the last element of collection that satisfies predicate,
// scanning from the end backward. It returns the element and true if found, or the
// zero value of T and false if no element matches. This is a zero-allocation
// operation with early return.
//
// Example:
//
//	nums := []int{1, 2, 3, 4, 5}
//	lastEven, found := FilterLast(nums, func(n int) bool { return n%2 == 0 }) // lastEven is 4, found is true
//	_, notFound := FilterLast(nums, func(n int) bool { return n > 10 })       // notFound is false
func FilterLast[T any](collection []T, predicate func(T) bool) (T, bool) {
	for i := len(collection) - 1; i >= 0; i-- {
		if predicate(collection[i]) {
			return collection[i], true
		}
	}

	var t T
	return t, false
}

// Reduce folds collection into a single value of type R by applying fn to each element
// in order, threading an accumulator through each call. The initial parameter is the
// starting accumulator value. fn receives the current accumulator, the element index,
// and the element value. This is a zero-allocation operation.
//
// Example:
//
//	nums := []int{1, 2, 3, 4, 5}
//	sum := Reduce(nums, 0, func(acc, idx, val int) int { return acc + val }) // 15
func Reduce[T ~[]E, E any, R any](collection T, initial R, fn func(accum R, idx int, val E) R) R {
	accum := initial
	for i, value := range collection {
		accum = fn(accum, i, value)
	}
	return accum
}

// MapParallel applies fn to every element of collection, returning a new slice of
// the transformed values. For large slices (batchSize * NumCPU * 2 or more elements),
// it splits the work across NumCPU goroutines for parallel execution; smaller slices
// are processed sequentially to avoid goroutine overhead.
//
// The fn callback must be safe for concurrent invocation from multiple goroutines.
// Element order in the result matches the input order.
// If collection is empty, nil is returned.
//
// Example:
//
//	nums := make([]int, 10000)
//	squares := MapParallel(nums, func(n int) int { return n * n })
func MapParallel[T ~[]E, E any, O []OE, OE any](collection T, fn func(val E) OE) O {
	if len(collection) == 0 {
		return nil
	}

	out := make(O, len(collection))

	// Process in batches for better performance
	const batchSize = 32
	cores := runtime.NumCPU()

	if len(collection) < batchSize*cores*2 {
		// Sequential processing for small slices
		for i := range len(collection) {
			out[i] = fn(collection[i])
		}
		return out
	}

	// Parallel processing for large slices
	var wg sync.WaitGroup
	chunkSize := len(collection) / cores

	for i := range cores {
		start := i * chunkSize
		end := start + chunkSize
		if i == cores-1 {
			end = len(collection)
		}

		wg.Go(func() {
			for j := start; j < end; j++ {
				out[j] = fn(collection[j])
			}
		})
	}

	wg.Wait()
	return out
}

// ToWithFilter combines filtering and transformation in a single pass: it returns a
// new slice containing the results of applying fn to each element of collection for
// which predicate returns true. This avoids the intermediate allocation that would
// occur when chaining a separate filter and map step.
// If collection is empty or no elements satisfy the predicate, nil is returned.
//
// Example:
//
//	nums := []int{1, 2, 3, 4, 5}
//	// Convert only even numbers to strings
//	evenStrs := ToWithFilter(nums,
//	    func(n int) bool { return n%2 == 0 },
//	    func(n int) string { return fmt.Sprintf("even:%d", n) })
//	// evenStrs is []string{"even:2", "even:4"}
func ToWithFilter[T ~[]E, E any, O []OE, OE any](collection T, predicate func(E) bool, fn func(val E) OE) O {
	if len(collection) == 0 {
		return nil
	}

	var out O
	for _, value := range collection {
		if predicate(value) {
			out = append(out, fn(value))
		}
	}

	if len(out) == 0 {
		return nil
	}
	return out
}

// GroupBy partitions collection into groups keyed by the result of keyFn. Each key
// maps to a slice of all elements that produced that key, preserving the original
// element order within each group. The internal map is pre-allocated with a
// load-factor-aware estimate to reduce rehashing.
// If collection is empty, nil is returned.
//
// Example:
//
//	type Person struct {
//		Name string
//		Age  int
//	}
//	people := []Person{{"Alice", 30}, {"Bob", 25}, {"Charlie", 30}}
//	groups := GroupBy(people, func(p Person) int { return p.Age })
//	// groups is map[int][]Person{25: {{"Bob", 25}}, 30: {{"Alice", 30}, {"Charlie", 30}}}
func GroupBy[T any, K comparable](collection []T, keyFn func(T) K) map[K][]T {
	if len(collection) == 0 {
		return nil
	}

	// Use load factor for initial map size
	const loadFactor = 0.75
	initialSize := max(int(float64(len(collection))*loadFactor), minMapCapacity)

	groups := make(map[K][]T, initialSize)

	// Process with reduced allocations
	for _, item := range collection {
		key := keyFn(item)
		groups[key] = append(groups[key], item)
	}

	return groups
}

// Any reports whether at least one element in collection satisfies predicate.
// It short-circuits on the first match. If collection is empty, false is returned.
//
// Example:
//
//	nums := []int{1, 2, 3, 4, 5}
//	hasEven := Any(nums, func(n int) bool { return n%2 == 0 }) // hasEven is true
//	hasNegative := Any(nums, func(n int) bool { return n < 0 }) // hasNegative is false
func Any[T any](collection []T, predicate func(T) bool) bool {
	return slices.ContainsFunc(collection, predicate)
}

// All reports whether every element in collection satisfies predicate. It
// short-circuits on the first non-matching element. If collection is empty,
// true is returned (vacuous truth).
//
// Example:
//
//	nums := []int{2, 4, 6, 8}
//	allEven := All(nums, func(n int) bool { return n%2 == 0 }) // allEven is true
//	allPositive := All(nums, func(n int) bool { return n > 0 }) // allPositive is true
//
//	nums2 := []int{1, 2, 3, 4}
//	allEven2 := All(nums2, func(n int) bool { return n%2 == 0 }) // allEven2 is false
func All[T any](collection []T, predicate func(T) bool) bool {
	for _, item := range collection {
		if !predicate(item) {
			return false
		}
	}
	return true
}

// DeduplicateBy returns a new slice containing only the elements of collection whose
// key (as determined by keyFn) appears for the first time. Subsequent elements with
// the same key are dropped. Element order is preserved, and the first occurrence of
// each key wins.
//
// As an optimization, if no duplicates are found the original slice is returned
// without allocation. Consecutive duplicates are detected via a fast path that avoids
// a map lookup. If collection is empty, nil is returned.
//
// Example:
//
//	type Person struct {
//		ID   int
//		Name string
//	}
//	people := []Person{{1, "Alice"}, {2, "Bob"}, {1, "Alice2"}, {3, "Charlie"}}
//	unique := DeduplicateBy(people, func(p Person) int { return p.ID })
//	// unique is []Person{{1, "Alice"}, {2, "Bob"}, {3, "Charlie"}}
func DeduplicateBy[T any, K comparable](collection []T, keyFn func(T) K) []T {
	if len(collection) == 0 {
		return nil
	}

	if len(collection) == 1 {
		return collection
	}

	// Pre-allocate map with reasonable size
	mapSize := min(len(collection), maxPreallocMap)
	seen := make(map[K]struct{}, mapSize)

	// Copy-on-write optimization
	var result []T
	foundDuplicate := false

	// Cache for last seen key to optimize consecutive duplicates
	var lastKey K
	var hasLastKey bool

	for i := range collection {
		key := keyFn(collection[i])

		// Fast path: consecutive duplicates
		if hasLastKey && key == lastKey {
			if !foundDuplicate {
				foundDuplicate = true
				// Allocate result slice on first duplicate
				result = make([]T, 0, len(collection))
				// Copy all unique elements seen so far
				for j := range i {
					result = append(result, collection[j])
				}
			}
			continue
		}

		if _, exists := seen[key]; exists {
			if !foundDuplicate {
				foundDuplicate = true
				// Allocate result slice on first duplicate
				result = make([]T, 0, len(collection))
				// Copy all unique elements seen so far
				for j := range i {
					result = append(result, collection[j])
				}
			}
		} else {
			seen[key] = struct{}{}
			if foundDuplicate {
				result = append(result, collection[i])
			}
		}

		lastKey = key
		hasLastKey = true
	}

	// If no duplicates found, return original slice
	if !foundDuplicate {
		return collection
	}

	return result
}

// IsStrictlyIncreasing reports whether every adjacent pair (a, b) in collection
// satisfies a < b. An empty slice or a single-element slice returns true.
//
// Example:
//
//	nums := []int{1, 2, 3, 4, 5}
//	IsStrictlyIncreasing(nums) // true
//
//	nums2 := []int{1, 2, 2, 3}
//	IsStrictlyIncreasing(nums2) // false (2 == 2, not strictly increasing)
//
//	floats := []float64{0.1, 0.5, 1.0, 2.5}
//	IsStrictlyIncreasing(floats) // true
func IsStrictlyIncreasing[T cmp.Ordered](collection []T) bool {
	for i := range len(collection) - 1 {
		if collection[i] >= collection[i+1] {
			return false
		}
	}
	return true
}

// IsStrictlyDecreasing reports whether every adjacent pair (a, b) in collection
// satisfies a > b. An empty slice or a single-element slice returns true.
//
// Example:
//
//	nums := []int{5, 4, 3, 2, 1}
//	IsStrictlyDecreasing(nums) // true
//
//	nums2 := []int{5, 4, 4, 3}
//	IsStrictlyDecreasing(nums2) // false (4 == 4, not strictly decreasing)
func IsStrictlyDecreasing[T cmp.Ordered](collection []T) bool {
	for i := range len(collection) - 1 {
		if collection[i] <= collection[i+1] {
			return false
		}
	}
	return true
}

// IsNonDecreasing reports whether every adjacent pair (a, b) in collection satisfies
// a <= b (equal values are permitted). An empty slice or a single-element slice
// returns true.
//
// Example:
//
//	nums := []int{1, 2, 2, 3, 4}
//	IsNonDecreasing(nums) // true (equal values are allowed)
//
//	nums2 := []int{1, 3, 2, 4}
//	IsNonDecreasing(nums2) // false (3 > 2)
func IsNonDecreasing[T cmp.Ordered](collection []T) bool {
	for i := range len(collection) - 1 {
		if collection[i] > collection[i+1] {
			return false
		}
	}
	return true
}

// IsNonIncreasing reports whether every adjacent pair (a, b) in collection satisfies
// a >= b (equal values are permitted). An empty slice or a single-element slice
// returns true.
//
// Example:
//
//	nums := []int{5, 4, 4, 3, 2}
//	IsNonIncreasing(nums) // true (equal values are allowed)
//
//	nums2 := []int{5, 3, 4, 2}
//	IsNonIncreasing(nums2) // false (3 < 4)
func IsNonIncreasing[T cmp.Ordered](collection []T) bool {
	for i := range len(collection) - 1 {
		if collection[i] < collection[i+1] {
			return false
		}
	}
	return true
}

// AppendIf appends values to slice only when cond is true, returning the original
// slice unchanged otherwise. All variadic values are evaluated regardless of cond;
// use [AppendIfFunc] when value construction is expensive or may panic when cond is false.
//
// Example:
//
//	var attrs []any
//	attrs = AppendIf(attrs, name != "", "name", name)
//	attrs = AppendIf(attrs, age > 0, "age", age)
func AppendIf[T any](slice []T, cond bool, values ...T) []T {
	if !cond {
		return slice
	}
	return append(slice, values...)
}

// AppendIfFunc appends the values returned by fn to slice only when cond is true.
// Unlike [AppendIf], fn is called lazily -- only when cond is true -- making this
// safe for cases where constructing the values would panic or be expensive when the
// condition is false (e.g., dereferencing a pointer that may be nil).
//
// Example:
//
//	opts = AppendIfFunc(opts, cfg.IsConfigured(), func() []Option {
//		return []Option{WithFoo(cfg.Foo.Value)}
//	})
func AppendIfFunc[T any](slice []T, cond bool, fn func() []T) []T {
	if !cond {
		return slice
	}
	return append(slice, fn()...)
}

// AppendNonEmpty appends key and value as two consecutive []any elements only when
// value is a non-empty string, returning the original slice otherwise. This is a
// convenience for building optional key-value attribute lists for structured logging
// or similar APIs.
//
// Example:
//
//	var attrs []any
//	attrs = AppendNonEmpty(attrs, "name", appName)
//	attrs = AppendNonEmpty(attrs, "version", appVersion)
//	logger.With(attrs...)
func AppendNonEmpty(slice []any, key, value string) []any {
	if value == "" {
		return slice
	}
	return append(slice, key, value)
}

// AppendNonNil appends value to slice only when value is not nil, as determined by
// [nilcheck.IsNil]. This is useful for conditionally accumulating interface values,
// pointers, or other nillable types without explicit nil checks at every call site.
func AppendNonNil[T any](slice []T, value T) []T {
	if nilcheck.IsNil(value) {
		return slice
	}
	return append(slice, value)
}

// AppendNonNilErr calls fn and appends the resulting value to slice only when fn
// returns a non-nil value and a nil error. If fn returns an error, the original slice
// is returned along with the error. This combines lazy evaluation, nil-checking via
// [AppendNonNil], and error propagation in a single call.
func AppendNonNilErr[T any](slice []T, fn func() (T, error)) ([]T, error) {
	value, err := fn()
	if err != nil {
		return slice, err
	}

	return AppendNonNil(slice, value), nil
}
